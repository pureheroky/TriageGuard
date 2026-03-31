package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	APIBaseURL                   string
	WebBaseURL                   string
	InternalOperatorUserIDs      []string
	SentryDSN                    string
	AlertWebhookURL              string
	AlertFiveXXThreshold         int
	AlertFiveXXWindowMinutes     int
	AlertFiveXXCooldownMinutes   int
	AlertDeadLetterThreshold     int
	AlertDeadLetterWindowMinutes int
	SupabaseURL                  string
	SupabaseDBURL                string
	SupabaseJWTSecret            string
	OAuthStateSecret             string
	BillingSignSecret            string
	BillingProvider              string
	TokenEncryptionKey           string
	JobsSecret                   string
	StripeSecretKey              string
	StripeWebhookSecret          string
	StripeTeamPaymentLink        string
	StripeEnterprisePaymentLink  string
	StripeTeamPriceID            string
	StripeEnterprisePriceID      string
	StripePortalReturnURL        string
	PayPalTeamPaymentLink        string
	PayPalEnterprisePaymentLink  string
	PayPalBusinessEmail          string
	SlackClientID                string
	SlackClientSecret            string
	SlackSigningSecret           string
	LinearClientID               string
	LinearClientSecret           string
	LinearRedirectURI            string
	LinearWebhookSecret          string
	RetentionEnabled             bool
	RetentionTerminalDays        int
	RetentionDedupeDays          int
	RetentionBatchSize           int
	RetentionIntervalMinutes     int
	Port                         string
}

