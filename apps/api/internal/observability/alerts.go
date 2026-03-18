package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type AlertConfig struct {
	WebhookURL               string
	FiveXXThreshold          int
	FiveXXWindow             time.Duration
	FiveXXCooldown           time.Duration
	DeadLetterCooldown       time.Duration
	DeadLetterAlertThreshold int
}

type AlertManager struct {
	cfg        AlertConfig
	sentry     *SentryClient
	httpClient *http.Client
	logger     *log.Logger

	mu                sync.Mutex
	fiveXXTimestamps  []time.Time
	deadLetterTimes   []time.Time
	lastFiveXXAlertAt time.Time
	lastDeadLetterAt  time.Time
}

func NewAlertManager(cfg AlertConfig, sentry *SentryClient, logger *log.Logger) *AlertManager {
	if logger == nil {
		logger = log.Default()
	}
	if cfg.FiveXXThreshold < 1 {
		cfg.FiveXXThreshold = 20
	}
	if cfg.FiveXXWindow <= 0 {
		cfg.FiveXXWindow = 5 * time.Minute
	}
	if cfg.FiveXXCooldown <= 0 {
		cfg.FiveXXCooldown = 20 * time.Minute
	}
	if cfg.DeadLetterCooldown <= 0 {
		cfg.DeadLetterCooldown = 10 * time.Minute
	}
	if cfg.DeadLetterAlertThreshold < 1 {
		cfg.DeadLetterAlertThreshold = 3
	}

	return &AlertManager{
		cfg:        cfg,
		sentry:     sentry,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		logger:     logger,
	}
}

func (a *AlertManager) Enabled() bool {
	return a != nil && strings.TrimSpace(a.cfg.WebhookURL) != ""
}

func (a *AlertManager) Record5xx(method, routePath, requestID string, status int) {
	if a == nil {
		return
	}

	now := time.Now().UTC()

	shouldAlert := false
	var count int

	a.mu.Lock()
	windowStart := now.Add(-a.cfg.FiveXXWindow)
	pruned := a.fiveXXTimestamps[:0]
	for _, ts := range a.fiveXXTimestamps {
		if ts.After(windowStart) {
			pruned = append(pruned, ts)
		}
	}
	a.fiveXXTimestamps = append(pruned, now)
	count = len(a.fiveXXTimestamps)

	if count >= a.cfg.FiveXXThreshold && now.Sub(a.lastFiveXXAlertAt) >= a.cfg.FiveXXCooldown {
		a.lastFiveXXAlertAt = now
		shouldAlert = true
	}
	a.mu.Unlock()

	if !shouldAlert {
		return
	}

	message := fmt.Sprintf(
		"TriageGuard alert: %d HTTP 5xx responses in last %dm. Latest=%s %s status=%d req_id=%s",
		count,
		int(a.cfg.FiveXXWindow.Minutes()),
		strings.ToUpper(strings.TrimSpace(method)),
		strings.TrimSpace(routePath),
		status,
		strings.TrimSpace(requestID),
	)
	a.sendAlert("5xx_spike", message, map[string]any{
		"count":     count,
		"window_ms": a.cfg.FiveXXWindow.Milliseconds(),
		"method":    method,
		"path":      routePath,
		"status":    status,
		"requestID": requestID,
	})
}

func (a *AlertManager) RecordDeadLetter(source, operation, errorMessage string) {
	if a == nil {
		return
	}

	now := time.Now().UTC()
	shouldAlert := false
	var count int

	a.mu.Lock()
	windowStart := now.Add(-a.cfg.DeadLetterCooldown)
	pruned := a.deadLetterTimes[:0]
	for _, ts := range a.deadLetterTimes {
		if ts.After(windowStart) {
			pruned = append(pruned, ts)
		}
	}
	a.deadLetterTimes = append(pruned, now)
	count = len(a.deadLetterTimes)
	if count >= a.cfg.DeadLetterAlertThreshold && now.Sub(a.lastDeadLetterAt) >= a.cfg.DeadLetterCooldown {
		a.lastDeadLetterAt = now
		shouldAlert = true
	}
	a.mu.Unlock()

	if !shouldAlert {
		return
	}

	message := fmt.Sprintf(
		"TriageGuard alert: dead letters spike (%d in last %dm). Latest=%s/%s error=%s",
		count,
		int(a.cfg.DeadLetterCooldown.Minutes()),
		strings.TrimSpace(source),
		strings.TrimSpace(operation),
		trimForAlert(errorMessage),
	)
	a.sendAlert("dead_letters_spike", message, map[string]any{
		"count":     count,
		"source":    source,
		"operation": operation,
		"error":     trimForAlert(errorMessage),
	})
}

func (a *AlertManager) sendAlert(kind, message string, extra map[string]any) {
	if a == nil {
		return
	}

	if a.sentry != nil && a.sentry.Enabled() {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		_ = a.sentry.Capture(ctx, "error", message, map[string]string{
			"alert_kind": strings.TrimSpace(kind),
		}, extra)
		cancel()
	}

	webhook := strings.TrimSpace(a.cfg.WebhookURL)
	if webhook == "" {
		return
	}

	payload := map[string]any{
		"text":    message,
		"content": message,
		"kind":    kind,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		a.logger.Printf("alert marshal failed kind=%s: %v", kind, err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook, bytes.NewReader(body))
	if err != nil {
		a.logger.Printf("alert request build failed kind=%s: %v", kind, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		a.logger.Printf("alert send failed kind=%s: %v", kind, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		a.logger.Printf("alert send failed kind=%s status=%d", kind, resp.StatusCode)
	}
}

func trimForAlert(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "unknown"
	}
	const max = 300
	if len(trimmed) <= max {
		return trimmed
	}
	return trimmed[:max] + "..."
}
