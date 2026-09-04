package httpx_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/genxcare/api/pkg/httpx"
)

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) httpx.ErrorResponse {
	t.Helper()
	var body httpx.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not the error envelope: %v (%s)", err, rec.Body.String())
	}
	return body
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)

	httpx.WriteJSON(rec, req, http.StatusCreated, map[string]string{"id": "abc"})

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"id":"abc"}` {
		t.Errorf("body = %q", got)
	}
}

func TestWriteErrorUsesEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/seniors/abc", nil)

	httpx.WriteError(rec, req, httpx.ErrForbidden("You do not have access to this senior."))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	body := decodeError(t, rec)
	if body.Error.Code != httpx.CodeForbidden {
		t.Errorf("code = %q, want %q", body.Error.Code, httpx.CodeForbidden)
	}
	if body.Error.Message != "You do not have access to this senior." {
		t.Errorf("message = %q", body.Error.Message)
	}
}

// An unexpected error must become a generic 500 that reveals nothing about the
// internals (docs/09-security-privacy.md).
func TestWriteErrorHidesInternalCause(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)

	httpx.WriteError(rec, req, errors.New(`pq: relation "users" does not exist`))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	body := decodeError(t, rec)
	if body.Error.Code != httpx.CodeInternal {
		t.Errorf("code = %q, want %q", body.Error.Code, httpx.CodeInternal)
	}
	if strings.Contains(rec.Body.String(), "relation") {
		t.Errorf("response leaked the database error: %s", rec.Body.String())
	}
}

func TestValidationErrorCarriesDetails(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/v1/me", nil)

	httpx.WriteError(rec, req, httpx.ErrValidation("Please check the highlighted fields.", map[string]string{
		"displayName": "This field is required.",
	}))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	body := decodeError(t, rec)
	if body.Error.Details["displayName"] != "This field is required." {
		t.Errorf("details = %v", body.Error.Details)
	}
}

func TestAsAPIErrorUnwrapsWrappedAPIErrors(t *testing.T) {
	wrapped := errors.Join(errors.New("context"), httpx.ErrNotFound("Gone."))

	apiErr := httpx.AsAPIError(wrapped)
	if apiErr.Code != httpx.CodeNotFound {
		t.Errorf("code = %q, want %q", apiErr.Code, httpx.CodeNotFound)
	}
}

func TestAPIErrorWithCauseKeepsCauseOutOfMessage(t *testing.T) {
	cause := errors.New("connection refused")
	apiErr := httpx.ErrUnavailable("The service is not ready.").WithCause(cause)

	if !errors.Is(apiErr, cause) {
		t.Error("WithCause should keep the cause reachable via errors.Is")
	}
	if apiErr.Message != "The service is not ready." {
		t.Errorf("client message changed to %q", apiErr.Message)
	}
}

func TestDecodeJSON(t *testing.T) {
	type payload struct {
		DisplayName string `json:"displayName"`
	}

	cases := map[string]struct {
		body        string
		wantErr     bool
		wantValue   string
		wantMessage string
	}{
		"valid":      {body: `{"displayName":"Sara"}`, wantValue: "Sara"},
		"empty body": {body: ``, wantErr: true, wantMessage: "A request body is required."},
		"malformed":  {body: `{"displayName":`, wantErr: true},
		"unknown field": {
			body:        `{"nickname":"Sara"}`,
			wantErr:     true,
			wantMessage: "The request body contains a field this endpoint does not accept.",
		},
		"trailing object": {body: `{"displayName":"Sara"}{"displayName":"Ahmed"}`, wantErr: true},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPatch, "/v1/me", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()

			var dst payload
			err := httpx.DecodeJSON(rec, req, &dst)

			if tc.wantErr {
				if err == nil {
					t.Fatal("DecodeJSON accepted an invalid body")
				}
				apiErr := httpx.AsAPIError(err)
				if apiErr.Status != http.StatusBadRequest {
					t.Errorf("status = %d, want 400", apiErr.Status)
				}
				if tc.wantMessage != "" && apiErr.Message != tc.wantMessage {
					t.Errorf("message = %q, want %q", apiErr.Message, tc.wantMessage)
				}
				return
			}
			if err != nil {
				t.Fatalf("DecodeJSON error = %v", err)
			}
			if dst.DisplayName != tc.wantValue {
				t.Errorf("displayName = %q, want %q", dst.DisplayName, tc.wantValue)
			}
		})
	}
}

func TestRecovererReturnsErrorEnvelope(t *testing.T) {
	handler := httpx.Recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/me", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if body := decodeError(t, rec); body.Error.Code != httpx.CodeInternal {
		t.Errorf("code = %q, want %q", body.Error.Code, httpx.CodeInternal)
	}
	if strings.Contains(rec.Body.String(), "boom") {
		t.Errorf("response leaked the panic value: %s", rec.Body.String())
	}
}

func TestCORSOnlyAnswersAllowedOrigins(t *testing.T) {
	handler := httpx.CORS([]string{"https://genxcare.app"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("allowed origin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
		req.Header.Set("Origin", "https://genxcare.app")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://genxcare.app" {
			t.Errorf("Access-Control-Allow-Origin = %q", got)
		}
	})

	t.Run("disallowed origin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
		req.Header.Set("Origin", "https://attacker.example")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("Access-Control-Allow-Origin = %q, want empty", got)
		}
	})

	t.Run("preflight", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/v1/me", nil)
		req.Header.Set("Origin", "https://genxcare.app")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Errorf("status = %d, want 204", rec.Code)
		}
	})
}
