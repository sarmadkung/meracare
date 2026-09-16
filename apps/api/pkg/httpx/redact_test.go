package httpx_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/genxcare/api/pkg/httpx"
	"github.com/genxcare/api/pkg/logging"
)

func TestRedactPath(t *testing.T) {
	cases := map[string]string{
		// An invitation token in the path must never be written down.
		"/v1/invitations/abc123token":        "/v1/invitations/[redacted]",
		"/v1/invitations/abc123token/accept": "/v1/invitations/[redacted]/accept",
		"/v1/invitations/abc123token/revoke": "/v1/invitations/[redacted]/revoke",
		// Everything else is left alone: IDs are not secrets and are useful.
		"/v1/seniors/9b1d/members": "/v1/seniors/9b1d/members",
		"/v1/me":                   "/v1/me",
		"/healthz":                 "/healthz",
		"/v1/invitations":          "/v1/invitations",
		"/v1/invitations/":         "/v1/invitations/",
	}

	for path, want := range cases {
		if got := httpx.RedactPath(path); got != want {
			t.Errorf("RedactPath(%q) = %q, want %q", path, got, want)
		}
	}
}

// The end-to-end property: a request carrying a token leaves no trace of it in
// the log (docs/09 forbids logging credentials).
func TestRequestLoggerDoesNotRecordInvitationTokens(t *testing.T) {
	const token = "s3cr3t-invitation-token-value"

	var buffer bytes.Buffer
	logger := logging.New(&buffer, logging.Options{Level: "info"})

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(httpx.RequestLogger(logger))
	router.Get("/v1/invitations/{token}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/invitations/"+token, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if strings.Contains(buffer.String(), token) {
		t.Fatalf("the invitation token was logged: %s", buffer.String())
	}
	if !strings.Contains(buffer.String(), "[redacted]") {
		t.Errorf("expected the path to be logged with the token redacted, got %s", buffer.String())
	}
}

// The Authorization header must never reach the log either.
func TestRequestLoggerDoesNotRecordHeaders(t *testing.T) {
	var buffer bytes.Buffer
	logger := logging.New(&buffer, logging.Options{Level: "debug"})

	handler := httpx.RequestLogger(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.Header.Set("Authorization", "Bearer super-secret-access-token")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if strings.Contains(buffer.String(), "super-secret-access-token") {
		t.Fatalf("the access token was logged: %s", buffer.String())
	}
}
