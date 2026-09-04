package users

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/genxcare/api/internal/auth"
	"github.com/genxcare/api/pkg/httpx"
	"github.com/genxcare/api/pkg/validation"
)

const (
	maxDisplayNameLength = 80
	maxPhoneLength       = 32
	maxAvatarURLLength   = 512
)

// Handler exposes the `/v1/me` endpoints.
type Handler struct {
	service *Service
}

// NewHandler builds the handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Routes mounts the profile endpoints. The caller is responsible for applying
// the authentication middleware.
func (h *Handler) Routes() chi.Router {
	router := chi.NewRouter()
	router.Get("/", h.getMe)
	router.Patch("/", h.updateMe)
	return router
}

// getMe returns the authenticated user's profile.
func (h *Handler) getMe(w http.ResponseWriter, r *http.Request) {
	principal := auth.MustPrincipal(r.Context())

	user, err := h.service.Get(r.Context(), principal)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, r, http.StatusOK, ToResponse(user))
}

// updateMeRequest is the `PATCH /v1/me` body. Absent fields are unchanged.
type updateMeRequest struct {
	DisplayName *string `json:"displayName"`
	AvatarURL   *string `json:"avatarUrl"`
	Phone       *string `json:"phone"`
}

// updateMe applies profile changes for the authenticated user. The user ID
// comes from the verified token, never from the request body.
func (h *Handler) updateMe(w http.ResponseWriter, r *http.Request) {
	principal := auth.MustPrincipal(r.Context())

	var body updateMeRequest
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	var errs validation.Errors
	if body.DisplayName != nil {
		trimmed := strings.TrimSpace(*body.DisplayName)
		errs.Required("displayName", trimmed)
		errs.MaxLength("displayName", trimmed, maxDisplayNameLength)
		body.DisplayName = &trimmed
	}
	if body.AvatarURL != nil {
		errs.MaxLength("avatarUrl", *body.AvatarURL, maxAvatarURLLength)
	}
	if body.Phone != nil {
		errs.MaxLength("phone", *body.Phone, maxPhoneLength)
	}
	if errs.Any() {
		httpx.WriteError(w, r, httpx.ErrValidation("Please check the highlighted fields.", errs))
		return
	}

	user, err := h.service.Update(r.Context(), principal, UpdateParams{
		DisplayName: body.DisplayName,
		AvatarURL:   body.AvatarURL,
		Phone:       body.Phone,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, r, http.StatusOK, ToResponse(user))
}

// writeError maps domain errors onto the shared error envelope.
func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, r, httpx.ErrNotFound("That profile no longer exists."))
		return
	}
	httpx.WriteError(w, r, httpx.ErrInternal(err))
}
