package medications

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
	"github.com/genxcare/api/internal/recurrence"
	"github.com/genxcare/api/pkg/httpx"
	"github.com/genxcare/api/pkg/validation"
)

const (
	medicationIDParam = "medicationID"
	scheduleIDParam   = "scheduleID"
	instanceIDParam   = "instanceID"

	maxNameLength         = 140
	maxDosageLength       = 140
	maxInstructionsLength = 2000
	maxNotesLength        = 2000
	maxSchedulesPerCreate = 12
)

// Handler exposes the medication endpoints.
type Handler struct {
	service *Service
	guard   *authz.Guard
}

// NewHandler builds the handler.
func NewHandler(service *Service, guard *authz.Guard) *Handler {
	return &Handler{service: service, guard: guard}
}

// SeniorRoutes mounts `/v1/seniors/{seniorID}/medications`, where the senior is
// named by the URL and the guard can run as middleware.
func (h *Handler) SeniorRoutes() chi.Router {
	router := chi.NewRouter()

	router.With(h.guard.RequirePermission(care.PermissionMedicationsView)).Get("/", h.list)
	router.With(h.guard.RequirePermission(care.PermissionMedicationsManage)).Post("/", h.create)

	// Today's medication, the coming days, and what has been missed: the doses
	// across every one of this senior's medicines, which is how the screen
	// reads them (plans/phase5.md §14).
	router.With(h.guard.RequirePermission(care.PermissionMedicationsView)).Get("/doses", h.listDoses)

	return router
}

// MedicationRoutes mounts `/v1/medications`, where a medication is named by its
// own ID.
//
// These cannot use the guard as middleware: the senior is not in the URL, and
// is only known once the medication has been loaded. Each handler therefore
// resolves the medication first and authorizes against the senior it belongs to
// — and answers exactly as the middleware would when that fails.
func (h *Handler) MedicationRoutes() chi.Router {
	router := chi.NewRouter()

	router.Route("/{"+medicationIDParam+"}", func(medication chi.Router) {
		medication.Get("/", h.get)
		medication.Patch("/", h.update)

		medication.Get("/schedules", h.listSchedules)
		medication.Post("/schedules", h.addSchedule)
		medication.Patch("/schedules/{"+scheduleIDParam+"}", h.updateSchedule)

		medication.Post("/doses", h.addDose)
		medication.Get("/instances", h.history)
		medication.Post("/instances/{"+instanceIDParam+"}/take", h.act(ActionTake))
		medication.Post("/instances/{"+instanceIDParam+"}/skip", h.act(ActionSkip))
	})

	return router
}

// --- Medications -----------------------------------------------------------

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	relationship := authz.MustRelationship(r.Context())

	found, err := h.service.List(r.Context(), relationship.SeniorID)
	if err != nil {
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return
	}

	items := make([]MedicationResponse, 0, len(found))
	for _, medication := range found {
		items = append(items, ToMedicationResponse(medication))
	}

	httpx.WriteJSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// createRequest is the `POST /v1/seniors/{id}/medications` body.
type createRequest struct {
	Name         string  `json:"name"`
	Dosage       *string `json:"dosage"`
	Form         *string `json:"form"`
	Instructions *string `json:"instructions"`
	Notes        *string `json:"notes"`

	// Schedules may be empty: a medicine can be recorded before anybody has
	// decided when it is taken.
	Schedules []scheduleRequest `json:"schedules"`
}

type scheduleRequest struct {
	Recurrence    recurrenceRequest `json:"recurrence"`
	ScheduledTime string            `json:"scheduledTime"`
}

