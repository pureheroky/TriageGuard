package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSAllowedOriginPreflight(t *testing.T) {
	mw := CORS([]string{"http://localhost:3000"})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("next handler should not be called for OPTIONS")
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/me", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Fatalf("allow-origin = %q", got)
	}
}

func TestCORSDisallowedOriginNoAllowHeader(t *testing.T) {
	mw := CORS([]string{"http://localhost:3000"})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("Origin", "https://malicious.example.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("allow-origin must be empty for disallowed origin, got %q", got)
	}
}

func TestCORSDisallowedOriginPreflightForbidden(t *testing.T) {
	mw := CORS([]string{"http://localhost:3000"})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("next handler should not be called for disallowed preflight")
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/me", nil)
	req.Header.Set("Origin", "https://malicious.example.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusForbidden)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("allow-origin must be empty for disallowed origin, got %q", got)
	}
}
