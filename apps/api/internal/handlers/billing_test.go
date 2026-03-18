package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"triageguard/apps/api/internal/config"
	"triageguard/apps/api/internal/db"
)

func TestBillingClientReferenceRoundTrip(t *testing.T) {
	app := &App{cfg: config.Config{BillingSignSecret: "test-billing-secret"}}
	workspaceID := uuid.New()

	ref := app.buildBillingClientReference(workspaceID, "TEAM")
	parsed, err := app.parseBillingClientReference(ref)
	if err != nil {
		t.Fatalf("parseBillingClientReference returned error: %v", err)
	}
	if parsed.WorkspaceID != workspaceID {
		t.Fatalf("workspace mismatch: got %s want %s", parsed.WorkspaceID, workspaceID)
	}
	if parsed.PlanKey != planTeam {
		t.Fatalf("plan mismatch: got %s want %s", parsed.PlanKey, planTeam)
	}
	if parsed.IssuedAt <= 0 {
		t.Fatalf("issuedAt must be set")
	}
}

func TestBillingClientReferenceRejectsTampering(t *testing.T) {
	app := &App{cfg: config.Config{BillingSignSecret: "test-billing-secret"}}
	ref := app.buildBillingClientReference(uuid.New(), "enterprise")

	parts := strings.Split(ref, "_")
	if len(parts) != 5 {
		t.Fatalf("unexpected reference format: %s", ref)
	}
	parts[4] = strings.Repeat("0", len(parts[4]))
	_, err := app.parseBillingClientReference(strings.Join(parts, "_"))
	if err == nil {
		t.Fatalf("expected signature validation error")
	}
}

func TestBillingClientReferenceRejectsExpiredTimestamp(t *testing.T) {
	app := &App{cfg: config.Config{BillingSignSecret: "test-billing-secret"}}
	workspaceID := uuid.New()
	issuedAt := time.Now().UTC().Add(-15 * 24 * time.Hour).Unix()
	base := fmt.Sprintf("%s|%s|%d", workspaceID.String(), planTeam, issuedAt)
	mac := hmac.New(sha256.New, []byte(app.cfg.BillingSignSecret))
	_, _ = mac.Write([]byte(base))
	sig := hex.EncodeToString(mac.Sum(nil))
	ref := fmt.Sprintf("v1_%s_%s_%d_%s", workspaceID.String(), planTeam, issuedAt, sig)

	_, err := app.parseBillingClientReference(ref)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "expired") {
		t.Fatalf("expected expired error, got: %v", err)
	}
}

func TestBillingPlanHelpers(t *testing.T) {
	if got := normalizePlanKey("TEAM"); got != planTeam {
		t.Fatalf("normalizePlanKey TEAM = %s", got)
	}
	if got := normalizePlanKey("unknown"); got != planTeam {
		t.Fatalf("normalizePlanKey fallback = %s", got)
	}
	if !isPaidSubscriptionStatus("active") {
		t.Fatalf("active should be paid status")
	}
	if isPaidSubscriptionStatus("canceled") {
		t.Fatalf("canceled should not be paid status")
	}

	sub := db.WorkspaceSubscription{PlanKey: planEnterprise, Status: "active"}
	if got := effectivePlan(sub); got != planEnterprise {
		t.Fatalf("effectivePlan active paid = %s", got)
	}
	sub.Status = "canceled"
	if got := effectivePlan(sub); got != planEnterprise {
		t.Fatalf("effectivePlan canceled paid = %s", got)
	}

	if limit := channelLimitForPlan(planTeam); limit == nil || *limit != 10 {
		t.Fatalf("team channel limit must be 10")
	}
	if limit := channelLimitForPlan(planEnterprise); limit != nil {
		t.Fatalf("enterprise channel limit must be unlimited")
	}
}

func TestBillingProviderHelpers(t *testing.T) {
	if got := normalizeBillingProvider("PAYPAL"); got != billingPayPal {
		t.Fatalf("normalizeBillingProvider paypal = %s", got)
	}
	if got := normalizeBillingProvider("unknown"); got != billingStripe {
		t.Fatalf("normalizeBillingProvider fallback = %s", got)
	}

	app := &App{cfg: config.Config{BillingProvider: "paypal", PayPalTeamPaymentLink: "https://paypal.example/checkout"}}
	if got := app.billingProvider(); got != billingPayPal {
		t.Fatalf("billingProvider paypal = %s", got)
	}
	if !app.billingEnabled() {
		t.Fatalf("billingEnabled should be true for paypal with team link")
	}
	if got := app.teamCheckoutLink(); got != "https://paypal.example/checkout" {
		t.Fatalf("teamCheckoutLink paypal = %s", got)
	}
	if got := app.enterpriseCheckoutLink(); got != "" {
		t.Fatalf("enterpriseCheckoutLink paypal default = %s", got)
	}

	app = &App{cfg: config.Config{BillingProvider: "unknown"}, stripeClient: nil}
	if got := app.billingProvider(); got != billingStripe {
		t.Fatalf("billingProvider fallback = %s", got)
	}
	if app.billingEnabled() {
		t.Fatalf("billingEnabled must be false without stripe client")
	}
}

func TestVerifyStripeWebhookSignature(t *testing.T) {
	secret := "whsec_test_secret"
	timestamp := time.Now().UTC().Unix()
	payload := map[string]any{
		"id":   "evt_test_123",
		"type": "checkout.session.completed",
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	toSign := strconv.FormatInt(timestamp, 10) + "." + string(raw)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(toSign))
	signature := hex.EncodeToString(mac.Sum(nil))
	header := fmt.Sprintf("t=%d,v1=%s", timestamp, signature)

	if err := verifyStripeWebhookSignature(raw, header, secret); err != nil {
		t.Fatalf("expected signature to validate, got: %v", err)
	}
	if err := verifyStripeWebhookSignature(raw, header, "wrong-secret"); err == nil {
		t.Fatalf("expected signature mismatch for wrong secret")
	}
}
