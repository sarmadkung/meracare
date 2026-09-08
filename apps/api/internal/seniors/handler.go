package seniors

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/genxcare/api/internal/auth"
	"github.com/genxcare/api/internal/authz"
	"github.com/genxcare/api/internal/care"
	"github.com/genxcare/api/internal/database"
	"github.com/genxcare/api/pkg/httpx"
	"github.com/genxcare/api/pkg/validation"
)

const (
	maxDisplayNameLength      = 120
	maxPhoneLength            = 32
	maxAddressLength          = 300
	maxEmergencyContactLength = 200
)

// Handler exposes the `/v1/seniors` endpoints.
type Handler struct {
	service *Service
	guard   *authz.Guard
}

// NewHandler builds the handler.
func NewHandler(service *Service, guard *authz.Guard) *Handler {
	return &Handler{service: service, guard: guard}
}

// SubRoutes are the senior-scoped subtrees mounted beneath `/{seniorID}`.
//
// They are passed in rather than mounted alongside, so every senior-scoped path
// shares one `{seniorID}` parameter and the guard reads the same value whatever
// the resource.
type SubRoutes struct {
	Members      chi.Router
	Invitations  chi.Router
	Tasks        chi.Router
	Medications  chi.Router
	Appointments chi.Router
	Activity     chi.Router
	Notes        chi.Router
	Messages     chi.Router
}

// Routes mounts the senior endpoints. The caller applies authentication; each
// senior-scoped route additionally passes through the authorization guard.
func (h *Handler) Routes(sub SubRoutes) chi.Router {
	router := chi.NewRouter()

	router.Get("/", h.list)
	router.Post("/", h.create)

	router.Route("/{"+authz.SeniorIDParam+"}", func(senior chi.Router) {
		senior.With(h.guard.RequirePermission(care.PermissionSeniorView)).Get("/", h.get)
		senior.With(h.guard.RequirePermission(care.PermissionSeniorEdit)).Patch("/", h.update)
		senior.With(h.guard.RequirePermission(care.PermissionMembersManage)).Delete("/", h.remove)

		if sub.Members != nil {
			senior.Mount("/members", sub.Members)
		}
		if sub.Invitations != nil {
			senior.Mount("/invitations", sub.Invitations)
		}
		if sub.Tasks != nil {
			senior.Mount("/tasks", sub.Tasks)
		}
		if sub.Medications != nil {
			senior.Mount("/medications", sub.Medications)
		}
		if sub.Appointments != nil {
			senior.Mount("/appointments", sub.Appointments)
		}
		if sub.Activity != nil {
			senior.Mount("/activity", sub.Activity)
		}
		if sub.Notes != nil {
			senior.Mount("/notes", sub.Notes)
		}
		if sub.Messages != nil {
			senior.Mount("/messages", sub.Messages)
		}
	})

	return router
}

// list returns the seniors the caller can reach. It needs no guard: the query
// is already scoped to the caller's own active relationships.
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	principal := auth.MustPrincipal(r.Context())

	memberships, err := h.service.List(r.Context(), principal)
	if err != nil {
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return
	}

	items := make([]Response, 0, len(memberships))
	for _, membership := range memberships {
		items = append(items, ToResponse(membership.Senior, membership.Relationship))
	}

	httpx.WriteJSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// createRequest is the `POST /v1/seniors` body.
