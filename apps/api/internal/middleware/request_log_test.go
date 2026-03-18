package middleware

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestLogRedactsQueryString(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := log.New(buf, "", 0)

	h := RequestLog(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/slack/oauth/callback?code=secret&state=also-secret", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)

	logLine := buf.String()
	if strings.Contains(logLine, "code=secret") || strings.Contains(logLine, "state=also-secret") {
		t.Fatalf("expected query params to be redacted, got: %s", logLine)
	}
	if !strings.Contains(logLine, "path=/slack/oauth/callback") {
		t.Fatalf("expected sanitized path in log, got: %s", logLine)
	}
}
