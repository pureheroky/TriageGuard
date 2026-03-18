package middleware

import (
	"net/http"
	"strings"
)

// SecurityHeaders sets baseline headers for API responses.
func SecurityHeaders() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("Cross-Origin-Resource-Policy", "same-site")
			w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
			w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
			proto := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")))
			if r.TLS != nil || proto == "https" {
				w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}
