package notes

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/genxcare/api/internal/auth"
	"github.com/genxcare/api/internal/authz"
	"github.com/genxcare/api/internal/care"
	"github.com/genxcare/api/pkg/httpx"
	"github.com/genxcare/api/pkg/validation"
)

const maxContentLength = 4000

type Handler struct {
	service *Service
	guard   *authz.Guard
}

func NewHandler(service *Service, guard *authz.Guard) *Handler {
	return &Handler{service: service, guard: guard}
}

func (h *Handler) SeniorRoutes() chi.Router {
	r := chi.NewRouter()
	r.With(h.guard.RequirePermission(care.PermissionNotesView)).Get("/", h.list)
	r.With(h.guard.RequirePermission(care.PermissionNotesCreate)).Post("/", h.create)
	return r
}

func (h *Handler) NoteRoutes() chi.Router {
	r := chi.NewRouter()
	r.Patch("/{noteID}", h.update)
	return r
}

type contentRequest struct {
	Content string `json:"content"`
}

func validateContent(value string) (string, validation.Errors) {
	content := strings.TrimSpace(value)
	var errs validation.Errors
	errs.Required("content", content)
	errs.MaxLength("content", content, maxContentLength)
	return content, errs
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	rel := authz.MustRelationship(r.Context())
	principal := auth.MustPrincipal(r.Context())
	items, err := h.service.List(r.Context(), rel.SeniorID)
	if err != nil {
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return
	}
	responses := make([]Response, 0, len(items))
	for _, note := range items {
		responses = append(responses, ToResponse(note, principal.UserID.String()))
	}
	httpx.WriteJSON(w, r, http.StatusOK, map[string]any{"items": responses})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var body contentRequest
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	content, errs := validateContent(body.Content)
	if errs.Any() {
		httpx.WriteError(w, r, httpx.ErrValidation("Please check the note.", errs))
		return
	}
	rel := authz.MustRelationship(r.Context())
	principal := auth.MustPrincipal(r.Context())
	created, err := h.service.Create(r.Context(), rel.SeniorID, principal.UserID, content)
	if err != nil {
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return
	}
	httpx.WriteJSON(w, r, http.StatusCreated, ToResponse(created, principal.UserID.String()))
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "noteID"))
	if err != nil {
		httpx.WriteError(w, r, httpx.ErrNotFound("That note is not available."))
		return
	}
	note, err := h.service.Get(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	principal := auth.MustPrincipal(r.Context())
	if _, err := h.guard.Authorize(r.Context(), principal.UserID, note.SeniorID, care.PermissionNotesCreate); err != nil || note.AuthorUserID != principal.UserID {
		httpx.WriteError(w, r, httpx.ErrNotFound("That note is not available."))
		return
	}
	var body contentRequest
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	content, errs := validateContent(body.Content)
	if errs.Any() {
		httpx.WriteError(w, r, httpx.ErrValidation("Please check the note.", errs))
		return
	}
	updated, err := h.service.Update(r.Context(), id, principal.UserID, content)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, r, http.StatusOK, ToResponse(updated, principal.UserID.String()))
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, r, httpx.ErrNotFound("That note is not available."))
		return
	}
	httpx.WriteError(w, r, httpx.ErrInternal(err))
}