type createRequest struct {
	// Mode is how the caller relates to this senior: self, family_member, or
	// professional_caregiver.
	Mode             string  `json:"mode"`
	DisplayName      string  `json:"displayName"`
	DateOfBirth      *string `json:"dateOfBirth"`
	Phone            *string `json:"phone"`
	Address          *string `json:"address"`
	EmergencyContact *string `json:"emergencyContact"`
	// Timezone is an IANA name. Omitted means UTC, which the client should
	// avoid by sending the device's zone at sign-up.
	Timezone *string `json:"timezone"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	principal := auth.MustPrincipal(r.Context())

	var body createRequest
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	var errs validation.Errors

	mode := CreateMode(strings.TrimSpace(body.Mode))
	if !mode.Valid() {
		errs.Add("mode", "Choose whether this profile is for you, a family member, or a client.")
	}

	displayName := strings.TrimSpace(body.DisplayName)
	errs.Required("displayName", displayName)
	errs.MaxLength("displayName", displayName, maxDisplayNameLength)

	dateOfBirth := parseOptionalDate(&errs, body.DateOfBirth)
	validateOptional(&errs, "phone", body.Phone, maxPhoneLength)
	validateOptional(&errs, "address", body.Address, maxAddressLength)
	validateOptional(&errs, "emergencyContact", body.EmergencyContact, maxEmergencyContactLength)
	timezone := validateTimezone(&errs, body.Timezone)

	if errs.Any() {
		httpx.WriteError(w, r, httpx.ErrValidation("Please check the highlighted fields.", errs))
		return
	}

	membership, err := h.service.Create(r.Context(), principal, CreateInput{
		Mode:             mode,
		DisplayName:      displayName,
		DateOfBirth:      dateOfBirth,
		Phone:            derefOrEmpty(body.Phone),
		Address:          derefOrEmpty(body.Address),
		EmergencyContact: derefOrEmpty(body.EmergencyContact),
		Timezone:         timezone,
	})
	if err != nil {
		if isSelfProfileConflict(err) {
			httpx.WriteError(w, r, httpx.ErrConflict("You already have a profile for yourself."))
			return
		}
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return
	}

	httpx.WriteJSON(w, r, http.StatusCreated, ToResponse(membership.Senior, membership.Relationship))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	relationship := authz.MustRelationship(r.Context())

	senior, err := h.service.Get(r.Context(), relationship.SeniorID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, r, http.StatusOK, ToResponse(senior, relationship))
}

func (h *Handler) remove(w http.ResponseWriter, r *http.Request) {
	relationship := authz.MustRelationship(r.Context())
	disposition, err := h.service.Remove(
		r.Context(), relationship.SeniorID, relationship.UserID,
	)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, r, http.StatusOK, map[string]string{"disposition": string(disposition)})
}

// updateRequest is the `PATCH /v1/seniors/{id}` body. Absent fields are
// unchanged; an explicit null clears an optional field.
type updateRequest struct {
	DisplayName      *string `json:"displayName"`
	DateOfBirth      *string `json:"dateOfBirth"`
	Phone            *string `json:"phone"`
	Address          *string `json:"address"`
	EmergencyContact *string `json:"emergencyContact"`
	Timezone         *string `json:"timezone"`
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	relationship := authz.MustRelationship(r.Context())

	var body updateRequest
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	var errs validation.Errors
	params := UpdateParams{
		Phone:            body.Phone,
		Address:          body.Address,
		EmergencyContact: body.EmergencyContact,
	}

	if body.DisplayName != nil {
		trimmed := strings.TrimSpace(*body.DisplayName)
		errs.Required("displayName", trimmed)
		errs.MaxLength("displayName", trimmed, maxDisplayNameLength)
		params.DisplayName = &trimmed
	}
	if body.DateOfBirth != nil {
		if strings.TrimSpace(*body.DateOfBirth) == "" {
			params.ClearDateOfBirth = true
		} else {
			params.DateOfBirth = parseOptionalDate(&errs, body.DateOfBirth)
		}
	}
	validateOptional(&errs, "phone", body.Phone, maxPhoneLength)
	validateOptional(&errs, "address", body.Address, maxAddressLength)
	validateOptional(&errs, "emergencyContact", body.EmergencyContact, maxEmergencyContactLength)

	if body.Timezone != nil {
		if timezone := validateTimezone(&errs, body.Timezone); timezone != "" {
			params.Timezone = &timezone
		}
	}

	if errs.Any() {
		httpx.WriteError(w, r, httpx.ErrValidation("Please check the highlighted fields.", errs))
		return
	}

	senior, err := h.service.Update(r.Context(), relationship.SeniorID, params)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, r, http.StatusOK, ToResponse(senior, relationship))
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, ErrSelfProfileRemoval) {
		httpx.WriteError(w, r, httpx.ErrForbidden(
			"Your own senior profile is managed through your account, not this client-removal action."))
		return
	}
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, r, httpx.ErrNotFound("That senior does not exist, or you do not have access."))
		return
	}
	httpx.WriteError(w, r, httpx.ErrInternal(err))
}

// parseOptionalDate reads a `YYYY-MM-DD` value, recording a validation failure
// rather than an error when it is malformed or in the future.
func parseOptionalDate(errs *validation.Errors, value *string) *time.Time {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}

	parsed, err := ParseDateOfBirth(*value)
	if err != nil {
		errs.Add("dateOfBirth", "Use the format YYYY-MM-DD.")
		return nil
	}
	if parsed.After(time.Now().UTC()) {
		errs.Add("dateOfBirth", "A date of birth cannot be in the future.")
		return nil
	}
	return &parsed
}

// validateTimezone checks an IANA name against the machine's tzdata.
//
// Rejected rather than silently defaulted: a zone the server cannot resolve
// would put every one of this senior's tasks at the wrong hour, and doing that
// quietly is worse than refusing to save the profile.
func validateTimezone(errs *validation.Errors, value *string) string {
	if value == nil {
		return ""
	}

	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return ""
	}
	if !ValidTimezone(trimmed) {
		errs.Add("timezone", "We do not recognise that timezone.")
		return ""
	}
	return trimmed
}

func validateOptional(errs *validation.Errors, field string, value *string, max int) {
	if value == nil {
		return
	}
	errs.MaxLength(field, *value, max)
}

func derefOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// isSelfProfileConflict reports whether the failure is a second Solo Mode
// profile for the same account, which the unique constraint on
// senior_profiles.user_id rejects.
func isSelfProfileConflict(err error) bool {
	return database.IsUniqueViolation(err, "senior_profiles_user_id_key")
}
