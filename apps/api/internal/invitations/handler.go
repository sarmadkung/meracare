package invitations

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

const (
	tokenParam        = "token"
	invitationIDParam = "invitationID"
	maxEmailLength    = 254 // RFC 5321 maximum
)

// Handler exposes the invitation endpoints.
type Handler struct {
	service *Service
	guard   *authz.Guard
	// requireAuth is injected so this package does not depend on how
	// authentication is performed. The token routes need it selectively:
	// previewing an invitation must work before the recipient has an account.
	requireAuth func(http.Handler) http.Handler
}

// NewHandler builds the handler.
func NewHandler(
	service *Service,
	guard *authz.Guard,
	requireAuth func(http.Handler) http.Handler,
) *Handler {
	return &Handler{service: service, guard: guard, requireAuth: requireAuth}
}

// SeniorRoutes mounts `/v1/seniors/{seniorID}/invitations`, where the circle's
// members create and review invitations.
func (h *Handler) SeniorRoutes() chi.Router {
	router := chi.NewRouter()

	router.With(h.guard.RequirePermission(care.PermissionMembersInvite)).Post("/", h.create)
	router.With(h.guard.RequirePermission(care.PermissionMembersView)).Get("/", h.list)

	return router
}

// TokenRoutes mounts `/v1/invitations`, which the recipient uses.
//
// These are addressed by token rather than by senior, because the recipient has
// no relationship to the senior yet — that is the thing they are accepting.
func (h *Handler) TokenRoutes() chi.Router {
	router := chi.NewRouter()

	// Readable by whoever holds the token, including someone not yet signed in,
	// so an invitee can see what they are being asked to join before creating
	// an account. The payload is deliberately thin; see PreviewResponse.
	router.Get("/{"+tokenParam+"}", h.preview)

	// Accepting requires an account: the relationship must attach to a user.
	router.Group(func(authenticated chi.Router) {
		authenticated.Use(h.requireAuth)
		authenticated.Post("/{"+tokenParam+"}/accept", h.accept)
		authenticated.Post("/{"+invitationIDParam+"}/revoke", h.revoke)
	})

	return router
}

// createRequest is the `POST /v1/seniors/{id}/invitations` body.
type createRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
	// Permissions is optional. Omitted means "the defaults for this role",
	// narrowed to what the inviter may delegate. Whatever is sent is validated
	// against the inviter's own permissions — never trusted.
	Permissions []string `json:"permissions"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	inviter := authz.MustRelationship(r.Context())

	var body createRequest
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	var errs validation.Errors

	email := NormaliseEmail(body.Email)
	errs.Required("email", email)
	errs.MaxLength("email", email, maxEmailLength)
	if email != "" && !looksLikeEmail(email) {
		errs.Add("email", "Enter a valid email address.")
	}

	role := care.Role(strings.TrimSpace(body.Role))
	if !care.CanInviteRole(role) {
		errs.Add("role", "Choose whether this person is family or a professional caregiver.")
	}

	if errs.Any() {
		httpx.WriteError(w, r, httpx.ErrValidation("Please check the highlighted fields.", errs))
		return
	}

	requested := make([]care.Permission, 0, len(body.Permissions))
	for _, permission := range body.Permissions {
		requested = append(requested, care.Permission(permission))
	}

	created, err := h.service.Create(r.Context(), inviter, CreateInput{
		SeniorID:    inviter.SeniorID,
		Email:       email,
		Role:        role,
		Permissions: requested,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	// The token appears here and nowhere else: it is not stored in a
	// retrievable form and no later response can return it.
	httpx.WriteJSON(w, r, http.StatusCreated, map[string]any{
		"invitation": ToResponse(created.Invitation, h.service.Now()),
		"token":      string(created.Token),
	})
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	relationship := authz.MustRelationship(r.Context())

	found, err := h.service.List(r.Context(), relationship.SeniorID)
	if err != nil {
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return
	}

	now := h.service.Now()
	items := make([]Response, 0, len(found))
	for _, invitation := range found {
		items = append(items, ToResponse(invitation, now))
	}

	httpx.WriteJSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) preview(w http.ResponseWriter, r *http.Request) {
	token := Token(chi.URLParam(r, tokenParam))

	preview, err := h.service.Preview(r.Context(), token)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, r, http.StatusOK, preview)
}

func (h *Handler) accept(w http.ResponseWriter, r *http.Request) {
	principal := auth.MustPrincipal(r.Context())
	token := Token(chi.URLParam(r, tokenParam))

	relationship, err := h.service.Accept(r.Context(), principal, token)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, r, http.StatusOK, map[string]any{
		"seniorId":    relationship.SeniorID.String(),
		"role":        string(relationship.Role),
		"permissions": relationship.Permissions.Strings(),
	})
}

// revoke withdraws a pending invitation.
//
// The invitation is addressed by its own ID, so the senior it belongs to comes
// from the record rather than the URL. Authorization therefore happens here
// rather than in middleware — through the same Guard, not a hand-rolled check.
func (h *Handler) revoke(w http.ResponseWriter, r *http.Request) {
	principal := auth.MustPrincipal(r.Context())

	invitationID, err := uuid.Parse(chi.URLParam(r, invitationIDParam))
	if err != nil {
		notFound(w, r)
		return
	}

	invitation, err := h.service.GetByID(r.Context(), invitationID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	if _, err := h.guard.Authorize(
		r.Context(), principal.UserID, invitation.SeniorID, care.PermissionMembersInvite,
	); err != nil {
		if errors.Is(err, authz.ErrDenied) {
			notFound(w, r)
			return
		}
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return
	}

	revoked, err := h.service.Revoke(r.Context(), invitationID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, r, http.StatusOK, ToResponse(revoked, h.service.Now()))
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		notFound(w, r)
	case errors.Is(err, ErrNotAcceptable):
		httpx.WriteError(w, r, httpx.ErrConflict(
			"This invitation has expired or has already been used."))
	case errors.Is(err, ErrWrongRecipient):
		httpx.WriteError(w, r, httpx.ErrForbidden(
			"This invitation was sent to a different email address."))
	case errors.Is(err, ErrAlreadyMember):
		httpx.WriteError(w, r, httpx.ErrConflict("That person is already in this care circle."))
	case errors.Is(err, ErrAlreadyInvited):
		httpx.WriteError(w, r, httpx.ErrConflict(
			"That person already has an invitation waiting. Revoke it first to send a new one."))
	case errors.Is(err, ErrRoleNotInvitable):
		httpx.WriteError(w, r, httpx.ErrBadRequest("That role cannot be invited."))
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

// notFound is the response for every unusable token, whatever the reason.
//
// An invalid token, a token for another circle, and a token that never existed
// are indistinguishable, so the endpoint cannot be used to confirm that a
// guessed token exists.
func notFound(w http.ResponseWriter, r *http.Request) {
	httpx.WriteError(w, r, httpx.ErrNotFound("That invitation could not be found."))
}

// looksLikeEmail is a deliberately loose check.
//
// The authoritative test of an address is whether mail to it arrives; rejecting
// unusual but legal addresses would lock real people out for no security gain.
func looksLikeEmail(value string) bool {
	local, domain, found := strings.Cut(value, "@")
	return found &&
		local != "" &&
		strings.Contains(domain, ".") &&
		!strings.HasPrefix(domain, ".") &&
		!strings.HasSuffix(domain, ".") &&
		!strings.ContainsAny(value, " \t\r\n")
}
