package observability

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

type SentryClient struct {
	enabled    bool
	endpoint   string
	publicKey  string
	secretKey  string
	httpClient *http.Client
}

func NewSentryClient(dsn string) *SentryClient {
	parsed := strings.TrimSpace(dsn)
	if parsed == "" {
		return &SentryClient{enabled: false}
	}

	u, err := url.Parse(parsed)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User == nil {
		return &SentryClient{enabled: false}
	}

	publicKey := strings.TrimSpace(u.User.Username())
	if publicKey == "" {
		return &SentryClient{enabled: false}
	}
	secretKey, _ := u.User.Password()
	secretKey = strings.TrimSpace(secretKey)

	cleanPath := strings.Trim(strings.TrimSpace(u.Path), "/")
	if cleanPath == "" {
		return &SentryClient{enabled: false}
	}
	parts := strings.Split(cleanPath, "/")
	projectID := strings.TrimSpace(parts[len(parts)-1])
	if projectID == "" {
		return &SentryClient{enabled: false}
	}

	prefix := ""
	if len(parts) > 1 {
		prefix = "/" + strings.Join(parts[:len(parts)-1], "/")
	}
	storePath := path.Clean(prefix + "/api/" + projectID + "/store/")
	if !strings.HasPrefix(storePath, "/") {
		storePath = "/" + storePath
	}

	endpoint := fmt.Sprintf("%s://%s%s", u.Scheme, u.Host, storePath)
	return &SentryClient{
		enabled:    true,
		endpoint:   endpoint,
		publicKey:  publicKey,
		secretKey:  secretKey,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

func (s *SentryClient) Enabled() bool {
	return s != nil && s.enabled
}

func (s *SentryClient) Capture(ctx context.Context, level, message string, tags map[string]string, extra map[string]any) error {
	if !s.Enabled() {
		return nil
	}
	level = strings.TrimSpace(strings.ToLower(level))
	if level == "" {
		level = "error"
	}

	event := map[string]any{
		"event_id":  randomEventID(),
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"platform":  "go",
		"logger":    "triageguard-api",
		"level":     level,
		"message":   strings.TrimSpace(message),
	}
	if len(tags) > 0 {
		event["tags"] = tags
	}
	if len(extra) > 0 {
		event["extra"] = extra
	}

	body, err := json.Marshal(event)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Sentry-Auth", s.authHeader())

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("sentry store failed status=%d", resp.StatusCode)
}

func (s *SentryClient) authHeader() string {
	if s.secretKey == "" {
		return fmt.Sprintf("Sentry sentry_version=7, sentry_client=triageguard-api/1.0, sentry_key=%s", s.publicKey)
	}
	return fmt.Sprintf("Sentry sentry_version=7, sentry_client=triageguard-api/1.0, sentry_key=%s, sentry_secret=%s", s.publicKey, s.secretKey)
}

func randomEventID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return strings.ReplaceAll(fmt.Sprintf("%d", time.Now().UTC().UnixNano()), "-", "")
	}
	return hex.EncodeToString(buf)
}
