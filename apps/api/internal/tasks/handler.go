package tasks

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
	taskIDParam     = "taskID"
	templateIDParam = "templateID"

	maxTitleLength       = 140
	maxDescriptionLength = 2000
	maxNotesLength       = 2000
)

// Handler exposes the care task endpoints.
type Handler struct {
	service *Service
	guard   *authz.Guard
}

// NewHandler builds the handler.
func NewHandler(service *Service, guard *authz.Guard) *Handler {
	return &Handler{service: service, guard: guard}
}

// SeniorRoutes mounts `/v1/seniors/{seniorID}/tasks`, where the senior is named
// by the URL and the guard can run as middleware.
func (h *Handler) SeniorRoutes() chi.Router {
	router := chi.NewRouter()

	router.With(h.guard.RequirePermission(care.PermissionTasksView)).Get("/", h.list)
	router.With(h.guard.RequirePermission(care.PermissionTasksManage)).Post("/", h.create)

	router.Route("/templates", func(templates chi.Router) {
		templates.With(h.guard.RequirePermission(care.PermissionTasksView)).Get("/", h.listTemplates)
		templates.With(h.guard.RequirePermission(care.PermissionTasksManage)).
			Patch("/{"+templateIDParam+"}", h.updateTemplate)
	})

	return router
}

// TaskRoutes mounts `/v1/tasks`, where a task is named by its own ID.
//
// These cannot use the guard as middleware: the senior is not in the URL, and
// is only known once the task has been loaded. Each handler therefore resolves
// the task first and authorizes against the senior it belongs to — and answers
// exactly as the middleware would when that fails.
func (h *Handler) TaskRoutes() chi.Router {
	router := chi.NewRouter()

	router.Get("/", h.listAssigned)

	router.Route("/{"+taskIDParam+"}", func(task chi.Router) {
		task.Get("/", h.get)
		task.Patch("/", h.update)
		task.Delete("/", h.cancel)
		task.Post("/complete", h.act(ActionComplete))
		task.Post("/skip", h.act(ActionSkip))
	})

	return router
}

// list returns a senior's tasks for one of the named views.
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	relationship := authz.MustRelationship(r.Context())
	now := time.Now()

	input := ListInput{SeniorID: relationship.SeniorID, Scope: ScopeToday}

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

	instances, err := h.service.List(r.Context(), input, now)
	if err != nil {
		if errors.Is(err, ErrBadWindow) {
			httpx.WriteError(w, r, httpx.ErrBadRequest(
				"Ask for a date range of at most three months, ending after it starts."))
			return
		}
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return
	}

	writeInstances(w, r, instances, now)
}

// listAssigned returns the caller's own pending tasks across every circle.
//
// No guard: the query itself only returns tasks from circles where the caller
// still holds an active relationship carrying tasks.view.
func (h *Handler) listAssigned(w http.ResponseWriter, r *http.Request) {
	principal := auth.MustPrincipal(r.Context())
	now := time.Now()

	days := upcomingDays
	if raw := r.URL.Query().Get("days"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			days = parsed
		}
	}

	instances, err := h.service.ListAssigned(r.Context(), principal.UserID, now, days)
	if err != nil {
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return
	}

	writeInstances(w, r, instances, now)
}

// createRequest is the `POST /v1/seniors/{id}/tasks` body.
//
// Either scheduledFor (a one-time task) or recurrence with dueTime (a recurring
// one), never both.
type createRequest struct {
	Title          string  `json:"title"`
	Description    *string `json:"description"`
	AssignedUserID *string `json:"assignedUserId"`

	ScheduledFor *string `json:"scheduledFor"`

	Recurrence *recurrenceRequest `json:"recurrence"`
	DueTime    *string            `json:"dueTime"`
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

	title := strings.TrimSpace(body.Title)
	errs.Required("title", title)
	errs.MaxLength("title", title, maxTitleLength)

	description := strings.TrimSpace(derefOrEmpty(body.Description))
	errs.MaxLength("description", description, maxDescriptionLength)

	input := CreateInput{
		SeniorID:        relationship.SeniorID,
		CreatedByUserID: principal.UserID,
		Title:           title,
		Description:     description,
		AssignedUserID:  parseAssignee(&errs, body.AssignedUserID),
	}

	switch {
	case body.Recurrence != nil && body.ScheduledFor != nil:
		errs.Add("recurrence", "A task either happens once or repeats, not both.")

	case body.Recurrence != nil:
		rule, err := ParseRecurrenceRequest(body.Recurrence.Frequency, body.Recurrence.Weekdays)
		if err != nil {
			errs.Add("recurrence", recurrence.Message(err))
		} else {
			input.Recurrence = &rule
		}

		if body.DueTime == nil {
			errs.Add("dueTime", "Choose what time of day this happens.")
		} else if dueTime, err := ParseTimeOfDay(*body.DueTime); err != nil {
			errs.Add("dueTime", "Use a time like 09:00.")
		} else {
			input.DueTime = &dueTime
		}

	case body.ScheduledFor != nil:
		scheduledFor, ok := parseInstant(*body.ScheduledFor)
		if !ok {
			errs.Add("scheduledFor", "Use an ISO-8601 date and time.")
		} else {
			input.ScheduledFor = &scheduledFor
		}

	default:
		errs.Add("scheduledFor", "Choose when this task happens.")
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

	response := map[string]any{
		"tasks": instanceResponses(created.Instances, now),
	}
	if created.Template != nil {
		response["template"] = ToTemplateResponse(*created.Template)
	}

	httpx.WriteJSON(w, r, http.StatusCreated, response)
}

// get returns one task.
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	instance, ok := h.authorizedTask(w, r, care.PermissionTasksView)
	if !ok {
		return
	}
	httpx.WriteJSON(w, r, http.StatusOK, ToInstanceResponse(instance, time.Now()))
}

