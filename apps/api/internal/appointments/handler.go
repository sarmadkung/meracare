package appointments

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/genxcare/api/internal/auth"
	"github.com/genxcare/api/internal/authz"
	"github.com/genxcare/api/internal/care"
	"github.com/genxcare/api/pkg/httpx"
	"github.com/genxcare/api/pkg/validation"
)

const (
	appointmentIDParam = "appointmentID"

	maxTitleLength    = 140
	maxProviderLength = 140
	maxLocationLength = 280
	maxNotesLength    = 2000
)

// Handler exposes the appointment endpoints.
type Handler struct {
	service *Service
	guard   *authz.Guard
}

// NewHandler builds the handler.
func NewHandler(service *Service, guard *authz.Guard) *Handler {
	return &Handler{service: service, guard: guard}
}

// SeniorRoutes mounts `/v1/seniors/{seniorID}/appointments`, where the senior is
// named by the URL and the guard can run as middleware.
func (h *Handler) SeniorRoutes() chi.Router {
	router := chi.NewRouter()

	router.With(h.guard.RequirePermission(care.PermissionAppointmentsView)).Get("/", h.list)
	router.With(h.guard.RequirePermission(care.PermissionAppointmentsManage)).Post("/", h.create)

	return router
}

// AppointmentRoutes mounts `/v1/appointments`, where an appointment is named by
// its own ID.
//
// These cannot use the guard as middleware: the senior is not in the URL, and
// is only known once the appointment has been loaded. Each handler therefore
// resolves the appointment first and authorizes against the senior it belongs
// to — and answers exactly as the middleware would when that fails.
//
// Cancelling and completing both require appointments.manage. docs/02 defines
// two appointment permissions and no third, and plans/phase6.md §11 is explicit
// that the existing vocabulary is the one to use rather than a new one invented
// here. A circle that wants a visiting caregiver to close off appointments
// grants them manage on that relationship, which is what "permissions belong to
// the relationship" means.
func (h *Handler) AppointmentRoutes() chi.Router {
	router := chi.NewRouter()

	router.Route("/{"+appointmentIDParam+"}", func(appointment chi.Router) {
		appointment.Get("/", h.get)
		appointment.Patch("/", h.update)
		appointment.Post("/cancel", h.act(ActionCancel))
		appointment.Post("/complete", h.act(ActionComplete))
	})

	return router
}

// --- Reading ---------------------------------------------------------------

// list answers `GET /v1/seniors/{id}/appointments`.
//
// One endpoint rather than several: docs/05 defines exactly this route, and a
// separate `/history` would be a second way to ask the same question
// (plans/phase6.md §14). The view is chosen by `scope`, and `nextCursor` is
// present for every scope — null where the view is not paged, so a client
// reading the envelope never has to know which is which.
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	relationship := authz.MustRelationship(r.Context())
	now := time.Now()

	scope := Scope(strings.TrimSpace(r.URL.Query().Get("scope")))
	if scope == "" {
		scope = ScopeUpcoming
	}

	if scope == ScopePast {
		h.listHistory(w, r, relationship.SeniorID, now)
		return
	}

	found, err := h.service.List(r.Context(), relationship.SeniorID, scope, now)
	if err != nil {
		if errors.Is(err, ErrBadScope) {
			httpx.WriteError(w, r, httpx.ErrBadRequest(
				"Ask for today's appointments, the upcoming ones, or the past ones."))
			return
		}
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return
	}

	httpx.WriteJSON(w, r, http.StatusOK, map[string]any{
		"items":      responses(found),
		"nextCursor": nil,
	})
}

func (h *Handler) listHistory(
	w http.ResponseWriter,
	r *http.Request,
	seniorID uuid.UUID,
	now time.Time,
) {
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}

	page, err := h.service.History(
		r.Context(), seniorID, strings.TrimSpace(r.URL.Query().Get("cursor")), limit, now,
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

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	appointment, ok := h.authorized(w, r, care.PermissionAppointmentsView)
	if !ok {
		return
	}
	httpx.WriteJSON(w, r, http.StatusOK, ToResponse(appointment))
}

// --- Writing ---------------------------------------------------------------

