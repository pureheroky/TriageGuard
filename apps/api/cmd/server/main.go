package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"triageguard/apps/api/internal/config"
	"triageguard/apps/api/internal/db"
	"triageguard/apps/api/internal/handlers"
	"triageguard/apps/api/internal/jobs"
	"triageguard/apps/api/internal/secret"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config load: %v", err)
	}

	dbHost := db.DBHostFromDSN(cfg.SupabaseDBURL)
	log.Printf(
		"boot: api_base=%s web_base=%s port=%s db_host=%s billing_provider=%s stripe_enabled=%t stripe_team_link_set=%t stripe_enterprise_link_set=%t stripe_webhook_set=%t paypal_team_link_set=%t paypal_enterprise_link_set=%t linear_webhook_set=%t retention_enabled=%t retention_terminal_days=%d retention_dedupe_days=%d retention_batch_size=%d retention_interval_minutes=%d sentry_enabled=%t alert_webhook_set=%t alert_5xx_threshold=%d alert_5xx_window_minutes=%d alert_dead_letter_threshold=%d alert_dead_letter_window_minutes=%d",
		cfg.APIBaseURL,
		cfg.WebBaseURL,
		cfg.Port,
		dbHost,
		cfg.BillingProvider,
		strings.TrimSpace(cfg.StripeSecretKey) != "",
		strings.TrimSpace(cfg.StripeTeamPaymentLink) != "",
		strings.TrimSpace(cfg.StripeEnterprisePaymentLink) != "",
		strings.TrimSpace(cfg.StripeWebhookSecret) != "",
		strings.TrimSpace(cfg.PayPalTeamPaymentLink) != "",
		strings.TrimSpace(cfg.PayPalEnterprisePaymentLink) != "",
		strings.TrimSpace(cfg.LinearWebhookSecret) != "",
		cfg.RetentionEnabled,
		cfg.RetentionTerminalDays,
		cfg.RetentionDedupeDays,
		cfg.RetentionBatchSize,
		cfg.RetentionIntervalMinutes,
		strings.TrimSpace(cfg.SentryDSN) != "",
		strings.TrimSpace(cfg.AlertWebhookURL) != "",
		cfg.AlertFiveXXThreshold,
		cfg.AlertFiveXXWindowMinutes,
		cfg.AlertDeadLetterThreshold,
		cfg.AlertDeadLetterWindowMinutes,
	)
	if strings.HasPrefix(strings.ToLower(dbHost), "db.") && strings.HasSuffix(strings.ToLower(dbHost), ".supabase.co") {
		log.Printf("boot warning: SUPABASE_DB_URL uses direct db host (%s). Prefer Session Pooler (*.pooler.supabase.com), especially if your network has no IPv6", dbHost)
	}

	pool, err := db.NewPool(context.Background(), cfg.SupabaseDBURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	tokenCipher, err := secret.NewTokenCipher(cfg.TokenEncryptionKey)
	if err != nil {
		log.Fatalf("token cipher init: %v", err)
	}

	app := handlers.NewApp(cfg, pool, tokenCipher)
	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           app.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	slaRunner := jobs.NewSLARunner(cfg, app.Store(), app.SlackClient(), app.LinearClient())
	outboxDispatcher := jobs.NewNotificationDispatcher(cfg, app.Store(), app.SlackClient(), app.LinearClient())
	retentionRunner := jobs.NewRetentionRunner(cfg, app.Store())
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	go slaRunner.Run(ctx, time.Minute)
	go outboxDispatcher.Run(ctx, 15*time.Second)
	go retentionRunner.Run(ctx, time.Duration(cfg.RetentionIntervalMinutes)*time.Minute)

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer shutdownCancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	log.Printf("api listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("listen: %v", err)
	}
}
