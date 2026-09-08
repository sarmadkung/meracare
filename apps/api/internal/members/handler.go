package members

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/genxcare/api/internal/authz"
	"github.com/genxcare/api/internal/care"
	"github.com/genxcare/api/internal/relationships"
	"github.com/genxcare/api/pkg/httpx"
	"github.com/genxcare/api/pkg/validation"
)

// relationshipIDParam is the route parameter naming a membership.
const relationshipIDParam = "relationshipID"

// Handler exposes the care-circle member endpoints, mounted under a senior.
type Handler struct {
	service *Service
	guard   *authz.Guard
}

// NewHandler builds the handler.
func NewHandler(service *Service, guard *authz.Guard) *Handler {
	return &Handler{service: service, guard: guard}
}

// Routes mounts `/v1/seniors/{seniorID}/members`.
func (h *Handler) Routes() chi.Router {
	router := chi.NewRouter()

	router.With(h.guard.RequirePermission(care.PermissionMembersView)).Get("/", h.list)
	router.With(h.guard.RequirePermission(care.PermissionMembersView)).Delete("/me", h.leave)

	router.Route("/{"+relationshipIDParam+"}", func(member chi.Router) {
		member.With(h.guard.RequirePermission(care.PermissionMembersManage)).Patch("/", h.update)
		member.With(h.guard.RequirePermission(care.PermissionMembersManage)).Delete("/", h.revoke)
	})

	return router
}

func (h *Handler) leave(w http.ResponseWriter, r *http.Request) {
	member := authz.MustRelationship(r.Context())
	revoked, err := h.service.Leave(r.Context(), member)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, r, http.StatusOK, relationships.ToResponse(revoked))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	relationship := authz.MustRelationship(r.Context())

	found, err := h.service.List(r.Context(), relationship.SeniorID)
	if err != nil {
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return
	}

	items := make([]relationships.MemberResponse, 0, len(found))
	for _, member := range found {
		items = append(items, relationships.ToMemberResponse(member, relationship.UserID))
	}

	httpx.WriteJSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// updateRequest is the `PATCH .../members/{id}` body.
type updateRequest struct {
	Permissions []string `json:"permissions"`
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	editor := authz.MustRelationship(r.Context())

	relationshipID, ok := parseRelationshipID(r)
	if !ok {
		notFound(w, r)
		return
	}

	var body updateRequest
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	var errs validation.Errors
	if body.Permissions == nil {
		errs.Add("permissions", "Choose what this person can do.")
	}
	if errs.Any() {
		httpx.WriteError(w, r, httpx.ErrValidation("Please check the highlighted fields.", errs))
		return
	}

	requested := make([]care.Permission, 0, len(body.Permissions))
	for _, permission := range body.Permissions {
		requested = append(requested, care.Permission(permission))
	}

	updated, err := h.service.UpdatePermissions(
		r.Context(), editor, editor.SeniorID, relationshipID, requested)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, r, http.StatusOK, relationships.ToResponse(updated))
}

func (h *Handler) revoke(w http.ResponseWriter, r *http.Request) {
	editor := authz.MustRelationship(r.Context())

	relationshipID, ok := parseRelationshipID(r)
	if !ok {
		notFound(w, r)
		return
	}

	revoked, err := h.service.Revoke(r.Context(), editor.SeniorID, relationshipID, editor.UserID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, r, http.StatusOK, relationships.ToResponse(revoked))
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		notFound(w, r)
	case errors.Is(err, ErrCannotModifySenior):
		httpx.WriteError(w, r, httpx.ErrForbidden(
			"The person this care circle is for cannot be changed or removed here."))
	case errors.Is(err, ErrCannotRemoveSelf):
		httpx.WriteError(w, r, httpx.ErrConflict(
			"Use Leave care circle to remove your own access safely."))
	case errors.Is(err, ErrLastCoordinator):
		httpx.WriteError(w, r, httpx.ErrConflict(
			"Give another person care-circle management access before you leave."))
	case errors.Is(err, ErrPermissionEscalation):
		var escalation *EscalationError
		details := map[string]string{}
		if errors.As(err, &escalation) {
			for _, permission := range escalation.Refused {
				details[string(permission)] = "You do not have this permission yourself."
			}
		}
		httpx.WriteError(w, r, httpx.ErrForbidden(
			"You can only grant permissions you have yourself.").WithDetails(details))
	default:
		httpx.WriteError(w, r, httpx.ErrInternal(err))
	}
}

// notFound matches the guard's response, so a membership in another circle is
// indistinguishable from one that does not exist.
func notFound(w http.ResponseWriter, r *http.Request) {
	httpx.WriteError(w, r, httpx.ErrNotFound("That member does not exist, or you do not have access."))
}

func parseRelationshipID(r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, relationshipIDParam))
	return id, err == nil
}
