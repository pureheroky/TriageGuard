package middleware

import (
	"log"
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *loggingResponseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *loggingResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += n
	return n, err
}

// RequestLog logs request metadata without query params to avoid leaking sensitive values.
func RequestLog(logger *log.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = log.Default()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := &loggingResponseWriter{ResponseWriter: w}
			next.ServeHTTP(ww, r)

			status := ww.status
			if status == 0 {
				status = http.StatusOK
			}

			logger.Printf(
				"http request id=%s method=%s path=%s remote=%s status=%d bytes=%d duration_ms=%d",
				chimw.GetReqID(r.Context()),
				r.Method,
				r.URL.Path,
				r.RemoteAddr,
				status,
				ww.bytes,
				time.Since(start).Milliseconds(),
			)
		})
	}
}
