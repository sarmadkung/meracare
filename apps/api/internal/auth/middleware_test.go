package auth_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/genxcare/api/internal/auth"
	"github.com/genxcare/api/pkg/httpx"
)

type stubResolver struct {
	principal auth.Principal
	err       error
	calls     int
}

func (s *stubResolver) ResolveFromClaims(_ context.Context, claims *auth.Claims) (auth.Principal, error) {
	s.calls++
	if s.err != nil {
		return auth.Principal{}, s.err
	}
	s.principal.AuthUserID = claims.AuthUserID
	s.principal.Email = claims.Email
	return s.principal, nil
}

// protectedHandler asserts that the principal reached the handler.
func protectedHandler(t *testing.T, want auth.Principal) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal := auth.MustPrincipal(r.Context())
		if principal.UserID != want.UserID {
			t.Errorf("principal.UserID = %s, want %s", principal.UserID, want.UserID)
		}
		w.WriteHeader(http.StatusOK)
	})
}

func TestRequireAuthAllowsValidToken(t *testing.T) {
	appUserID := uuid.New()
	resolver := &stubResolver{principal: auth.Principal{UserID: appUserID}}
	handler := auth.RequireAuth(newVerifier(t), resolver)(protectedHandler(t, auth.Principal{UserID: appUserID}))

	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+signToken(t, tokenOverrides{email: "ahmed@example.com"}))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if resolver.calls != 1 {
		t.Errorf("resolver called %d times, want 1", resolver.calls)
	}
}

func TestRequireAuthRejectsMissingAndInvalidTokens(t *testing.T) {
	cases := map[string]string{
		"no header":       "",
		"not bearer":      "Basic abc",
		"invalid token":   "Bearer not-a-jwt",
		"wrong signature": "Bearer " + signToken(t, tokenOverrides{secret: "another-secret"}),
	}

	for name, header := range cases {
		t.Run(name, func(t *testing.T) {
			resolver := &stubResolver{principal: auth.Principal{UserID: uuid.New()}}
			called := false
			handler := auth.RequireAuth(newVerifier(t), resolver)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				called = true
			}))

			req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
			if header != "" {
				req.Header.Set("Authorization", header)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if called {
				t.Error("protected handler ran for an unauthenticated request")
			}
			if resolver.calls != 0 {
				t.Error("resolver was called for an unauthenticated request")
			}
			if got := rec.Header().Get("WWW-Authenticate"); got == "" {
				t.Error("missing WWW-Authenticate header")
			}

			var body httpx.ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("response is not the error envelope: %v (%s)", err, rec.Body.String())
			}
			if body.Error.Code != httpx.CodeUnauthenticated {
				t.Errorf("error code = %q, want %q", body.Error.Code, httpx.CodeUnauthenticated)
			}
		})
	}
}

func TestRequireAuthReturnsInternalWhenResolverFails(t *testing.T) {
	resolver := &stubResolver{err: errors.New("database is down")}
	handler := auth.RequireAuth(newVerifier(t), resolver)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("protected handler ran despite a resolver failure")
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+signToken(t, tokenOverrides{}))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	// The internal cause must not leak to the client.
	if body := rec.Body.String(); strings.Contains(body, "database is down") {
		t.Errorf("response leaked the internal error: %s", body)
	}
}

func TestPrincipalFromEmptyContext(t *testing.T) {
	if _, ok := auth.PrincipalFrom(context.Background()); ok {
		t.Error("PrincipalFrom returned a principal for an unauthenticated context")
	}

	defer func() {
		if recover() == nil {
			t.Error("MustPrincipal did not panic outside an authenticated request")
		}
	}()
	auth.MustPrincipal(context.Background())
}
