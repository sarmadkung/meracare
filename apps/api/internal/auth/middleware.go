package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/genxcare/api/pkg/httpx"
	"github.com/genxcare/api/pkg/logging"
)

// UserResolver maps a verified Supabase identity onto an application user,
// creating the record on first sign-in.
//
// It is an interface so that `internal/auth` does not depend on
// `internal/users`, keeping the dependency direction one-way.
type UserResolver interface {
	ResolveFromClaims(ctx context.Context, claims *Claims) (Principal, error)
}

// RequireAuth verifies the bearer token, resolves the application user, and
// puts the resulting Principal in the request context.
//
// Every failure returns the same UNAUTHENTICATED response so the endpoint does
// not become an oracle for which tokens or accounts exist.
func RequireAuth(verifier Verifier, resolver UserResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			logger := logging.FromContext(ctx)

			rawToken, ok := BearerToken(r.Header.Get("Authorization"))
			if !ok {
				unauthenticated(w, r)
				return
			}

			claims, err := verifier.Verify(ctx, rawToken)
			if err != nil {
				// Log the reason, never the token itself.
				logger.Info("rejected access token", slog.String("reason", redactedReason(err)))
				unauthenticated(w, r)
				return
			}

			principal, err := resolver.ResolveFromClaims(ctx, claims)
			if err != nil {
				logger.Error("failed to resolve application user", slog.Any("error", err))
				httpx.WriteError(w, r, httpx.ErrInternal(err))
				return
			}

			ctx = WithPrincipal(ctx, principal)
			ctx = logging.WithLogger(ctx, logger.With(slog.String("user_id", principal.UserID.String())))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func unauthenticated(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="genxcare"`)
	httpx.WriteError(w, r, httpx.ErrUnauthenticated("Sign in to continue."))
}

// redactedReason keeps token contents out of logs while preserving the
// diagnostic category of the failure.
func redactedReason(err error) string {
	if errors.Is(err, ErrInvalidToken) {
		return "token failed validation"
	}
	return "verification error"
}
