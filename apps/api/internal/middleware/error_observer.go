package middleware

import (
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

type ServerErrorEvent struct {
	Method    string
	Path      string
	Status    int
	RequestID string
	Duration  time.Duration
}

type ServerErrorObserver func(event ServerErrorEvent)

type observeStatusWriter struct {
	http.ResponseWriter
	status int
}

func (w *observeStatusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *observeStatusWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(p)
}

// Observe5xx invokes observer for HTTP 5xx responses.
func Observe5xx(observer ServerErrorObserver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if observer == nil {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()
			ww := &observeStatusWriter{ResponseWriter: w}
			next.ServeHTTP(ww, r)

			status := ww.status
			if status == 0 {
				status = http.StatusOK
			}
			if status < http.StatusInternalServerError {
				return
			}

			observer(ServerErrorEvent{
				Method:    r.Method,
				Path:      r.URL.Path,
				Status:    status,
				RequestID: chimw.GetReqID(r.Context()),
				Duration:  time.Since(start),
			})
		})
	}
}
