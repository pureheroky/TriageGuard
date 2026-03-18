package slack

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func makeSlackSignature(secret, timestamp string, body []byte) string {
	base := "v0:" + timestamp + ":" + string(body)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(base))
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifyRequestValid(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"type":"event_callback"}`)
	ts := strconv.FormatInt(time.Now().Unix(), 10)

	req := httptest.NewRequest("POST", "/slack/events", strings.NewReader(string(body)))
	req.Header.Set("X-Slack-Request-Timestamp", ts)
	req.Header.Set("X-Slack-Signature", makeSlackSignature(secret, ts, body))

	got, err := VerifyRequest(req, secret)
	if err != nil {
		t.Fatalf("VerifyRequest returned error: %v", err)
	}
	if string(got) != string(body) {
		t.Fatalf("body mismatch")
	}
}

func TestVerifyRequestRejectsFutureTimestampSkew(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"type":"event_callback"}`)
	ts := strconv.FormatInt(time.Now().Add(10*time.Minute).Unix(), 10)

	req := httptest.NewRequest("POST", "/slack/events", strings.NewReader(string(body)))
	req.Header.Set("X-Slack-Request-Timestamp", ts)
	req.Header.Set("X-Slack-Signature", makeSlackSignature(secret, ts, body))

	if _, err := VerifyRequest(req, secret); err == nil {
		t.Fatalf("expected stale slack request error")
	}
}

func TestVerifyRequestRejectsOversizedBody(t *testing.T) {
	secret := "test-secret"
	body := []byte(strings.Repeat("x", maxSlackBodyBytes+1))
	ts := strconv.FormatInt(time.Now().Unix(), 10)

	req := httptest.NewRequest("POST", "/slack/events", strings.NewReader(string(body)))
	req.Header.Set("X-Slack-Request-Timestamp", ts)
	req.Header.Set("X-Slack-Signature", makeSlackSignature(secret, ts, body))

	if _, err := VerifyRequest(req, secret); err == nil {
		t.Fatalf("expected oversized body error")
	}
}
