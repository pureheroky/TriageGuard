package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestObserve5xxInvokesObserver(t *testing.T) {
	calls := 0
	var last ServerErrorEvent

	h := Observe5xx(func(event ServerErrorEvent) {
		calls++
		last = event
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test?token=secret", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)

	if calls != 1 {
		t.Fatalf("expected one observer call, got %d", calls)
	}
	if last.Path != "/api/test" {
		t.Fatalf("expected path without query, got %q", last.Path)
	}
	if last.Status != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", last.Status)
	}
}

func TestObserve5xxSkipsNonServerErrors(t *testing.T) {
	calls := 0

	h := Observe5xx(func(event ServerErrorEvent) {
		calls++
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)

	if calls != 0 {
		t.Fatalf("expected no observer calls, got %d", calls)
	}
}
