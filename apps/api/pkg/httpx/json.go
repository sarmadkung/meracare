package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/genxcare/api/pkg/logging"
)

// maxRequestBody caps decoded request bodies. MVP payloads are small forms and
// notes; anything larger is rejected rather than buffered.
const maxRequestBody = 1 << 20 // 1 MiB

// WriteJSON writes a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, r *http.Request, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if payload == nil || status == http.StatusNoContent {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		// The status line is already sent; all we can do is record it.
		logging.FromContext(r.Context()).Error("failed to encode response body", slog.Any("error", err))
	}
}

// WriteError renders err using the shared error envelope. Internal causes are
// logged server-side and stripped from the response.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	apiErr := AsAPIError(err)

	logger := logging.FromContext(r.Context())
	attrs := []any{slog.String("code", apiErr.Code), slog.Int("status", apiErr.Status)}
	if cause := apiErr.Unwrap(); cause != nil {
		attrs = append(attrs, slog.Any("error", cause))
	}
	if apiErr.Status >= http.StatusInternalServerError {
		logger.Error("request failed", attrs...)
	} else {
		logger.Info("request rejected", attrs...)
	}

	WriteJSON(w, r, apiErr.Status, ErrorResponse{Error: ErrorBody{
		Code:    apiErr.Code,
		Message: apiErr.Message,
		Details: apiErr.Details,
	}})
}

// DecodeJSON reads and strictly decodes a JSON request body into dst.
// It rejects unknown fields so client typos surface immediately.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	if r.Body == nil {
		return ErrBadRequest("A request body is required.")
	}

	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return ErrBadRequest("The request body is too large.")
		}
		if errors.Is(err, io.EOF) {
			return ErrBadRequest("A request body is required.")
		}
		// DisallowUnknownFields reports a distinct, actionable failure: the
		// client sent a field this endpoint does not accept.
		if strings.HasPrefix(err.Error(), "json: unknown field ") {
			return ErrBadRequest("The request body contains a field this endpoint does not accept.").
				WithCause(err)
		}
		return ErrBadRequest("The request body is not valid JSON.").WithCause(err)
	}

	// Reject trailing content so `{...}{...}` is not silently accepted.
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ErrBadRequest("The request body must contain a single JSON object.")
	}
	return nil
}
