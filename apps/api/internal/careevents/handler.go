package careevents

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/genxcare/api/internal/authz"
	"github.com/genxcare/api/internal/care"
	"github.com/genxcare/api/pkg/httpx"
)

// Handler exposes the activity timeline.
//
// One endpoint, and only a read. docs/05 defines
// `GET /v1/seniors/{id}/activity?cursor=...` and nothing else, and there is
// deliberately no `POST /v1/care-events`: an activity feed a client can write to
// records what people claimed rather than what happened, and would let anybody
// with an account manufacture a history of care that was never given
// (plans/phase7.md §§11, 21).
type Handler struct {
	service *Service
}

// NewHandler builds the handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// SeniorRoutes mounts `/v1/seniors/{seniorID}/activity`.
//
// The senior is named by the URL, so the guard runs as middleware and answers
// exactly as it does for every other senior-scoped resource: a stranger and a
// member without the permission both get the same 404, and neither learns
// whether the senior has any activity at all (plans/phase7.md §22).
//
// The permission is `activity.view`, which docs/02 already defines — family
// members and professional caregivers both hold it by default, and a circle can
// withhold it from an individual relationship. No new permission was invented
// (plans/phase7.md §10).
func (h *Handler) SeniorRoutes(guard *authz.Guard) chi.Router {
	router := chi.NewRouter()

	router.With(guard.RequirePermission(care.PermissionActivityView)).Get("/", h.list)

	return router
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	relationship := authz.MustRelationship(r.Context())

	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}

	page, err := h.service.Activity(
		r.Context(),
		relationship.SeniorID,
		strings.TrimSpace(r.URL.Query().Get("cursor")),
		limit,
	)
	if err != nil {
		if errors.Is(err, ErrBadCursor) {
			httpx.WriteError(w, r, httpx.ErrBadRequest("Start again from the first page."))
			return
		}
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return
	}

	var nextCursor *string
	if page.NextCursor != "" {
		nextCursor = &page.NextCursor
	}

	httpx.WriteJSON(w, r, http.StatusOK, map[string]any{
		"items":      responses(page.Items),
		"nextCursor": nextCursor,
	})
}