// updateRequest is the `PATCH /v1/tasks/{id}` body. An absent field is
// unchanged; an explicit null assignee unassigns.
type updateRequest struct {
	Title          *string `json:"title"`
	Description    *string `json:"description"`
	ScheduledFor   *string `json:"scheduledFor"`
	AssignedUserID *string `json:"assignedUserId"`
	// ClearAssignee is sent as `"assignedUserId": null` alongside this flag,
	// because JSON cannot otherwise distinguish "absent" from "null".
	ClearAssignee bool `json:"clearAssignee"`
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	instance, ok := h.authorizedTask(w, r, care.PermissionTasksManage)
	if !ok {
		return
	}

	var body updateRequest
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	var errs validation.Errors
	input := UpdateInstanceInput{
		InstanceID:     instance.ID,
		SeniorID:       instance.SeniorID,
		Description:    body.Description,
		ClearAssignee:  body.ClearAssignee,
		AssignedUserID: parseAssignee(&errs, body.AssignedUserID),
	}

	if body.Title != nil {
		title := strings.TrimSpace(*body.Title)
		errs.Required("title", title)
		errs.MaxLength("title", title, maxTitleLength)
		input.Title = &title
	}
	if body.Description != nil {
		errs.MaxLength("description", *body.Description, maxDescriptionLength)
	}
	if body.ScheduledFor != nil {
		scheduledFor, ok := parseInstant(*body.ScheduledFor)
		if !ok {
			errs.Add("scheduledFor", "Use an ISO-8601 date and time.")
		} else {
			input.ScheduledFor = &scheduledFor
		}
	}

	if errs.Any() {
		httpx.WriteError(w, r, httpx.ErrValidation("Please check the highlighted fields.", errs))
		return
	}

	updated, err := h.service.UpdateInstance(r.Context(), input)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, r, http.StatusOK, ToInstanceResponse(updated, time.Now()))
}

// actRequest carries an optional note with a completion or skip.
type actRequest struct {
	Notes *string `json:"notes"`
}

// act builds the handler for completing or skipping a task.
//
// Both need tasks.complete, and neither requires the caller to be the assignee:
// care given by whoever was present is still care given, and the record names
// who actually did it.
func (h *Handler) act(action Action) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instance, ok := h.authorizedTask(w, r, care.PermissionTasksComplete)
		if !ok {
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
			ActorID:    principal.UserID,
			Notes:      body.Notes,
		})
		if err != nil {
			h.writeError(w, r, err)
			return
		}

		httpx.WriteJSON(w, r, http.StatusOK, ToInstanceResponse(result.Instance, time.Now()))
	}
}

