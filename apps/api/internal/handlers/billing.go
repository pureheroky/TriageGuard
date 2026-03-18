package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"triageguard/apps/api/internal/db"
	stripelib "triageguard/apps/api/internal/stripe"
)

const (
	planTeam       = "team"
	planEnterprise = "enterprise"
	billingStripe  = "stripe"
	billingPayPal  = "paypal"
)

func normalizeBillingProvider(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case billingPayPal:
		return billingPayPal
	default:
		return billingStripe
	}
}

func normalizePlanKey(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case planTeam:
		return planTeam
	case planEnterprise:
		return planEnterprise
	case "pro":
		return planEnterprise
	case "starter":
		return planTeam
	default:
		return planTeam
	}
}

func isPaidSubscriptionStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active", "trialing", "past_due", "unpaid", "incomplete":
		return true
	default:
		return false
	}
}

func effectivePlan(sub db.WorkspaceSubscription) string {
	return normalizePlanKey(sub.PlanKey)
}

func hasPaidWorkspaceAccess(sub db.WorkspaceSubscription) bool {
	plan := effectivePlan(sub)
	return (plan == planTeam || plan == planEnterprise) && isPaidSubscriptionStatus(sub.Status)
}

func channelLimitForPlan(plan string) *int {
	normalized := normalizePlanKey(plan)
	if normalized == planTeam {
		limit := 10
		return &limit
	}
	return nil
}

type billingClientReference struct {
	WorkspaceID uuid.UUID
	PlanKey     string
	IssuedAt    int64
}

func (a *App) buildBillingClientReference(workspaceID uuid.UUID, planKey string) string {
	planKey = normalizePlanKey(planKey)
	issuedAt := time.Now().UTC().Unix()
	base := fmt.Sprintf("%s|%s|%d", workspaceID.String(), planKey, issuedAt)
	mac := hmac.New(sha256.New, []byte(a.cfg.BillingSignSecret))
	_, _ = mac.Write([]byte(base))
	signature := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("v1_%s_%s_%d_%s", workspaceID.String(), planKey, issuedAt, signature)
}

func (a *App) parseBillingClientReference(raw string) (billingClientReference, error) {
	parts := strings.Split(strings.TrimSpace(raw), "_")
	if len(parts) != 5 || parts[0] != "v1" {
		return billingClientReference{}, errors.New("invalid client_reference_id")
	}

	workspaceID, err := uuid.Parse(parts[1])
	if err != nil {
		return billingClientReference{}, errors.New("invalid workspace in client_reference_id")
	}
	planKey := normalizePlanKey(parts[2])
	if planKey != planTeam && planKey != planEnterprise {
		return billingClientReference{}, errors.New("invalid plan in client_reference_id")
	}

	issuedAt, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return billingClientReference{}, errors.New("invalid timestamp in client_reference_id")
	}
	now := time.Now().UTC().Unix()
	if issuedAt > now+300 || now-issuedAt > int64(14*24*time.Hour.Seconds()) {
		return billingClientReference{}, errors.New("expired client_reference_id")
	}

	base := fmt.Sprintf("%s|%s|%d", workspaceID.String(), planKey, issuedAt)
	mac := hmac.New(sha256.New, []byte(a.cfg.BillingSignSecret))
	_, _ = mac.Write([]byte(base))
	expected := hex.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(parts[4]), []byte(expected)) != 1 {
		return billingClientReference{}, errors.New("invalid client_reference_id signature")
	}
	return billingClientReference{
		WorkspaceID: workspaceID,
		PlanKey:     planKey,
		IssuedAt:    issuedAt,
	}, nil
}

