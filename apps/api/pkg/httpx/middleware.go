package httpx

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/genxcare/api/pkg/logging"
)

// RequestLogger attaches a request-scoped logger and records one line per
// request. It deliberately never logs headers (which carry the access token)
// or bodies (which carry care data).
func RequestLogger(base *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Echo the request ID so support can correlate a user report with
			// server logs.
			requestID := middleware.GetReqID(r.Context())
			if requestID != "" {
				w.Header().Set("X-Request-Id", requestID)
			}

			logger := base.With(
				slog.String("request_id", requestID),
				slog.String("method", r.Method),
				slog.String("path", RedactPath(r.URL.Path)),
			)
			ctx := logging.WithLogger(r.Context(), logger)
			r = r.WithContext(ctx)

			recorder := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(recorder, r)

			logger.Info("request completed",
				slog.Int("status", recorder.Status()),
				slog.Int("bytes", recorder.BytesWritten()),
				slog.Duration("duration", time.Since(start)),
			)
		})
	}
}

// Recoverer converts a panic into the standard error envelope instead of
// letting the connection drop with a raw stack trace.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}
			// http.ErrAbortHandler is an intentional abort; re-panic so the
			// server handles it as designed.
			if err, ok := recovered.(error); ok && err == http.ErrAbortHandler {
				panic(recovered)
			}

			logging.FromContext(r.Context()).Error("panic recovered",
				slog.Any("panic", recovered),
				slog.String("stack", string(debug.Stack())),
			)
			WriteError(w, r, ErrInternal(fmt.Errorf("panic: %v", recovered)))
		}()

		next.ServeHTTP(w, r)
	})
}

// CORS allows the listed origins. The MVP is mobile-only, so the default
// (empty) configuration adds no CORS headers at all.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	origins := make([]string, 0, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		if trimmed := strings.TrimSpace(origin); trimmed != "" {
			origins = append(origins, trimmed)
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && slices.Contains(origins, origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
				w.Header().Add("Vary", "Origin")

				if r.Method == http.MethodOptions {
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// NotFoundHandler renders unknown routes with the shared error envelope.
func NotFoundHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, ErrNotFound("The requested resource does not exist."))
	}
}

// MethodNotAllowedHandler renders wrong-method requests with the shared envelope.
func MethodNotAllowedHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, &APIError{
			Status:  http.StatusMethodNotAllowed,
			Code:    CodeBadRequest,
			Message: "That method is not supported for this resource.",
		})
	}
}
