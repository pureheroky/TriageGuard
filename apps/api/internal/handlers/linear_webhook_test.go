package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestVerifyLinearWebhookSignature(t *testing.T) {
	body := []byte(`{"type":"Issue","action":"update","data":{"id":"abc"}}`)
	secret := "linear-test-secret"

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	if err := verifyLinearWebhookSignature(body, "sha256="+sig, secret); err != nil {
		t.Fatalf("expected valid signature, got: %v", err)
	}
	if err := verifyLinearWebhookSignature(body, "v1="+sig, secret); err != nil {
		t.Fatalf("expected v1 signature to validate, got: %v", err)
	}
	if err := verifyLinearWebhookSignature(body, "sha256=deadbeef", secret); err == nil {
		t.Fatalf("expected invalid signature error")
	}
	if err := verifyLinearWebhookSignature(body, "", secret); err == nil {
		t.Fatalf("expected missing signature error")
	}
}

func TestLinearWebhookDeliveryID(t *testing.T) {
	body := []byte("payload")
	if got := linearWebhookDeliveryID("delivery-123", body); got != "delivery-123" {
		t.Fatalf("expected header delivery id, got %q", got)
	}
	if got := linearWebhookDeliveryID("", body); !strings.HasPrefix(got, "sha256:") {
		t.Fatalf("expected sha256 fallback id, got %q", got)
	}
}

func TestRequestStatusSyncActionFromLinearState(t *testing.T) {
	tests := []struct {
		currentStatus string
		linearState   string
		wantAction    string
	}{
		{currentStatus: "NEW", linearState: "completed", wantAction: "SYNC_RESOLVE"},
		{currentStatus: "ACKED", linearState: "canceled", wantAction: "SYNC_IGNORE"},
		{currentStatus: "RESOLVED", linearState: "started", wantAction: "SYNC_REOPEN"},
		{currentStatus: "IGNORED", linearState: "unstarted", wantAction: "SYNC_REOPEN"},
		{currentStatus: "IGNORED", linearState: "completed", wantAction: ""},
		{currentStatus: "RESOLVED", linearState: "canceled", wantAction: ""},
		{currentStatus: "ASSIGNED", linearState: "started", wantAction: ""},
	}

	for _, tc := range tests {
		got := requestStatusSyncActionFromLinearState(tc.currentStatus, tc.linearState)
		if got != tc.wantAction {
			t.Fatalf("requestStatusSyncActionFromLinearState(%q,%q)=%q want=%q", tc.currentStatus, tc.linearState, got, tc.wantAction)
		}
	}
}

func TestLinearDueDateConversionAndComparison(t *testing.T) {
	raw := "2026-03-06"
	dueAt, err := linearDueDateToDueAt(&raw, "Europe/Madrid")
	if err != nil {
		t.Fatalf("linearDueDateToDueAt returned error: %v", err)
	}
	if dueAt == nil {
		t.Fatalf("expected dueAt value")
	}

	date := dueAtToLinearDate(dueAt, "Europe/Madrid")
	if date == nil || *date != raw {
		t.Fatalf("expected due date %q, got %v", raw, date)
	}

	if !sameDueDate(dueAt, &raw, "Europe/Madrid") {
		t.Fatalf("expected due date to match")
	}

	other := "2026-03-07"
	if sameDueDate(dueAt, &other, "Europe/Madrid") {
		t.Fatalf("expected due date mismatch")
	}

	if !sameDueDate(nil, nil, "UTC") {
		t.Fatalf("nil due dates should match")
	}
}

func TestSameOptionalString(t *testing.T) {
	a := " U1 "
	b := "U1"
	if !sameOptionalString(&a, &b) {
		t.Fatalf("expected trimmed strings to match")
	}
	if sameOptionalString(&a, nil) {
		t.Fatalf("expected mismatch with nil")
	}
}
