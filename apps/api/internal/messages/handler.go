package messages

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/genxcare/api/internal/auth"
	"github.com/genxcare/api/internal/authz"
	"github.com/genxcare/api/internal/care"
	"github.com/genxcare/api/pkg/httpx"
	"github.com/genxcare/api/pkg/validation"
)

const maxContentLength = 2000

type Handler struct {
	service *Service
	guard   *authz.Guard
}

func NewHandler(service *Service, guard *authz.Guard) *Handler {
	return &Handler{service: service, guard: guard}
}

func (h *Handler) SeniorRoutes() chi.Router {
	r := chi.NewRouter()
	r.Use(h.guard.RequirePermission(care.PermissionMessagesParticipate))
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Post("/read", h.markRead)
	return r
}

type createRequest struct {
	Content string `json:"content"`
}
type readRequest struct {
	ThroughMessageID string `json:"throughMessageId"`
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	rel := authz.MustRelationship(r.Context())
	principal := auth.MustPrincipal(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	page, err := h.service.List(r.Context(), rel.SeniorID, principal.UserID,
		strings.TrimSpace(r.URL.Query().Get("cursor")), limit)
	if err != nil {
		if errors.Is(err, ErrBadCursor) {
			httpx.WriteError(w, r, httpx.ErrBadRequest("Start again from the first page."))
			return
		}
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return
	}
	items := make([]Response, 0, len(page.Items))
	for _, message := range page.Items {
		items = append(items, ToResponse(message, principal.UserID.String()))
	}
	var next *string
	if page.NextCursor != "" {
		next = &page.NextCursor
	}
	httpx.WriteJSON(w, r, http.StatusOK, map[string]any{
		"items": items, "nextCursor": next, "unreadCount": page.UnreadCount,
	})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var body createRequest
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	content := strings.TrimSpace(body.Content)
	var errs validation.Errors
	errs.Required("content", content)
	errs.MaxLength("content", content, maxContentLength)
	if errs.Any() {
		httpx.WriteError(w, r, httpx.ErrValidation("Please check the message.", errs))
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

func (h *Handler) markRead(w http.ResponseWriter, r *http.Request) {
	var body readRequest
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	messageID, err := uuid.Parse(strings.TrimSpace(body.ThroughMessageID))
	if err != nil {
		httpx.WriteError(w, r, httpx.ErrValidation("Please check the message.",
			validation.Errors{"throughMessageId": "Choose a valid message."}))
		return
	}
	rel := authz.MustRelationship(r.Context())
	principal := auth.MustPrincipal(r.Context())
	if err := h.service.MarkRead(r.Context(), rel.SeniorID, principal.UserID, messageID); err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.WriteError(w, r, httpx.ErrNotFound("That message is not available."))
			return
		}
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
