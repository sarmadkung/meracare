package summary

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/meracare/api/internal/auth"
	"github.com/meracare/api/pkg/httpx"
)

// Handler exposes `GET /v1/seniors/summary`.
type Handler struct {
	service *Service
}

// NewHandler builds the handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Routes mounts the summary endpoint. The caller applies authentication.
//
// No authorization guard: like the senior list it sits beside, this is already
// scoped to the caller's own active relationships, and each senior's counts are
// filtered by that senior's permissions inside the service.
func (h *Handler) Routes() chi.Router {
	router := chi.NewRouter()
	router.Get("/", h.list)
	return router
}

// CountsResponse is a finished-out-of-total pair.
type CountsResponse struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

// Response is one senior's day.
//
// Medications and Tasks are null when the caller cannot view that domain,
// which the client must render as absence rather than as zero.
type Response struct {
	SeniorID       string          `json:"seniorId"`
	Medications    *CountsResponse `json:"medications"`
	Tasks          *CountsResponse `json:"tasks"`
	NeedsAttention int             `json:"needsAttention"`
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	principal := auth.MustPrincipal(r.Context())

	summaries, err := h.service.ForUser(r.Context(), principal, time.Now())
	if err != nil {
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return
	}

	items := make([]Response, 0, len(summaries))
	for _, entry := range summaries {
		items = append(items, Response{
			SeniorID:       entry.SeniorID.String(),
			Medications:    toCounts(entry.Medications),
			Tasks:          toCounts(entry.Tasks),
			NeedsAttention: entry.NeedsAttention,
		})
	}

	httpx.WriteJSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func toCounts(counts *Counts) *CountsResponse {
	if counts == nil {
		return nil
	}
	return &CountsResponse{Done: counts.Done, Total: counts.Total}
}