// cancel withdraws one occurrence.
//
// The row stays, marked cancelled. A task that was scheduled and then called
// off is part of the care record, and deleting it would erase the fact that it
// was ever planned (plans/phase4.md §19).
func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) {
	instance, ok := h.authorizedTask(w, r, care.PermissionTasksManage)
	if !ok {
		return
	}

	principal := auth.MustPrincipal(r.Context())

	result, err := h.service.Act(r.Context(), ActInput{
		InstanceID: instance.ID,
		Action:     ActionCancel,
		ActorID:    principal.UserID,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, r, http.StatusOK, ToInstanceResponse(result.Instance, time.Now()))
}

func (h *Handler) listTemplates(w http.ResponseWriter, r *http.Request) {
	relationship := authz.MustRelationship(r.Context())

	templates, err := h.service.ListTemplates(r.Context(), relationship.SeniorID)
	if err != nil {
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return
	}

	items := make([]TemplateResponse, 0, len(templates))
	for _, template := range templates {
		items = append(items, ToTemplateResponse(template))
	}

	httpx.WriteJSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// updateTemplateRequest is the `PATCH .../tasks/templates/{id}` body.
type updateTemplateRequest struct {
	Title          *string            `json:"title"`
	Description    *string            `json:"description"`
	Recurrence     *recurrenceRequest `json:"recurrence"`
	DueTime        *string            `json:"dueTime"`
	Active         *bool              `json:"active"`
	AssignedUserID *string            `json:"assignedUserId"`
	ClearAssignee  bool               `json:"clearAssignee"`
}

func (h *Handler) updateTemplate(w http.ResponseWriter, r *http.Request) {
	relationship := authz.MustRelationship(r.Context())
	now := time.Now()

	templateID, err := uuid.Parse(chi.URLParam(r, templateIDParam))
	if err != nil {
		notFound(w, r)
		return
	}

	// The template must belong to the senior the guard authorized, so a
	// template ID from another circle is unreachable even with permission here.
	template, err := h.service.GetTemplate(r.Context(), templateID)
	if err != nil || template.SeniorID != relationship.SeniorID {
		notFound(w, r)
		return
	}

	var body updateTemplateRequest
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	var errs validation.Errors
	input := UpdateTemplateInput{
		TemplateID:     template.ID,
		SeniorID:       template.SeniorID,
		Description:    body.Description,
		Active:         body.Active,
		ClearAssignee:  body.ClearAssignee,
		AssignedUserID: parseAssignee(&errs, body.AssignedUserID),
	}

	if body.Title != nil {
		title := strings.TrimSpace(*body.Title)
		errs.Required("title", title)
		errs.MaxLength("title", title, maxTitleLength)
		input.Title = &title
	}
	if body.Description != nil {
		errs.MaxLength("description", *body.Description, maxDescriptionLength)
	}
	if body.Recurrence != nil {
		rule, err := ParseRecurrenceRequest(body.Recurrence.Frequency, body.Recurrence.Weekdays)
		if err != nil {
			errs.Add("recurrence", recurrence.Message(err))
		} else {
			input.Recurrence = &rule
		}
	}
	if body.DueTime != nil {
		dueTime, err := ParseTimeOfDay(*body.DueTime)
		if err != nil {
			errs.Add("dueTime", "Use a time like 09:00.")
		} else {
			input.DueTime = &dueTime
		}
	}

	if errs.Any() {
		httpx.WriteError(w, r, httpx.ErrValidation("Please check the highlighted fields.", errs))
		return
	}

	updated, err := h.service.UpdateTemplate(r.Context(), input, now)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, r, http.StatusOK, ToTemplateResponse(updated))
}

// authorizedTask loads the task named in the URL and authorizes the caller
// against the senior it belongs to.
//
// Every failure — a malformed ID, a task that does not exist, no relationship
// to its senior, or a missing permission — produces the same 404. Anything else
// would let somebody discover which tasks exist in circles they cannot see, by
// watching the answers change (plans/phase4.md §16).
func (h *Handler) authorizedTask(
	w http.ResponseWriter,
	r *http.Request,
	permissions ...care.Permission,
) (Instance, bool) {
	principal := auth.MustPrincipal(r.Context())

	taskID, err := uuid.Parse(chi.URLParam(r, taskIDParam))
	if err != nil {
		notFound(w, r)
		return Instance{}, false
	}

	instance, err := h.service.Get(r.Context(), taskID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			notFound(w, r)
			return Instance{}, false
		}
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return Instance{}, false
	}

	if _, err := h.guard.Authorize(
		r.Context(), principal.UserID, instance.SeniorID, permissions...,
	); err != nil {
		if errors.Is(err, authz.ErrDenied) {
			notFound(w, r)
			return Instance{}, false
		}
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return Instance{}, false
	}

	return instance, true
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		notFound(w, r)

	case errors.Is(err, ErrInvalidAssignee):
		httpx.WriteError(w, r, httpx.ErrValidation(
			"Please check the highlighted fields.",
			map[string]string{"assignedUserId": "That person is not in this care circle."},
		))

	case errors.Is(err, ErrInvalidTransition):
		httpx.WriteError(w, r, httpx.ErrConflict(
			"Somebody has already recorded a different outcome for this task."))

	default:
		httpx.WriteError(w, r, httpx.ErrInternal(err))
	}
}

// notFound matches the guard's wording, so an inaccessible task and a
// non-existent one are indistinguishable.
func notFound(w http.ResponseWriter, r *http.Request) {
	httpx.WriteError(w, r, httpx.ErrNotFound("That task does not exist, or you do not have access."))
}

func writeInstances(w http.ResponseWriter, r *http.Request, instances []Instance, now time.Time) {
	httpx.WriteJSON(w, r, http.StatusOK, map[string]any{
		"items": instanceResponses(instances, now),
	})
}

func instanceResponses(instances []Instance, now time.Time) []InstanceResponse {
	items := make([]InstanceResponse, 0, len(instances))
	for _, instance := range instances {
		items = append(items, ToInstanceResponse(instance, now))
	}
	return items
}

func parseInstant(value string) (time.Time, bool) {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	return parsed, err == nil
}

// parseAssignee reads an optional assignee ID, recording a validation failure
// rather than an error when it is not a UUID.
func parseAssignee(errs *validation.Errors, value *string) *uuid.UUID {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}

	parsed, err := uuid.Parse(strings.TrimSpace(*value))
	if err != nil {
		errs.Add("assignedUserId", "That is not a person we recognise.")
		return nil
	}
	return &parsed
}

func derefOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
