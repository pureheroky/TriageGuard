package middleware

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type rateLimitEntry struct {
	Tokens float64
	Last   time.Time
}

// RateLimit applies an in-memory token-bucket limiter per client IP.
// It is intended for public endpoints (webhooks/auth-like routes) as a safety baseline.
func RateLimit(requestsPerMinute, burst int) func(http.Handler) http.Handler {
	if requestsPerMinute <= 0 {
		requestsPerMinute = 60
	}
	if burst < 1 {
		burst = requestsPerMinute
	}

	ttl := 30 * time.Minute
	ratePerSecond := float64(requestsPerMinute) / 60.0

	var (
		mu      sync.Mutex
		entries = map[string]rateLimitEntry{}
	)

	allow := func(key string, now time.Time) (bool, int) {
		mu.Lock()
		defer mu.Unlock()

		// Opportunistic cleanup of stale entries.
		for k, v := range entries {
			if now.Sub(v.Last) > ttl {
				delete(entries, k)
			}
		}

		entry, ok := entries[key]
		if !ok {
			entry = rateLimitEntry{
				Tokens: float64(burst),
				Last:   now,
			}
		}

		elapsed := now.Sub(entry.Last).Seconds()
		if elapsed > 0 {
			entry.Tokens += elapsed * ratePerSecond
			if entry.Tokens > float64(burst) {
				entry.Tokens = float64(burst)
			}
		}
		entry.Last = now

		if entry.Tokens >= 1 {
			entry.Tokens -= 1
			entries[key] = entry
			return true, 0
		}

		entries[key] = entry
		missing := 1 - entry.Tokens
		retryAfter := int(missing/ratePerSecond + 0.999) // ceil
		if retryAfter < 1 {
			retryAfter = 1
		}
		return false, retryAfter
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			now := time.Now().UTC()
			key := clientIPKey(r)
			allowed, retryAfter := allow(key, now)
			if !allowed {
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func clientIPKey(r *http.Request) string {
	realIP := strings.TrimSpace(r.Header.Get("X-Real-IP"))
	if realIP != "" {
		return "ip:" + realIP
	}

	xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				return "ip:" + ip
			}
		}
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return "ip:" + host
	}
	if strings.TrimSpace(r.RemoteAddr) != "" {
		return "ip:" + strings.TrimSpace(r.RemoteAddr)
	}
	return "ip:unknown"
}

func limiterDebugLabel(requestsPerMinute, burst int) string {
	return fmt.Sprintf("rpm=%d burst=%d", requestsPerMinute, burst)
}
