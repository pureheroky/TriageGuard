package handlers

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"triageguard/apps/api/internal/config"
	"triageguard/apps/api/internal/db"
	"triageguard/apps/api/internal/jobs"
	"triageguard/apps/api/internal/linear"
	authmw "triageguard/apps/api/internal/middleware"
	"triageguard/apps/api/internal/observability"
	"triageguard/apps/api/internal/secret"
	"triageguard/apps/api/internal/slack"
	"triageguard/apps/api/internal/stripe"
)

type App struct {
	cfg          config.Config
	store        *db.Store
	slackClient  *slack.Client
	linearClient *linear.Client
	stripeClient *stripe.Client
	sentry       *observability.SentryClient
	alerts       *observability.AlertManager
	logger       *log.Logger
}

const maxJSONBodyBytes = 1 << 20 // 1 MiB

func NewApp(cfg config.Config, pool *pgxpool.Pool, tokenCipher *secret.TokenCipher) *App {
	logger := log.Default()
	sentryClient := observability.NewSentryClient(cfg.SentryDSN)
	alertManager := observability.NewAlertManager(observability.AlertConfig{
		WebhookURL:               cfg.AlertWebhookURL,
		FiveXXThreshold:          cfg.AlertFiveXXThreshold,
		FiveXXWindow:             time.Duration(cfg.AlertFiveXXWindowMinutes) * time.Minute,
		FiveXXCooldown:           time.Duration(cfg.AlertFiveXXCooldownMinutes) * time.Minute,
		DeadLetterAlertThreshold: cfg.AlertDeadLetterThreshold,
		DeadLetterCooldown:       time.Duration(cfg.AlertDeadLetterWindowMinutes) * time.Minute,
	}, sentryClient, logger)

	return &App{
		cfg:          cfg,
		store:        db.NewStore(pool, tokenCipher),
		slackClient:  slack.NewClient(),
		linearClient: linear.NewClient(),
		stripeClient: stripe.NewClient(cfg.StripeSecretKey),
		sentry:       sentryClient,
		alerts:       alertManager,
		logger:       logger,
	}
}

