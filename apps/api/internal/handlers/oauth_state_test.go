package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"triageguard/apps/api/internal/config"
)

func TestOAuthStateBuildAndValidateWithCookie(t *testing.T) {
	userID := uuid.New()
	workspaceID := uuid.New()
	app := &App{cfg: config.Config{
		OAuthStateSecret: "state-secret",
		APIBaseURL:       "https://example.ngrok-free.dev",
	}}

	state, nonce, err := app.buildOAuthState("linear", &userID, &workspaceID)
	if err != nil {
		t.Fatalf("buildOAuthState: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/linear/oauth/callback?state="+url.QueryEscape(state), nil)
	req.AddCookie(&http.Cookie{Name: oauthStateCookieName("linear"), Value: nonce})

	payload, err := app.validateOAuthState(req, "linear")
	if err != nil {
		t.Fatalf("validateOAuthState: %v", err)
	}
	if payload.Provider != "linear" {
		t.Fatalf("provider mismatch: %s", payload.Provider)
	}
	if payload.UserID != userID.String() {
		t.Fatalf("user mismatch: %s", payload.UserID)
	}
	if payload.WorkspaceID != workspaceID.String() {
		t.Fatalf("workspace mismatch: %s", payload.WorkspaceID)
	}
}

func TestOAuthStateValidateWithoutCookieAllowed(t *testing.T) {
	app := &App{cfg: config.Config{
		OAuthStateSecret: "state-secret",
		APIBaseURL:       "https://example.ngrok-free.dev",
	}}
	state, _, err := app.buildOAuthState("slack", nil, nil)
	if err != nil {
		t.Fatalf("buildOAuthState: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/slack/oauth/callback?state="+url.QueryEscape(state), nil)
	payload, err := app.validateOAuthState(req, "slack")
	if err != nil {
		t.Fatalf("validateOAuthState without cookie: %v", err)
	}
	if payload.Provider != "slack" {
		t.Fatalf("provider mismatch: %s", payload.Provider)
	}
}

func TestOAuthStateRejectsNonceMismatch(t *testing.T) {
	app := &App{cfg: config.Config{
		OAuthStateSecret: "state-secret",
		APIBaseURL:       "https://example.ngrok-free.dev",
	}}
	state, _, err := app.buildOAuthState("linear", nil, nil)
	if err != nil {
		t.Fatalf("buildOAuthState: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/linear/oauth/callback?state="+url.QueryEscape(state), nil)
	req.AddCookie(&http.Cookie{Name: oauthStateCookieName("linear"), Value: "wrong-nonce"})
	_, err = app.validateOAuthState(req, "linear")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "nonce mismatch") {
		t.Fatalf("expected nonce mismatch error, got: %v", err)
	}
}

func TestOAuthStateRejectsTamperedSignature(t *testing.T) {
	app := &App{cfg: config.Config{
		OAuthStateSecret: "state-secret",
		APIBaseURL:       "https://example.ngrok-free.dev",
	}}
	state, _, err := app.buildOAuthState("linear", nil, nil)
	if err != nil {
		t.Fatalf("buildOAuthState: %v", err)
	}

	parts := strings.Split(state, ".")
	if len(parts) != 2 {
		t.Fatalf("unexpected state format: %s", state)
	}
	tampered := parts[0] + ".invalidsignature"
	req := httptest.NewRequest(http.MethodGet, "/linear/oauth/callback?state="+url.QueryEscape(tampered), nil)
	_, err = app.validateOAuthState(req, "linear")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "signature") {
		t.Fatalf("expected signature error, got: %v", err)
	}
}