type recurrenceRequest struct {
	Frequency string   `json:"frequency"`
	Weekdays  []string `json:"weekdays"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	relationship := authz.MustRelationship(r.Context())
	principal := auth.MustPrincipal(r.Context())
	now := time.Now()

	var body createRequest
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	var errs validation.Errors

	name := strings.TrimSpace(body.Name)
	errs.Required("name", name)
	errs.MaxLength("name", name, maxNameLength)

	dosage := strings.TrimSpace(derefOrEmpty(body.Dosage))
	errs.MaxLength("dosage", dosage, maxDosageLength)

	instructions := strings.TrimSpace(derefOrEmpty(body.Instructions))
	errs.MaxLength("instructions", instructions, maxInstructionsLength)

	notes := strings.TrimSpace(derefOrEmpty(body.Notes))
	errs.MaxLength("notes", notes, maxNotesLength)

	input := CreateInput{
		SeniorID:        relationship.SeniorID,
		CreatedByUserID: principal.UserID,
		Name:            name,
		Dosage:          dosage,
		Form:            parseForm(&errs, body.Form),
		Instructions:    instructions,
		Notes:           notes,
		Schedules:       parseSchedules(&errs, body.Schedules),
	}

	if errs.Any() {
		httpx.WriteError(w, r, httpx.ErrValidation("Please check the highlighted fields.", errs))
		return
	}

	created, err := h.service.Create(r.Context(), input, now)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, r, http.StatusCreated, map[string]any{
		"medication": ToMedicationDetailResponse(created.Medication, created.Schedules, nil),
		"doses":      instanceResponses(created.Instances, now),
	})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	medication, ok := h.authorizedMedication(w, r, care.PermissionMedicationsView)
	if !ok {
		return
	}
	h.writeDetail(w, r, medication)
}

// updateRequest is the `PATCH /v1/medications/{id}` body. An absent field is
// unchanged; a null form alongside clearForm removes it.
type updateRequest struct {
	Name         *string `json:"name"`
	Dosage       *string `json:"dosage"`
	Form         *string `json:"form"`
	ClearForm    bool    `json:"clearForm"`
	Instructions *string `json:"instructions"`
	Notes        *string `json:"notes"`
	// Active false stops the medicine. Its doses are kept
	// (plans/phase5.md §13).
	Active *bool `json:"active"`
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	medication, ok := h.authorizedMedication(w, r, care.PermissionMedicationsManage)
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
		MedicationID: medication.ID,
		Dosage:       trimmedOptional(&errs, "dosage", body.Dosage, maxDosageLength),
		Instructions: trimmedOptional(&errs, "instructions", body.Instructions, maxInstructionsLength),
		Notes:        trimmedOptional(&errs, "notes", body.Notes, maxNotesLength),
		Active:       body.Active,
		ClearForm:    body.ClearForm,
	}

	if body.Name != nil {
		name := strings.TrimSpace(*body.Name)
		errs.Required("name", name)
		errs.MaxLength("name", name, maxNameLength)
		input.Name = &name
	}
	if body.Form != nil {
		form := parseForm(&errs, body.Form)
		if form != "" {
			input.Form = &form
		}
	}

	if errs.Any() {
		httpx.WriteError(w, r, httpx.ErrValidation("Please check the highlighted fields.", errs))
		return
	}

	updated, err := h.service.Update(r.Context(), input, time.Now())
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	h.writeDetail(w, r, updated)
}

// writeDetail answers with a medication, its times, and its next dose.
func (h *Handler) writeDetail(w http.ResponseWriter, r *http.Request, medication Medication) {
	schedules, err := h.service.Schedules(r.Context(), medication.ID)
	if err != nil {
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return
	}

	nextDose, err := h.service.NextDose(r.Context(), medication, time.Now())
	if err != nil {
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return
	}

	httpx.WriteJSON(w, r, http.StatusOK,
		ToMedicationDetailResponse(medication, schedules, nextDose))
}

// --- Schedules -------------------------------------------------------------

func (h *Handler) listSchedules(w http.ResponseWriter, r *http.Request) {
	medication, ok := h.authorizedMedication(w, r, care.PermissionMedicationsView)
	if !ok {
		return
	}

	schedules, err := h.service.Schedules(r.Context(), medication.ID)
	if err != nil {
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return
	}

	items := make([]ScheduleResponse, 0, len(schedules))
	for _, schedule := range schedules {
		items = append(items, ToScheduleResponse(schedule))
	}

	httpx.WriteJSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) addSchedule(w http.ResponseWriter, r *http.Request) {
	medication, ok := h.authorizedMedication(w, r, care.PermissionMedicationsManage)
	if !ok {
		return
	}

	var body scheduleRequest
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	var errs validation.Errors
	parsed := parseSchedule(&errs, "", body)

	if errs.Any() {
		httpx.WriteError(w, r, httpx.ErrValidation("Please check the highlighted fields.", errs))
		return
	}

	schedule, err := h.service.AddSchedule(r.Context(), medication, parsed, time.Now())
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, r, http.StatusCreated, ToScheduleResponse(schedule))
}

// updateScheduleRequest is the `PATCH .../schedules/{id}` body.
type updateScheduleRequest struct {
	Recurrence    *recurrenceRequest `json:"recurrence"`
	ScheduledTime *string            `json:"scheduledTime"`
	// Active false stops this time of day without touching the others.
	Active *bool `json:"active"`
}

func (h *Handler) updateSchedule(w http.ResponseWriter, r *http.Request) {
	medication, ok := h.authorizedMedication(w, r, care.PermissionMedicationsManage)
	if !ok {
		return
	}

	scheduleID, err := uuid.Parse(chi.URLParam(r, scheduleIDParam))
	if err != nil {
		notFound(w, r)
		return
	}

	// The schedule must belong to the medication the caller was authorized for,
	// so a schedule ID from another circle is unreachable even with permission
	// here.
	schedule, err := h.service.GetSchedule(r.Context(), scheduleID)
	if err != nil || schedule.MedicationID != medication.ID {
		notFound(w, r)
		return
	}

	var body updateScheduleRequest
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	var errs validation.Errors
	input := UpdateScheduleInput{ScheduleID: schedule.ID, Active: body.Active}

	if body.Recurrence != nil {
		rule, err := recurrence.ParseRequest(body.Recurrence.Frequency, body.Recurrence.Weekdays)
		if err != nil {
			errs.Add("recurrence", recurrence.Message(err))
		} else {
			input.Recurrence = &rule
		}
	}
	if body.ScheduledTime != nil {
		at, err := recurrence.ParseTimeOfDay(*body.ScheduledTime)
		if err != nil {
			errs.Add("scheduledTime", "Use a time like 08:00.")
		} else {
			input.ScheduledTime = &at
		}
	}

	if errs.Any() {
		httpx.WriteError(w, r, httpx.ErrValidation("Please check the highlighted fields.", errs))
		return
	}

	updated, err := h.service.UpdateSchedule(r.Context(), medication, input, time.Now())
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, r, http.StatusOK, ToScheduleResponse(updated))
}

// --- Doses -----------------------------------------------------------------

func (h *Handler) listDoses(w http.ResponseWriter, r *http.Request) {
	relationship := authz.MustRelationship(r.Context())
	now := time.Now()

	input := ListDosesInput{SeniorID: relationship.SeniorID, Scope: ScopeToday}

	if scope := strings.TrimSpace(r.URL.Query().Get("scope")); scope != "" {
		input.Scope = Scope(scope)
	}

	if input.Scope == ScopeWindow {
		from, okFrom := parseInstant(r.URL.Query().Get("from"))
		to, okTo := parseInstant(r.URL.Query().Get("to"))
		if !okFrom || !okTo {
			httpx.WriteError(w, r, httpx.ErrBadRequest(
				"Give both from and to as ISO-8601 timestamps."))
			return
		}
		input.From, input.To = from, to
	}

	doses, err := h.service.ListDoses(r.Context(), input, now)
	if err != nil {
		if errors.Is(err, ErrBadWindow) {
			httpx.WriteError(w, r, httpx.ErrBadRequest(
				"Ask for a date range of at most three months, ending after it starts."))
			return
		}
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return
	}

	httpx.WriteJSON(w, r, http.StatusOK, map[string]any{
		"items": instanceResponses(doses, now),
	})
}

// addDoseRequest is the `POST /v1/medications/{id}/doses` body: one dose, at
// one instant, with no rule behind it.
type addDoseRequest struct {
	ScheduledFor string `json:"scheduledFor"`
}

func (h *Handler) addDose(w http.ResponseWriter, r *http.Request) {
	medication, ok := h.authorizedMedication(w, r, care.PermissionMedicationsManage)
	if !ok {
		return
	}

	var body addDoseRequest
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	scheduledFor, valid := parseInstant(body.ScheduledFor)
	if !valid {
		httpx.WriteError(w, r, httpx.ErrValidation(
			"Please check the highlighted fields.",
			map[string]string{"scheduledFor": "Use an ISO-8601 date and time."},
		))
		return
	}

	principal := auth.MustPrincipal(r.Context())

	dose, err := h.service.AddDose(r.Context(), medication, principal.UserID, scheduledFor)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, r, http.StatusCreated, ToInstanceResponse(dose, time.Now()))
}

// history returns one page of a medication's doses, newest first.
func (h *Handler) history(w http.ResponseWriter, r *http.Request) {
	medication, ok := h.authorizedMedication(w, r, care.PermissionMedicationsView)
	if !ok {
		return
	}

	now := time.Now()
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}

	page, err := h.service.History(
		r.Context(), medication, strings.TrimSpace(r.URL.Query().Get("cursor")), limit, now,
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
		"items":      instanceResponses(page.Items, now),
		"nextCursor": nextCursor,
	})
}

// actRequest carries an optional note with a dose.
type actRequest struct {
	Notes *string `json:"notes"`
}

// act builds the handler for taking or skipping a dose.
//
// Both need medications.record, which a circle can grant without granting
// medications.manage: a visiting caregiver may hand somebody their tablets
// without being able to change the prescription (plans/phase5.md §9).
func (h *Handler) act(action Action) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		medication, ok := h.authorizedMedication(w, r, care.PermissionMedicationsRecord)
		if !ok {
			return
		}

		instanceID, err := uuid.Parse(chi.URLParam(r, instanceIDParam))
		if err != nil {
			notFound(w, r)
			return
		}

		// The dose must belong to the medication the caller was authorized for.
		instance, err := h.service.GetInstance(r.Context(), instanceID)
		if err != nil || instance.MedicationID != medication.ID {
			notFound(w, r)
			return
		}

		principal := auth.MustPrincipal(r.Context())

		// A body is optional: the queued offline mutation sends none.
		var body actRequest
		if r.ContentLength > 0 {
			if err := httpx.DecodeJSON(w, r, &body); err != nil {
				httpx.WriteError(w, r, err)
				return
			}
		}

		var errs validation.Errors
		if body.Notes != nil {
			errs.MaxLength("notes", *body.Notes, maxNotesLength)
		}
		if errs.Any() {
			httpx.WriteError(w, r, httpx.ErrValidation("Please check the highlighted fields.", errs))
			return
		}

		result, err := h.service.Act(r.Context(), ActInput{
			InstanceID: instance.ID,
			Action:     action,
			// Never the client's word for who took it: an actor supplied in a
			// body could attribute somebody else's medicine to them
			// (plans/phase5.md §6).
			ActorID: principal.UserID,
			Notes:   body.Notes,
		})
		if err != nil {
			h.writeError(w, r, err)
			return
		}

		httpx.WriteJSON(w, r, http.StatusOK, ToInstanceResponse(result.Instance, time.Now()))
	}
}

// --- Authorization ---------------------------------------------------------

// authorizedMedication loads the medication named in the URL and authorizes the
// caller against the senior it belongs to.
//
// Every failure — a malformed ID, a medication that does not exist, no
// relationship to its senior, or a missing permission — produces the same 404.
// Anything else would let somebody discover what another family's relative
// takes, by watching the answers change (plans/phase5.md §25).
func (h *Handler) authorizedMedication(
	w http.ResponseWriter,
	r *http.Request,
	permissions ...care.Permission,
) (Medication, bool) {
	principal := auth.MustPrincipal(r.Context())

	medicationID, err := uuid.Parse(chi.URLParam(r, medicationIDParam))
	if err != nil {
		notFound(w, r)
		return Medication{}, false
	}

	medication, err := h.service.Get(r.Context(), medicationID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			notFound(w, r)
			return Medication{}, false
		}
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return Medication{}, false
	}

	if _, err := h.guard.Authorize(
		r.Context(), principal.UserID, medication.SeniorID, permissions...,
	); err != nil {
		if errors.Is(err, authz.ErrDenied) {
			notFound(w, r)
			return Medication{}, false
		}
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return Medication{}, false
	}

	return medication, true
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		notFound(w, r)

	case errors.Is(err, ErrDuplicateSchedule):
		httpx.WriteError(w, r, httpx.ErrValidation(
			"Please check the highlighted fields.",
			map[string]string{"scheduledTime": "This medication is already scheduled for that time."},
		))

	case errors.Is(err, ErrInvalidTransition):
		httpx.WriteError(w, r, httpx.ErrConflict(
			"Somebody has already recorded a different outcome for this dose."))

	default:
		httpx.WriteError(w, r, httpx.ErrInternal(err))
	}
}

// notFound matches the guard's wording, so an inaccessible medication and a
// non-existent one are indistinguishable.
func notFound(w http.ResponseWriter, r *http.Request) {
	httpx.WriteError(w, r, httpx.ErrNotFound(
		"That medication does not exist, or you do not have access."))
}

// --- Parsing ---------------------------------------------------------------

func parseSchedules(errs *validation.Errors, requested []scheduleRequest) []ScheduleInput {
	if len(requested) > maxSchedulesPerCreate {
		errs.Add("schedules", "That is more times than a medication can have.")
		return nil
	}

	parsed := make([]ScheduleInput, 0, len(requested))
	for index, entry := range requested {
		parsed = append(parsed, parseSchedule(errs, "schedules."+strconv.Itoa(index)+".", entry))
	}
	return parsed
}

// parseSchedule reads one time of day. The prefix names the field in a list, so
// the form can highlight the row that is wrong.
func parseSchedule(
	errs *validation.Errors,
	prefix string,
	requested scheduleRequest,
) ScheduleInput {
	var input ScheduleInput

	rule, err := recurrence.ParseRequest(
		requested.Recurrence.Frequency, requested.Recurrence.Weekdays,
	)
	if err != nil {
		errs.Add(prefix+"recurrence", recurrence.Message(err))
	} else {
		input.Recurrence = rule
	}

	at, err := recurrence.ParseTimeOfDay(requested.ScheduledTime)
	if err != nil {
		errs.Add(prefix+"scheduledTime", "Use a time like 08:00.")
	} else {
		input.ScheduledTime = at
	}

	return input
}

// parseForm reads an optional medication form, recording a validation failure
// rather than storing a value the database would refuse.
func parseForm(errs *validation.Errors, value *string) Form {
	if value == nil || strings.TrimSpace(*value) == "" {
		return ""
	}

	form := Form(strings.ToLower(strings.TrimSpace(*value)))
	if !form.Valid() {
		errs.Add("form", "Choose one of the listed forms.")
		return ""
	}
	return form
}

// trimmedOptional reads an optional free-text field, trimming and length-checking
// it only when the client actually sent one.
func trimmedOptional(
	errs *validation.Errors,
	field string,
	value *string,
	maxLength int,
) *string {
	if value == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*value)
	errs.MaxLength(field, trimmed, maxLength)
	return &trimmed
}

func parseInstant(value string) (time.Time, bool) {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	return parsed, err == nil
}

func derefOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