func (a *App) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(authmw.RequestLog(a.logger))
	r.Use(authmw.Observe5xx(a.onServerError))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(authmw.SecurityHeaders())
	r.Use(authmw.CORS(buildAllowedOrigins(a.cfg.WebBaseURL)))

	r.Get("/livez", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusOK, map[string]any{"ok": true})
	})
	readyHandler := func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := a.store.Ping(ctx); err != nil {
			jsonResponse(w, http.StatusServiceUnavailable, map[string]any{
				"ok": false,
				"dependencies": map[string]string{
					"db": "down",
				},
			})
			return
		}
		jsonResponse(w, http.StatusOK, map[string]any{
			"ok": true,
			"dependencies": map[string]string{
				"db": "up",
			},
		})
	}
	r.Get("/readyz", readyHandler)
	// Backward-compatible alias used by existing probes/scripts.
	r.Get("/healthz", readyHandler)

	r.Get("/slack/install", a.handleSlackInstall)
	r.Get("/slack/oauth/callback", a.handleSlackOAuthCallback)
	r.With(authmw.RateLimit(1200, 300)).Post("/slack/events", a.handleSlackEvents)
	r.With(authmw.RateLimit(900, 200)).Post("/slack/actions", a.handleSlackActions)

	r.Get("/linear/oauth/callback", a.handleLinearOAuthCallback)
	r.With(authmw.RateLimit(600, 150)).Post("/webhooks/stripe", a.handleStripeWebhook)
	r.With(authmw.RateLimit(600, 150)).Post("/webhooks/linear", a.handleLinearWebhook)

	r.Group(func(pr chi.Router) {
		pr.Use(authmw.RequireJWT(a.cfg.SupabaseJWTSecret, a.cfg.SupabaseURL))
		pr.Get("/api/me", a.handleGetMe)
		pr.Get("/api/billing", a.handleGetBilling)
		pr.Post("/api/billing/checkout", a.handleCreateBillingCheckout)
		pr.Post("/api/billing/portal", a.handleCreateBillingPortal)
		pr.Post("/api/billing/paypal/confirm", a.handleConfirmPayPalBilling)
		pr.Post("/api/workspace/delete", a.handleDeleteWorkspace)

		pr.Group(func(sr chi.Router) {
			sr.Use(a.requireActivePaidSubscription())
			sr.Get("/api/slack/install", a.handleSlackInstall)
			sr.Get("/api/linear/install", a.handleLinearInstall)
			sr.Get("/api/channels", a.handleListChannels)
			sr.Post("/api/channels/sync", a.handleSyncChannels)
			sr.Put("/api/channels", a.handleUpdateChannels)
			sr.Get("/api/channels/overrides", a.handleGetChannelOverrides)
			sr.Put("/api/channels/overrides", a.handleUpdateChannelOverrides)
			sr.Post("/api/slack/disconnect", a.handleDisconnectSlack)
			sr.Get("/api/policies", a.handleGetPolicies)
			sr.Put("/api/policies", a.handleUpdatePolicies)
			sr.Get("/api/linear", a.handleGetLinear)
			sr.Get("/api/linear/teams", a.handleListLinearTeams)
			sr.Put("/api/linear/defaults", a.handleSetLinearDefaults)
			sr.Post("/api/linear/disconnect", a.handleDisconnectLinear)
			sr.Get("/api/requests", a.handleListRequests)
			sr.Get("/api/requests/summary", a.handleRequestSummary)
			sr.Get("/api/reports/requests.csv", a.handleRequestsCSV)
			sr.Get("/api/reports/requests.pdf", a.handleRequestsPDF)
			sr.Get("/api/reports/analytics.csv", a.handleAnalyticsCSV)
			sr.Get("/api/reports/analytics.pdf", a.handleAnalyticsPDF)
			sr.Get("/api/activity", a.handleActivity)
			sr.Get("/api/dead-letters", a.handleListDeadLetters)
		})
	})

	r.With(authmw.RateLimit(60, 20)).Post("/jobs/sla", a.handleRunSLA)
	r.With(authmw.RateLimit(60, 20)).Post("/jobs/retention", a.handleRunRetention)

	return r
}

func buildAllowedOrigins(webBaseURL string) []string {
	allowed := []string{
		"http://localhost:3000",
		"http://127.0.0.1:3000",
		"https://triageguard.com",
		"https://www.triageguard.com",
	}
	if strings.TrimSpace(webBaseURL) != "" {
		if parsed, err := url.Parse(webBaseURL); err == nil && parsed.Scheme != "" && parsed.Host != "" {
			allowed = append(allowed, parsed.Scheme+"://"+parsed.Host)
		} else {
			allowed = append(allowed, strings.TrimSpace(webBaseURL))
		}
	}
	return allowed
}

func (a *App) SlackClient() *slack.Client {
	return a.slackClient
}

func (a *App) LinearClient() *linear.Client {
	return a.linearClient
}

func (a *App) Store() *db.Store {
	return a.store
}

func jsonResponse(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func readJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return fmt.Errorf("request body is required")
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxJSONBodyBytes+1))
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return fmt.Errorf("request body is required")
	}
	if len(body) > maxJSONBodyBytes {
		return fmt.Errorf("request body too large (max %d bytes)", maxJSONBodyBytes)
	}

	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("request body must contain a single JSON object")
	}
	return nil
}

func optionalString(v string) *string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	x := strings.TrimSpace(v)
	return &x
}