func (a *App) inferPlanFromPriceID(priceID string, fallback string) string {
	priceID = strings.TrimSpace(priceID)
	if priceID == "" {
		return normalizePlanKey(fallback)
	}
	if strings.TrimSpace(a.cfg.StripeTeamPriceID) != "" && priceID == strings.TrimSpace(a.cfg.StripeTeamPriceID) {
		return planTeam
	}
	if strings.TrimSpace(a.cfg.StripeEnterprisePriceID) != "" && priceID == strings.TrimSpace(a.cfg.StripeEnterprisePriceID) {
		return planEnterprise
	}
	return normalizePlanKey(fallback)
}

func (a *App) stripeEnabled() bool {
	return a.stripeClient != nil && a.stripeClient.Enabled()
}

func (a *App) billingProvider() string {
	return normalizeBillingProvider(a.cfg.BillingProvider)
}

func (a *App) billingEnabled() bool {
	if a.billingProvider() == billingPayPal {
		return strings.TrimSpace(a.cfg.PayPalTeamPaymentLink) != "" || strings.TrimSpace(a.cfg.PayPalEnterprisePaymentLink) != ""
	}
	return a.stripeEnabled()
}

func (a *App) teamCheckoutLink() string {
	if a.billingProvider() == billingPayPal {
		return strings.TrimSpace(a.cfg.PayPalTeamPaymentLink)
	}
	return strings.TrimSpace(a.cfg.StripeTeamPaymentLink)
}

func (a *App) enterpriseCheckoutLink() string {
	if a.billingProvider() == billingPayPal {
		return strings.TrimSpace(a.cfg.PayPalEnterprisePaymentLink)
	}
	return strings.TrimSpace(a.cfg.StripeEnterprisePaymentLink)
}

func (a *App) handleGetBilling(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	provider := a.billingProvider()
	secretSet := strings.TrimSpace(a.cfg.StripeSecretKey) != ""
	teamLinkSet := a.teamCheckoutLink() != ""
	enterpriseLinkSet := a.enterpriseCheckoutLink() != ""
	webhookSet := strings.TrimSpace(a.cfg.StripeWebhookSecret) != ""
	portalReturnSet := strings.TrimSpace(a.cfg.StripePortalReturnURL) != ""
	payPalEmailSet := strings.TrimSpace(a.cfg.PayPalBusinessEmail) != ""

	sub, err := a.store.GetWorkspaceSubscriptionOrDefault(r.Context(), workspace.ID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	subscriptionProvider := normalizeBillingProvider(sub.BillingProvider)
	providerMismatch := hasPaidWorkspaceAccess(sub) && subscriptionProvider != provider
	effPlan := effectivePlan(sub)
	limit := channelLimitForPlan(effPlan)
	channels, _ := a.store.ListChannels(r.Context(), workspace.ID)
	enabledCount := 0
	for _, channel := range channels {
		if channel.Enabled {
			enabledCount++
		}
	}

	jsonResponse(w, http.StatusOK, map[string]any{
		"enabled":                a.billingEnabled(),
		"provider":               provider,
		"subscription_provider":  subscriptionProvider,
		"provider_mismatch":      providerMismatch,
		"plan_key":               normalizePlanKey(sub.PlanKey),
		"status":                 strings.ToLower(strings.TrimSpace(sub.Status)),
		"effective_plan":         effPlan,
		"cancel_at_period_end":   sub.CancelAtPeriodEnd,
		"current_period_end":     sub.CurrentPeriodEnd,
		"channel_limit":          limit,
		"channels_enabled":       enabledCount,
		"paypal_payer_email":     sub.PayPalPayerEmail,
		"paypal_last_payment_at": sub.PayPalLastPaymentAt,
		"portal_available":       provider == billingStripe && sub.StripeCustomerID != nil && strings.TrimSpace(*sub.StripeCustomerID) != "",
		"checkout": map[string]any{
			"team_available":       teamLinkSet,
			"enterprise_available": enterpriseLinkSet,
		},
		"config": map[string]any{
			"billing_provider":           provider,
			"stripe_secret_key_set":      secretSet,
			"stripe_webhook_secret_set":  webhookSet,
			"stripe_team_link_set":       strings.TrimSpace(a.cfg.StripeTeamPaymentLink) != "",
			"stripe_enterprise_link_set": strings.TrimSpace(a.cfg.StripeEnterprisePaymentLink) != "",
			"stripe_portal_return_set":   portalReturnSet,
			"paypal_team_link_set":       strings.TrimSpace(a.cfg.PayPalTeamPaymentLink) != "",
			"paypal_enterprise_link_set": strings.TrimSpace(a.cfg.PayPalEnterprisePaymentLink) != "",
			"paypal_email_set":           payPalEmailSet,
		},
	})
}

func (a *App) handleCreateBillingCheckout(w http.ResponseWriter, r *http.Request) {
	if !a.billingEnabled() {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "billing is not configured"})
		return
	}
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}

	var payload struct {
		PlanKey string `json:"plan_key"`
	}
	if err := readJSON(r, &payload); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	planKey := normalizePlanKey(payload.PlanKey)
	if planKey != planTeam && planKey != planEnterprise {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "plan_key must be team or enterprise"})
		return
	}

	paymentLink := ""
	if planKey == planTeam {
		paymentLink = a.teamCheckoutLink()
	} else {
		paymentLink = a.enterpriseCheckoutLink()
	}
	if paymentLink == "" {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "checkout link is not configured for selected plan"})
		return
	}

	linkURL, err := url.Parse(paymentLink)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": "invalid payment link configuration"})
		return
	}
	reference := a.buildBillingClientReference(workspace.ID, planKey)
	if a.billingProvider() == billingStripe {
		query := linkURL.Query()
		query.Set("client_reference_id", reference)
		linkURL.RawQuery = query.Encode()
	} else {
		// Keep a signed reference in query for manual support reconciliation in PayPal mode.
		query := linkURL.Query()
		query.Set("tg_ref", reference)
		linkURL.RawQuery = query.Encode()
	}
	jsonResponse(w, http.StatusOK, map[string]any{
		"url":      linkURL.String(),
		"plan_key": planKey,
		"provider": a.billingProvider(),
	})
}

