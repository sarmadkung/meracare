package server_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/genxcare/api/internal/auth"
	"github.com/genxcare/api/internal/config"
	"github.com/genxcare/api/internal/server"
	"github.com/genxcare/api/pkg/httpx"
	"github.com/genxcare/api/pkg/logging"
)

// rejectingVerifier stands in for Supabase; these tests never present a valid
// token, so no request should reach the database.
type rejectingVerifier struct{}

func (rejectingVerifier) Verify(context.Context, string) (*auth.Claims, error) {
	return nil, auth.ErrInvalidToken
}

// newTestServer builds the router with a nil pool. That is deliberate: every
// route exercised here must answer without touching PostgreSQL.
func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	return server.New(server.Dependencies{
		Config: &config.Config{
			Env:            config.EnvTest,
			Port:           8080,
			RequestTimeout: 5 * time.Second,
		},
		Logger:   logging.New(io.Discard, logging.Options{Level: "error"}),
		Pool:     nil,
		Verifier: rejectingVerifier{},
	})
}

func TestHealthzIsPublic(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestServer(t).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want ok", body["status"])
	}
}

func TestReadyzReportsUnavailableWithoutDatabase(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestServer(t).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if code := decodeErrorCode(t, rec); code != httpx.CodeUnavailable {
		t.Errorf("code = %q, want %q", code, httpx.CodeUnavailable)
	}
}

// Every /v1 route sits behind authentication.
func TestV1RoutesRequireAuthentication(t *testing.T) {
	handler := newTestServer(t)

	for _, method := range []string{http.MethodGet, http.MethodPatch} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(method, "/v1/me", nil))

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s /v1/me status = %d, want 401", method, rec.Code)
		}
		if code := decodeErrorCode(t, rec); code != httpx.CodeUnauthenticated {
			t.Errorf("%s /v1/me code = %q, want %q", method, code, httpx.CodeUnauthenticated)
		}
	}
}

func TestUnknownRouteUsesErrorEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestServer(t).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/does-not-exist", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if code := decodeErrorCode(t, rec); code != httpx.CodeNotFound {
		t.Errorf("code = %q, want %q", code, httpx.CodeNotFound)
	}
}

// Authentication runs before routing inside /v1, so an unauthenticated caller
// cannot probe which endpoints exist.
func TestUnknownV1RouteDoesNotRevealItselfToAnonymousCallers(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestServer(t).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/does-not-exist", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRequestIDHeaderIsSet(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestServer(t).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Header().Get("X-Request-Id") == "" {
		t.Error("expected chi's RequestID middleware to set X-Request-Id")
	}
}

func decodeErrorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body httpx.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not the error envelope: %v (%s)", err, rec.Body.String())
	}
	return body.Error.Code
}
