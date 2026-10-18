package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

type contextKey struct {
	name string
}

var httpContextKey = &contextKey{"HTTPContext"}

// HTTPContext carries request-local observations for the logging middleware.
type HTTPContext struct {
	RequestStart time.Time
	StatusCode   int
	Err          error
}

func GetHTTPContext(r *http.Request) *HTTPContext {
	if ctx, ok := r.Context().Value(httpContextKey).(*HTTPContext); ok {
		return ctx
	}
	panic("HTTPContext not found")
}

// HTTPContextMiddleware must wrap logging and all handlers that use HTTPContext.
func HTTPContextMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := &HTTPContext{RequestStart: time.Now()}
		r = r.WithContext(context.WithValue(r.Context(), httpContextKey, ctx))
		next.ServeHTTP(w, r)
	})
}

// CSRFMiddleware applies Go's standard browser cross-origin protections.
func CSRFMiddleware(next http.Handler) http.Handler {
	csrf := http.NewCrossOriginProtection()
	csrf.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusForbidden, map[string]string{"error": "cross_origin_request"})
	}))
	return csrf.Handler(next)
}

// LoggingMiddleware records route patterns rather than user-provided paths or query strings.
func LoggingMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := GetHTTPContext(r)
		rw := &responseWriter{ResponseWriter: w, status: &ctx.StatusCode}
		completed := false
		defer func() {
			if completed && ctx.StatusCode == 0 {
				ctx.StatusCode = http.StatusOK
			}
			args := []any{
				"method", r.Method,
				"route", r.Pattern,
				"status", ctx.StatusCode,
				"duration_ms", float64(time.Since(ctx.RequestStart).Microseconds()) / 1000,
				"aborted", !completed,
			}
			level := slog.LevelInfo
			if ctx.Err != nil {
				args = append(args, "error", ctx.Err)
				level = slog.LevelError
			}
			logger.Log(r.Context(), level, "http_request", args...)
		}()
		next.ServeHTTP(rw, r)
		completed = true
	})
}

type responseWriter struct {
	http.ResponseWriter
	status *int
}

func (w *responseWriter) WriteHeader(status int) {
	if *w.status != 0 {
		return
	}
	w.ResponseWriter.WriteHeader(status)
	if status >= 200 || status == http.StatusSwitchingProtocols {
		*w.status = status
	}
}

func (w *responseWriter) Write(data []byte) (int, error) {
	if *w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

// Unwrap preserves access through http.ResponseController.
func (w *responseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *responseWriter) FlushError() error {
	if *w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return http.NewResponseController(w.ResponseWriter).Flush()
}