func (a *App) handleConfirmPayPalBilling(w http.ResponseWriter, r *http.Request) {
	if a.billingProvider() != billingPayPal {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "paypal confirmation is available only in paypal billing mode"})
		return
	}
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}

	var payload struct {
		PlanKey          string `json:"plan_key"`
		PayerEmail       string `json:"payer_email"`
		CurrentPeriodEnd string `json:"current_period_end"`
	}
	if err := readJSON(r, &payload); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	planKey := normalizePlanKey(payload.PlanKey)
	if planKey != planTeam && planKey != planEnterprise {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "paypal confirmation supports only team or enterprise plan"})
		return
	}

	now := time.Now().UTC()
	periodEnd := now.AddDate(0, 1, 0)
	if raw := strings.TrimSpace(payload.CurrentPeriodEnd); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "current_period_end must be RFC3339 datetime"})
			return
		}
		periodEnd = parsed.UTC()
	}
	if periodEnd.Before(now.Add(5 * time.Minute)) {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "current_period_end must be in the future"})
		return
	}

	sub, err := a.store.GetWorkspaceSubscriptionOrDefault(r.Context(), workspace.ID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	sub.PlanKey = planKey
	sub.Status = "active"
	sub.BillingProvider = billingPayPal
	sub.PayPalPayerEmail = optionalString(payload.PayerEmail)
	sub.CancelAtPeriodEnd = false
	sub.CurrentPeriodEnd = &periodEnd
	sub.PayPalLastPaymentAt = &now
	if err := a.store.UpsertWorkspaceSubscription(r.Context(), sub); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	jsonResponse(w, http.StatusOK, map[string]any{
		"ok":                 true,
		"plan_key":           sub.PlanKey,
		"status":             sub.Status,
		"billing_provider":   sub.BillingProvider,
		"current_period_end": sub.CurrentPeriodEnd,
	})
}