// createRequest is the `POST /v1/seniors/{id}/appointments` body.
//
// There is no creator field, and there never will be: httpx.DecodeJSON refuses
// unknown fields, so a body naming `createdBy` is rejected outright rather than
// silently ignored (plans/phase6.md §13).
type createRequest struct {
	Title        string  `json:"title"`
	Kind         *string `json:"kind"`
	ProviderName *string `json:"providerName"`
	Location     *string `json:"location"`
	Notes        *string `json:"notes"`

	AssignedUserID *string `json:"assignedUserId"`

	ScheduledAt string  `json:"scheduledAt"`
	EndsAt      *string `json:"endsAt"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	relationship := authz.MustRelationship(r.Context())
	principal := auth.MustPrincipal(r.Context())

	var body createRequest
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	var errs validation.Errors

	title := strings.TrimSpace(body.Title)
	errs.Required("title", title)
	errs.MaxLength("title", title, maxTitleLength)

	provider := trimmed(&errs, "providerName", body.ProviderName, maxProviderLength)
	location := trimmed(&errs, "location", body.Location, maxLocationLength)
	notes := trimmed(&errs, "notes", body.Notes, maxNotesLength)

	scheduledAt, ok := parseInstant(body.ScheduledAt)
	if !ok {
		errs.Add("scheduledAt", "Give the date and time as an ISO-8601 timestamp.")
	}

	input := CreateInput{
		SeniorID:        relationship.SeniorID,
		CreatedByUserID: principal.UserID,
		Title:           title,
		Kind:            parseKind(&errs, body.Kind),
		ProviderName:    provider,
		Location:        location,
		Notes:           notes,
		AssignedUserID:  parseAssignee(&errs, body.AssignedUserID),
		ScheduledAt:     scheduledAt,
		EndsAt:          parseEnd(&errs, body.EndsAt, scheduledAt, ok),
	}

	if errs.Any() {
		httpx.WriteError(w, r, httpx.ErrValidation("Please check the highlighted fields.", errs))
		return
	}

	created, err := h.service.Create(r.Context(), input)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, r, http.StatusCreated, ToResponse(created))
}

// updateRequest is the `PATCH /v1/appointments/{id}` body. An absent field is
// unchanged; the clear flags remove a value, which absence alone cannot express.
type updateRequest struct {
	Title        *string `json:"title"`
	Kind         *string `json:"kind"`
	ClearKind    bool    `json:"clearKind"`
	ProviderName *string `json:"providerName"`
	Location     *string `json:"location"`
	Notes        *string `json:"notes"`

	AssignedUserID *string `json:"assignedUserId"`
	ClearAssignee  bool    `json:"clearAssignee"`

	ScheduledAt *string `json:"scheduledAt"`
	EndsAt      *string `json:"endsAt"`
	ClearEndsAt bool    `json:"clearEndsAt"`
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	appointment, ok := h.authorized(w, r, care.PermissionAppointmentsManage)
	if !ok {
		return
	}

	var body updateRequest
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	var errs validation.Errors
	input := UpdateInput{
		AppointmentID:  appointment.ID,
		SeniorID:       appointment.SeniorID,
		ProviderName:   trimmedOptional(&errs, "providerName", body.ProviderName, maxProviderLength),
		Location:       trimmedOptional(&errs, "location", body.Location, maxLocationLength),
		Notes:          trimmedOptional(&errs, "notes", body.Notes, maxNotesLength),
		ClearKind:      body.ClearKind,
		ClearEndsAt:    body.ClearEndsAt,
		AssignedUserID: parseAssignee(&errs, body.AssignedUserID),
		ClearAssignee:  body.ClearAssignee,
	}

	if body.Title != nil {
		title := strings.TrimSpace(*body.Title)
		errs.Required("title", title)
		errs.MaxLength("title", title, maxTitleLength)
		input.Title = &title
	}
	if body.Kind != nil {
		if kind := parseKind(&errs, body.Kind); kind != "" {
			input.Kind = &kind
		}
	}

	// The start time an edit is validated against is the one the appointment
	// will have when it is saved, which is the new one if the request moved it
	// and the stored one otherwise. Checking the end against a start the request
	// is in the middle of changing would reject a perfectly good move.
	start := appointment.ScheduledAt
	if body.ScheduledAt != nil {
		moved, valid := parseInstant(*body.ScheduledAt)
		if !valid {
			errs.Add("scheduledAt", "Give the date and time as an ISO-8601 timestamp.")
		} else {
			input.ScheduledAt = &moved
			start = moved
		}
	}
	if body.EndsAt != nil {
		input.EndsAt = parseEnd(&errs, body.EndsAt, start, true)
	}

	if errs.Any() {
		httpx.WriteError(w, r, httpx.ErrValidation("Please check the highlighted fields.", errs))
		return
	}

	updated, err := h.service.Update(r.Context(), input)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, r, http.StatusOK, ToResponse(updated))
}

// act builds the handler for cancelling or completing an appointment.
//
// Neither takes a body. There is nothing a client could usefully say that the
// session does not already establish, and no field it could send that would be
// trusted (plans/phase6.md §§10, 13).
func (h *Handler) act(action Action) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		appointment, ok := h.authorized(w, r, care.PermissionAppointmentsManage)
		if !ok {
			return
		}

		principal := auth.MustPrincipal(r.Context())

		result, err := h.service.Act(r.Context(), ActInput{
			AppointmentID: appointment.ID,
			Action:        action,
			ActorID:       principal.UserID,
		})
		if err != nil {
			h.writeError(w, r, err)
			return
		}

		httpx.WriteJSON(w, r, http.StatusOK, ToResponse(result.Appointment))
	}
}

// --- Authorization ---------------------------------------------------------

// authorized loads the appointment named in the URL and authorizes the caller
// against the senior it belongs to.
//
// Every failure — a malformed ID, an appointment that does not exist, no
// relationship to its senior, or a missing permission — produces the same 404.
// Anything else would let somebody learn that another family's relative has a
// cardiology appointment, by watching the answers change
// (plans/phase6.md §12).
func (h *Handler) authorized(
	w http.ResponseWriter,
	r *http.Request,
	permissions ...care.Permission,
) (Appointment, bool) {
	principal := auth.MustPrincipal(r.Context())

	appointmentID, err := uuid.Parse(chi.URLParam(r, appointmentIDParam))
	if err != nil {
		notFound(w, r)
		return Appointment{}, false
	}

	appointment, err := h.service.Get(r.Context(), appointmentID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			notFound(w, r)
			return Appointment{}, false
		}
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return Appointment{}, false
	}

	if _, err := h.guard.Authorize(
		r.Context(), principal.UserID, appointment.SeniorID, permissions...,
	); err != nil {
		if errors.Is(err, authz.ErrDenied) {
			notFound(w, r)
			return Appointment{}, false
		}
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return Appointment{}, false
	}

	return appointment, true
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		notFound(w, r)

	case errors.Is(err, ErrInvalidAssignee):
		httpx.WriteError(w, r, httpx.ErrValidation(
			"Please check the highlighted fields.",
			map[string]string{"assignedUserId": "Choose somebody in this care circle."},
		))

	case errors.Is(err, ErrSettled):
		httpx.WriteError(w, r, httpx.ErrConflict(
			"This appointment has already happened or been called off, so it cannot be changed."))

	case errors.Is(err, ErrInvalidTransition):
		httpx.WriteError(w, r, httpx.ErrConflict(
			"Somebody has already recorded a different outcome for this appointment."))

	default:
		httpx.WriteError(w, r, httpx.ErrInternal(err))
	}
}

// notFound matches the guard's wording, so an inaccessible appointment and a
// non-existent one are indistinguishable.
func notFound(w http.ResponseWriter, r *http.Request) {
	httpx.WriteError(w, r, httpx.ErrNotFound(
		"That appointment does not exist, or you do not have access."))
}

// --- Parsing ---------------------------------------------------------------

// parseKind reads an optional appointment kind, recording a validation failure
// rather than storing a value the database would refuse.
func parseKind(errs *validation.Errors, value *string) Kind {
	if value == nil || strings.TrimSpace(*value) == "" {
		return ""
	}

	kind := Kind(strings.ToLower(strings.TrimSpace(*value)))
	if !kind.Valid() {
		errs.Add("kind", "Choose one of the listed kinds of appointment.")
		return ""
	}
	return kind
}

// parseAssignee reads an optional assignee. Whether the person is actually in
// the circle is the service's question, not this one's.
func parseAssignee(errs *validation.Errors, value *string) *uuid.UUID {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}

	assignee, err := uuid.Parse(strings.TrimSpace(*value))
	if err != nil {
		errs.Add("assignedUserId", "Choose somebody in this care circle.")
		return nil
	}
	return &assignee
}

// parseEnd reads an optional end time and checks it against the start.
//
// startKnown says whether the start parsed: an end time cannot be compared with
// a start that is itself unreadable, and reporting both fields as wrong when
// only one is would send somebody looking in the wrong place.
func parseEnd(
	errs *validation.Errors,
	value *string,
	start time.Time,
	startKnown bool,
) *time.Time {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}

	end, ok := parseInstant(*value)
	if !ok {
		errs.Add("endsAt", "Give the end time as an ISO-8601 timestamp.")
		return nil
	}
	if startKnown && !end.After(start) {
		errs.Add("endsAt", "The appointment has to end after it starts.")
		return nil
	}
	return &end
}

// trimmed reads an optional free-text field for a create, where an absent field
// and an empty one mean the same thing.
func trimmed(errs *validation.Errors, field string, value *string, maxLength int) string {
	if value == nil {
		return ""
	}

	text := strings.TrimSpace(*value)
	errs.MaxLength(field, text, maxLength)
	return text
}

// trimmedOptional reads an optional free-text field for an edit, where absence
// means "leave it alone" and so must stay distinguishable from empty.
func trimmedOptional(
	errs *validation.Errors,
	field string,
	value *string,
	maxLength int,
) *string {
	if value == nil {
		return nil
	}

	text := strings.TrimSpace(*value)
	errs.MaxLength(field, text, maxLength)
	return &text
}

func parseInstant(value string) (time.Time, bool) {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	return parsed, err == nil
}