func Load() (Config, error) {
	// Best-effort load of local development env file.
	// Process-level env vars still take precedence.
	_ = godotenv.Load(".env")

	cfg := Config{
		APIBaseURL:                   envOr("API_BASE_URL", "http://localhost:8080"),
		WebBaseURL:                   envOr("WEB_BASE_URL", "http://localhost:3000"),
		InternalOperatorUserIDs:      envCSV("INTERNAL_OPERATOR_USER_IDS"),
		SentryDSN:                    os.Getenv("SENTRY_DSN"),
		AlertWebhookURL:              os.Getenv("ALERT_WEBHOOK_URL"),
		AlertFiveXXThreshold:         envIntOr("ALERT_5XX_THRESHOLD", 20),
		AlertFiveXXWindowMinutes:     envIntOr("ALERT_5XX_WINDOW_MINUTES", 5),
		AlertFiveXXCooldownMinutes:   envIntOr("ALERT_5XX_COOLDOWN_MINUTES", 20),
		AlertDeadLetterThreshold:     envIntOr("ALERT_DEAD_LETTER_THRESHOLD", 3),
		AlertDeadLetterWindowMinutes: envIntOr("ALERT_DEAD_LETTER_WINDOW_MINUTES", 10),
		SupabaseURL:                  os.Getenv("SUPABASE_URL"),
		SupabaseDBURL:                os.Getenv("SUPABASE_DB_URL"),
		SupabaseJWTSecret:            os.Getenv("SUPABASE_JWT_SECRET"),
		OAuthStateSecret:             os.Getenv("OAUTH_STATE_SECRET"),
		BillingSignSecret:            os.Getenv("BILLING_SIGNING_SECRET"),
		BillingProvider:              strings.ToLower(strings.TrimSpace(envOr("BILLING_PROVIDER", "stripe"))),
		TokenEncryptionKey:           os.Getenv("TOKENS_ENCRYPTION_KEY"),
		JobsSecret:                   os.Getenv("JOBS_SECRET"),
		StripeSecretKey:              os.Getenv("STRIPE_SECRET_KEY"),
		StripeWebhookSecret:          os.Getenv("STRIPE_WEBHOOK_SECRET"),
		StripeTeamPaymentLink:        os.Getenv("STRIPE_TEAM_PAYMENT_LINK"),
		StripeEnterprisePaymentLink:  os.Getenv("STRIPE_ENTERPRISE_PAYMENT_LINK"),
		StripeTeamPriceID:            os.Getenv("STRIPE_TEAM_PRICE_ID"),
		StripeEnterprisePriceID:      os.Getenv("STRIPE_ENTERPRISE_PRICE_ID"),
		StripePortalReturnURL:        os.Getenv("STRIPE_PORTAL_RETURN_URL"),
		PayPalTeamPaymentLink:        os.Getenv("PAYPAL_TEAM_PAYMENT_LINK"),
		PayPalEnterprisePaymentLink:  os.Getenv("PAYPAL_ENTERPRISE_PAYMENT_LINK"),
		PayPalBusinessEmail:          os.Getenv("PAYPAL_BUSINESS_EMAIL"),
		SlackClientID:                os.Getenv("SLACK_CLIENT_ID"),
		SlackClientSecret:            os.Getenv("SLACK_CLIENT_SECRET"),
		SlackSigningSecret:           os.Getenv("SLACK_SIGNING_SECRET"),
		LinearClientID:               os.Getenv("LINEAR_CLIENT_ID"),
		LinearClientSecret:           os.Getenv("LINEAR_CLIENT_SECRET"),
		LinearRedirectURI:            envOr("LINEAR_REDIRECT_URI", "http://localhost:8080/linear/oauth/callback"),
		LinearWebhookSecret:          os.Getenv("LINEAR_WEBHOOK_SECRET"),
		RetentionEnabled:             envBoolOr("RETENTION_ENABLED", true),
		RetentionTerminalDays:        envIntOr("RETENTION_TERMINAL_DAYS", 180),
		RetentionDedupeDays:          envIntOr("RETENTION_DEDUPE_DAYS", 30),
		RetentionBatchSize:           envIntOr("RETENTION_BATCH_SIZE", 500),
		RetentionIntervalMinutes:     envIntOr("RETENTION_INTERVAL_MINUTES", 60),
		Port:                         envOr("PORT", "8080"),
	}
	if cfg.SupabaseDBURL == "" {
		return Config{}, fmt.Errorf("SUPABASE_DB_URL is required")
	}
	if cfg.OAuthStateSecret == "" {
		cfg.OAuthStateSecret = cfg.SlackSigningSecret
	}
	if cfg.OAuthStateSecret == "" {
		return Config{}, fmt.Errorf("OAUTH_STATE_SECRET (or SLACK_SIGNING_SECRET) is required")
	}
	if cfg.BillingSignSecret == "" {
		cfg.BillingSignSecret = cfg.OAuthStateSecret
	}
	if cfg.BillingProvider != "paypal" && cfg.BillingProvider != "stripe" {
		cfg.BillingProvider = "stripe"
	}
	if cfg.StripePortalReturnURL == "" {
		cfg.StripePortalReturnURL = strings.TrimSuffix(cfg.WebBaseURL, "/") + "/billing"
	}
	if cfg.RetentionTerminalDays < 1 {
		cfg.RetentionTerminalDays = 180
	}
	if cfg.RetentionDedupeDays < 1 {
		cfg.RetentionDedupeDays = 30
	}
	if cfg.RetentionBatchSize < 1 {
		cfg.RetentionBatchSize = 500
	}
	if cfg.RetentionIntervalMinutes < 1 {
		cfg.RetentionIntervalMinutes = 60
	}
	if cfg.AlertFiveXXThreshold < 1 {
		cfg.AlertFiveXXThreshold = 20
	}
	if cfg.AlertFiveXXWindowMinutes < 1 {
		cfg.AlertFiveXXWindowMinutes = 5
	}
	if cfg.AlertFiveXXCooldownMinutes < 1 {
		cfg.AlertFiveXXCooldownMinutes = 20
	}
	if cfg.AlertDeadLetterThreshold < 1 {
		cfg.AlertDeadLetterThreshold = 3
	}
	if cfg.AlertDeadLetterWindowMinutes < 1 {
		cfg.AlertDeadLetterWindowMinutes = 10
	}
	return cfg, nil
}

func envCSV(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envIntOr(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBoolOr(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}