func (a *App) workspaceFromContext(r *http.Request) (db.Workspace, error) {
	userID, err := userIDFromContext(r.Context())
	if err != nil {
		return db.Workspace{}, err
	}

	var preferredWorkspaceID *uuid.UUID
	if rawWorkspaceID := strings.TrimSpace(r.Header.Get("X-Workspace-ID")); rawWorkspaceID != "" {
		parsed, err := uuid.Parse(rawWorkspaceID)
		if err != nil {
			return db.Workspace{}, errors.New("invalid X-Workspace-ID header")
		}
		preferredWorkspaceID = &parsed
	}

	w, err := a.store.GetWorkspaceForUser(r.Context(), userID, preferredWorkspaceID)
	if err == nil {
		return w, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return db.Workspace{}, err
	}

	// Legacy bootstrap path: allow initial owner claim only if DB has exactly one workspace
	// and no memberships yet (single-tenant migration safety).
	claimed, claimErr := a.store.BootstrapSingleWorkspaceMembership(r.Context(), userID)
	if claimErr == nil {
		return claimed, nil
	}
	if !errors.Is(claimErr, pgx.ErrNoRows) {
		return db.Workspace{}, claimErr
	}

	return a.store.ProvisionWorkspaceForUser(r.Context(), userID)
}

func (a *App) workspaceHasPaidAccess(ctx context.Context, workspaceID uuid.UUID) (bool, string, string, error) {
	sub, err := a.store.GetWorkspaceSubscriptionOrDefault(ctx, workspaceID)
	if err != nil {
		return false, "", "", err
	}
	status := strings.ToLower(strings.TrimSpace(sub.Status))
	plan := effectivePlan(sub)
	return hasPaidWorkspaceAccess(sub), plan, status, nil
}

func (a *App) requireActivePaidSubscription() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			workspace, err := a.workspaceFromContext(r)
			if err != nil {
				jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
				return
			}
			allowed, plan, status, err := a.workspaceHasPaidAccess(r.Context(), workspace.ID)
			if err != nil {
				jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
				return
			}
			if !allowed {
				jsonResponse(w, http.StatusPaymentRequired, map[string]any{
					"error":          "active paid subscription required. Open Billing and activate Team or Enterprise plan.",
					"effective_plan": plan,
					"status":         status,
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func userIDFromContext(ctx context.Context) (uuid.UUID, error) {
	claims, ok := ctx.Value(authmw.ClaimsKey).(jwt.MapClaims)
	if !ok || claims == nil {
		return uuid.Nil, errors.New("missing auth claims")
	}
	sub, _ := claims["sub"].(string)
	sub = strings.TrimSpace(sub)
	if sub == "" {
		return uuid.Nil, errors.New("missing token subject")
	}
	userID, err := uuid.Parse(sub)
	if err != nil {
		return uuid.Nil, errors.New("invalid token subject")
	}
	return userID, nil
}

func parseUUID(v string) (uuid.UUID, error) {
	u, err := uuid.Parse(v)
	if err != nil {
		return uuid.Nil, errors.New("invalid request id")
	}
	return u, nil
}

func (a *App) handleRunSLA(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(a.cfg.JobsSecret) == "" {
		http.NotFound(w, r)
		return
	}
	provided := strings.TrimSpace(r.Header.Get("X-Jobs-Secret"))
	if provided == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(a.cfg.JobsSecret)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	runner := jobs.NewSLARunner(a.cfg, a.store, a.slackClient, a.linearClient)
	if err := runner.RunOnce(r.Context()); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) onServerError(event authmw.ServerErrorEvent) {
	if a == nil {
		return
	}

	if a.alerts != nil {
		a.alerts.Record5xx(event.Method, event.Path, event.RequestID, event.Status)
	}
	if a.sentry != nil && a.sentry.Enabled() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = a.sentry.Capture(ctx, "error", "api returned 5xx", map[string]string{
			"component": "http",
		}, map[string]any{
			"method":      event.Method,
			"path":        event.Path,
			"status":      event.Status,
			"request_id":  event.RequestID,
			"duration_ms": event.Duration.Milliseconds(),
		})
	}
}

func (a *App) handleRunRetention(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(a.cfg.JobsSecret) == "" {
		http.NotFound(w, r)
		return
	}
	provided := strings.TrimSpace(r.Header.Get("X-Jobs-Secret"))
	if provided == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(a.cfg.JobsSecret)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	runner := jobs.NewRetentionRunner(a.cfg, a.store)
	if err := runner.RunOnce(r.Context()); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true})
}