func (a *App) handleCreateBillingPortal(w http.ResponseWriter, r *http.Request) {
	if !a.billingEnabled() {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "billing is not configured"})
		return
	}
	if a.billingProvider() == billingPayPal {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "billing portal is unavailable for PayPal"})
		return
	}
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}

	sub, err := a.store.GetWorkspaceSubscriptionOrDefault(r.Context(), workspace.ID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if sub.StripeCustomerID == nil || strings.TrimSpace(*sub.StripeCustomerID) == "" {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "no stripe customer found for this workspace"})
		return
	}
	portalURL, err := a.stripeClient.CreateBillingPortalSession(r.Context(), strings.TrimSpace(*sub.StripeCustomerID), a.cfg.StripePortalReturnURL)
	if err != nil {
		jsonResponse(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"url": portalURL})
}

func verifyStripeWebhookSignature(body []byte, headerValue, webhookSecret string) error {
	timestamp, signatures, err := stripelib.ParseStripeSignatureHeader(headerValue)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Unix()
	if timestamp > now+300 || now-timestamp > 300 {
		return errors.New("stripe signature timestamp outside tolerance")
	}

	payloadToSign := strconv.FormatInt(timestamp, 10) + "." + string(body)
	mac := hmac.New(sha256.New, []byte(strings.TrimSpace(webhookSecret)))
	_, _ = mac.Write([]byte(payloadToSign))
	expected := hex.EncodeToString(mac.Sum(nil))

	for _, signature := range signatures {
		if subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) == 1 {
			return nil
		}
	}
	return errors.New("stripe signature mismatch")
}

