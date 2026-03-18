package slack

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxSlackRequestAge = 5 * time.Minute
const maxSlackBodyBytes = 2 << 20 // 2 MiB

func VerifyRequest(r *http.Request, signingSecret string) ([]byte, error) {
	if signingSecret == "" {
		return nil, fmt.Errorf("slack signing secret not configured")
	}
	timestamp := r.Header.Get("X-Slack-Request-Timestamp")
	signature := r.Header.Get("X-Slack-Signature")
	if timestamp == "" || signature == "" {
		return nil, fmt.Errorf("missing slack signature headers")
	}
	tsInt, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid slack timestamp: %w", err)
	}
	ts := time.Unix(tsInt, 0)
	age := time.Since(ts)
	if age > maxSlackRequestAge || age < -maxSlackRequestAge {
		return nil, fmt.Errorf("stale slack request")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxSlackBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}
	if len(body) > maxSlackBodyBytes {
		return nil, fmt.Errorf("slack request body too large")
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	base := "v0:" + timestamp + ":" + string(body)
	mac := hmac.New(sha256.New, []byte(signingSecret))
	_, _ = mac.Write([]byte(base))
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(strings.TrimSpace(signature)), []byte(expected)) {
		return nil, fmt.Errorf("invalid slack signature")
	}
	return body, nil
}
