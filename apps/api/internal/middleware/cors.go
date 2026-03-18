package middleware

import (
	"net/http"
	"strings"
)

// CORS applies a minimal cross-origin policy for browser-based admin calls.
// It is intentionally narrow: only allowed origins get CORS headers.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := map[string]struct{}{}
	for _, origin := range allowedOrigins {
		trimmed := strings.TrimSpace(origin)
		if trimmed == "" {
			continue
		}
		allowed[trimmed] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			allowedOrigin := false
			if origin != "" {
				if _, ok := allowed[origin]; ok {
					allowedOrigin = true
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Vary", "Origin")
					w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type")
					w.Header().Set("Access-Control-Max-Age", "600")
				}
			}

			if r.Method == http.MethodOptions {
				if origin != "" && !allowedOrigin {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