func (a *App) handleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(a.cfg.StripeWebhookSecret) == "" {
		http.NotFound(w, r)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if err := verifyStripeWebhookSignature(body, r.Header.Get("Stripe-Signature"), a.cfg.StripeWebhookSecret); err != nil {
		http.Error(w, "invalid stripe signature", http.StatusUnauthorized)
		return
	}

	var event struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Data struct {
			Object json.RawMessage `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "invalid stripe event", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(event.ID) != "" {
		recorded, err := a.store.TryRecordStripeWebhookEvent(r.Context(), event.ID, event.Type)
		if err != nil {
			a.logger.Printf("stripe webhook dedupe insert failed event_id=%s: %v", event.ID, err)
			http.Error(w, "stripe webhook dedupe failed", http.StatusInternalServerError)
			return
		}
		if !recorded {
			jsonResponse(w, http.StatusOK, map[string]any{"received": true, "duplicate": true})
			return
		}
	}

	switch strings.TrimSpace(event.Type) {
	case "checkout.session.completed":
		a.handleStripeCheckoutCompleted(r.Context(), event.Data.Object)
	case "customer.subscription.created", "customer.subscription.updated", "customer.subscription.deleted":
		a.handleStripeSubscriptionChanged(r.Context(), event.Type, event.Data.Object)
	default:
		// Ignore unsupported events.
	}
	jsonResponse(w, http.StatusOK, map[string]any{"received": true})
}

func (a *App) handleStripeCheckoutCompleted(ctx context.Context, rawObject json.RawMessage) {
	var session struct {
		ID                string `json:"id"`
		Mode              string `json:"mode"`
		Customer          string `json:"customer"`
		Subscription      string `json:"subscription"`
		ClientReferenceID string `json:"client_reference_id"`
	}
	if err := json.Unmarshal(rawObject, &session); err != nil {
		a.logger.Printf("stripe webhook checkout completed: decode failed: %v", err)
		return
	}
	if strings.TrimSpace(session.Mode) != "subscription" {
		return
	}
	reference, err := a.parseBillingClientReference(session.ClientReferenceID)
	if err != nil {
		a.logger.Printf("stripe webhook checkout completed: invalid client_reference_id: %v", err)
		return
	}

	subRecord := db.WorkspaceSubscription{
		WorkspaceID:          reference.WorkspaceID,
		PlanKey:              reference.PlanKey,
		Status:               "active",
		BillingProvider:      billingStripe,
		StripeCustomerID:     optionalString(session.Customer),
		StripeSubscriptionID: optionalString(session.Subscription),
		StripeCheckoutID:     optionalString(session.ID),
		CancelAtPeriodEnd:    false,
	}

	if strings.TrimSpace(session.Subscription) != "" && a.stripeEnabled() {
		stripeSubscription, err := a.stripeClient.GetSubscription(ctx, session.Subscription)
		if err != nil {
			a.logger.Printf("stripe webhook checkout completed: get subscription failed id=%s: %v", session.Subscription, err)
		} else {
			subRecord.Status = strings.ToLower(strings.TrimSpace(stripeSubscription.Status))
			subRecord.CancelAtPeriodEnd = stripeSubscription.CancelAtPeriodEnd
			subRecord.CurrentPeriodEnd = stripeSubscription.CurrentPeriodEnd
			subRecord.StripeCustomerID = optionalString(stripeSubscription.CustomerID)
			subRecord.PlanKey = a.inferPlanFromPriceID(stripeSubscription.PriceID, subRecord.PlanKey)
		}
	}

	if err := a.store.UpsertWorkspaceSubscription(ctx, subRecord); err != nil {
		a.logger.Printf("stripe webhook checkout completed: upsert failed workspace=%s: %v", reference.WorkspaceID, err)
	}
}

func (a *App) handleStripeSubscriptionChanged(ctx context.Context, eventType string, rawObject json.RawMessage) {
	var subscription struct {
		ID                string `json:"id"`
		Customer          string `json:"customer"`
		Status            string `json:"status"`
		CancelAtPeriodEnd bool   `json:"cancel_at_period_end"`
		CurrentPeriodEnd  int64  `json:"current_period_end"`
		Items             struct {
			Data []struct {
				Price struct {
					ID string `json:"id"`
				} `json:"price"`
			} `json:"data"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rawObject, &subscription); err != nil {
		a.logger.Printf("stripe webhook subscription change: decode failed: %v", err)
		return
	}

	subRecord, err := a.store.FindWorkspaceSubscriptionByStripeSubscriptionID(ctx, subscription.ID)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			a.logger.Printf("stripe webhook subscription change: lookup by subscription failed: %v", err)
			return
		}
		subRecord, err = a.store.FindWorkspaceSubscriptionByStripeCustomerID(ctx, subscription.Customer)
		if err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				a.logger.Printf("stripe webhook subscription change: lookup by customer failed: %v", err)
			}
			return
		}
	}

	subRecord.StripeCustomerID = optionalString(subscription.Customer)
	subRecord.StripeSubscriptionID = optionalString(subscription.ID)
	subRecord.Status = strings.ToLower(strings.TrimSpace(subscription.Status))
	subRecord.BillingProvider = billingStripe
	if strings.TrimSpace(eventType) == "customer.subscription.deleted" {
		subRecord.Status = "canceled"
	}
	subRecord.CancelAtPeriodEnd = subscription.CancelAtPeriodEnd
	if subscription.CurrentPeriodEnd > 0 {
		t := time.Unix(subscription.CurrentPeriodEnd, 0).UTC()
		subRecord.CurrentPeriodEnd = &t
	}
	if len(subscription.Items.Data) > 0 {
		subRecord.PlanKey = a.inferPlanFromPriceID(subscription.Items.Data[0].Price.ID, subRecord.PlanKey)
	}

	if err := a.store.UpsertWorkspaceSubscription(ctx, subRecord); err != nil {
		a.logger.Printf("stripe webhook subscription change: upsert failed workspace=%s: %v", subRecord.WorkspaceID, err)
	}
}
