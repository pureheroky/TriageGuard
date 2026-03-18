package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRateLimitBlocksAfterBurst(t *testing.T) {
	mw := RateLimit(60, 2)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req1 := httptest.NewRequest(http.MethodPost, "/slack/events", nil)
	req1.RemoteAddr = "1.2.3.4:12345"
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("first request status=%d, want=%d", rr1.Code, http.StatusOK)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/slack/events", nil)
	req2.RemoteAddr = "1.2.3.4:12345"
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("second request status=%d, want=%d", rr2.Code, http.StatusOK)
	}

	req3 := httptest.NewRequest(http.MethodPost, "/slack/events", nil)
	req3.RemoteAddr = "1.2.3.4:12345"
	rr3 := httptest.NewRecorder()
	handler.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusTooManyRequests {
		t.Fatalf("third request status=%d, want=%d", rr3.Code, http.StatusTooManyRequests)
	}
	if rr3.Header().Get("Retry-After") == "" {
		t.Fatalf("expected Retry-After header")
	}
}

func TestRateLimitSeparateIPs(t *testing.T) {
	mw := RateLimit(60, 1)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	reqA := httptest.NewRequest(http.MethodPost, "/slack/events", nil)
	reqA.RemoteAddr = "1.2.3.4:12345"
	rrA := httptest.NewRecorder()
	handler.ServeHTTP(rrA, reqA)
	if rrA.Code != http.StatusOK {
		t.Fatalf("ip A first status=%d, want=%d", rrA.Code, http.StatusOK)
	}

	reqB := httptest.NewRequest(http.MethodPost, "/slack/events", nil)
	reqB.RemoteAddr = "5.6.7.8:54321"
	rrB := httptest.NewRecorder()
	handler.ServeHTTP(rrB, reqB)
	if rrB.Code != http.StatusOK {
		t.Fatalf("ip B first status=%d, want=%d", rrB.Code, http.StatusOK)
	}
}

func TestClientIPKeyPrefersForwardedHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.RemoteAddr = "9.9.9.9:1234"
	req.Header.Set("X-Forwarded-For", "8.8.8.8, 7.7.7.7")
	req.Header.Set("X-Real-IP", "6.6.6.6")

	key := clientIPKey(req)
	if key != "ip:6.6.6.6" {
		t.Fatalf("clientIPKey=%q, want ip:6.6.6.6", key)
	}
}
