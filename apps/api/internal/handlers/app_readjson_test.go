package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadJSONValidPayload(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/policies", strings.NewReader(`{"ack_sla_minutes":15}`))
	var payload struct {
		AckSLAMinutes int `json:"ack_sla_minutes"`
	}

	if err := readJSON(req, &payload); err != nil {
		t.Fatalf("readJSON returned error: %v", err)
	}
	if payload.AckSLAMinutes != 15 {
		t.Fatalf("ack_sla_minutes = %d, want 15", payload.AckSLAMinutes)
	}
}

func TestReadJSONUnknownFieldRejected(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/policies", strings.NewReader(`{"ack_sla_minutes":15,"unexpected":true}`))
	var payload struct {
		AckSLAMinutes int `json:"ack_sla_minutes"`
	}

	if err := readJSON(req, &payload); err == nil {
		t.Fatalf("expected unknown field error")
	}
}

func TestReadJSONTrailingDataRejected(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/policies", strings.NewReader(`{"ack_sla_minutes":15}{"x":1}`))
	var payload struct {
		AckSLAMinutes int `json:"ack_sla_minutes"`
	}

	err := readJSON(req, &payload)
	if err == nil {
		t.Fatalf("expected trailing JSON error")
	}
	if !strings.Contains(err.Error(), "single JSON object") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadJSONBodyTooLarge(t *testing.T) {
	oversized := strings.Repeat("a", maxJSONBodyBytes+1)
	req := httptest.NewRequest("POST", "/api/policies", strings.NewReader(`{"name":"`+oversized+`"}`))
	var payload struct {
		Name string `json:"name"`
	}

	err := readJSON(req, &payload)
	if err == nil {
		t.Fatalf("expected body too large error")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Fatalf("unexpected error: %v", err)
	}
}
