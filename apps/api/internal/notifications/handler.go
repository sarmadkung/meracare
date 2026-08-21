package notifications

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/meracare/api/internal/auth"
	"github.com/meracare/api/pkg/httpx"
)

// Path segments this handler reads.
const (
	deviceIDParam       = "deviceId"
	notificationIDParam = "notificationId"
)

// Handler exposes the `/v1/notifications` endpoints.
//
// Every route here is about the caller themselves — their settings, their
// devices, their reminders — so none of them is senior-scoped and none takes
// the authorization guard. The senior-level checks happen inside the plan,
// where the reminders actually are (plans/phase8.md §5).
type Handler struct {
	service *Service
}

// NewHandler builds the handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Routes mounts the notification endpoints. The caller applies the
// authentication middleware.
func (h *Handler) Routes() chi.Router {
	router := chi.NewRouter()

	router.Get("/preferences", h.getPreferences)
	router.Patch("/preferences", h.updatePreferences)

	router.Post("/devices", h.registerDevice)
	router.Delete("/devices/{"+deviceIDParam+"}", h.deactivateDevice)

	router.Get("/reminders", h.reminders)

	// The inbox. Every route is scoped to the caller in the query itself; none
	// of them takes a user id from anywhere but the verified token
	// (plans/phase11.md §8).
	router.Get("/", h.list)
	router.Post("/read-all", h.readAll)
	router.Patch("/{"+notificationIDParam+"}/read", h.markRead)

	// There is deliberately no endpoint that schedules, sends, or cancels a
	// notification. Reminders follow from care, so the way to change them is to
	// change the care or the preferences (plans/phase8.md §33).
	return router
}

// getPreferences returns the caller's notification settings.
func (h *Handler) getPreferences(w http.ResponseWriter, r *http.Request) {
	principal := auth.MustPrincipal(r.Context())

	preferences, err := h.service.Preferences(r.Context(), principal.UserID)
	if err != nil {
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return
	}

	httpx.WriteJSON(w, r, http.StatusOK, ToPreferencesResponse(preferences))
}

// updatePreferencesRequest is the `PATCH /v1/notifications/preferences` body.
// Absent fields are left unchanged.
//
// There is no user field. The caller is the subject, always
// (plans/phase8.md §40).
type updatePreferencesRequest struct {
	TaskReminders        *bool `json:"taskReminders"`
	MedicationReminders  *bool `json:"medicationReminders"`
	AppointmentReminders *bool `json:"appointmentReminders"`
	OverdueTaskAlerts    *bool `json:"overdueTaskAlerts"`
	CareActivity         *bool `json:"careActivity"`
}

// updatePreferences changes the caller's notification settings.
func (h *Handler) updatePreferences(w http.ResponseWriter, r *http.Request) {
	principal := auth.MustPrincipal(r.Context())

	var body updatePreferencesRequest
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	preferences, err := h.service.UpdatePreferences(r.Context(), principal.UserID, PreferenceUpdate{
		TaskReminders:        body.TaskReminders,
		MedicationReminders:  body.MedicationReminders,
		AppointmentReminders: body.AppointmentReminders,
		OverdueTaskAlerts:    body.OverdueTaskAlerts,
		CareActivity:         body.CareActivity,
	})
	if err != nil {
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return
	}

	httpx.WriteJSON(w, r, http.StatusOK, ToPreferencesResponse(preferences))
}

// registerDeviceRequest is the `POST /v1/notifications/devices` body.
type registerDeviceRequest struct {
	DeviceID   string `json:"deviceId"`
	Platform   string `json:"platform"`
	PushToken  string `json:"pushToken"`
	AppVersion string `json:"appVersion"`
}

// registerDevice records or refreshes the caller's installation.
//
// Always 200, never 201: an install that registers on every launch is doing the
// same thing every time, and a status that alternated between created and
// updated would invite the client to care about a distinction it has no use for
// (plans/phase8.md §25).
func (h *Handler) registerDevice(w http.ResponseWriter, r *http.Request) {
	principal := auth.MustPrincipal(r.Context())

	var body registerDeviceRequest
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	device, err := h.service.RegisterDevice(r.Context(), RegisterParams{
		UserID:     principal.UserID,
		DeviceID:   body.DeviceID,
		Platform:   Platform(body.Platform),
		PushToken:  body.PushToken,
		AppVersion: body.AppVersion,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, r, http.StatusOK, ToDeviceResponse(device))
}

// deactivateDevice stops MeraCare reaching one of the caller's installations.
func (h *Handler) deactivateDevice(w http.ResponseWriter, r *http.Request) {
	principal := auth.MustPrincipal(r.Context())

	err := h.service.DeactivateDevice(r.Context(), principal.UserID, chi.URLParam(r, deviceIDParam))
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// reminders returns the plan the caller's device should schedule.
func (h *Handler) reminders(w http.ResponseWriter, r *http.Request) {
	principal := auth.MustPrincipal(r.Context())

	now := time.Now()
	plan, err := h.service.Plan(r.Context(), principal.UserID, now)
	if err != nil {
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return
	}

	httpx.WriteJSON(w, r, http.StatusOK, ToPlanResponse(plan, now))
}

// list returns one page of the caller's inbox, newest first.
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	principal := auth.MustPrincipal(r.Context())

	// Same shape as the activity timeline's paging, deliberately: one cursor
	// implementation and one way of reading it (plans/phase11.md §41).
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}

	page, err := h.service.Inbox(
		r.Context(),
		principal.UserID,
		strings.TrimSpace(r.URL.Query().Get("cursor")),
		limit,
		time.Now(),
	)
	if errors.Is(err, ErrBadCursor) {
		httpx.WriteError(w, r, httpx.ErrBadRequest("Start again from the first page."))
		return
	}
	if err != nil {
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return
	}

	httpx.WriteJSON(w, r, http.StatusOK, ToInboxResponse(page))
}

// markRead marks one of the caller's notifications as read.
func (h *Handler) markRead(w http.ResponseWriter, r *http.Request) {
	principal := auth.MustPrincipal(r.Context())

	notificationID, err := uuid.Parse(chi.URLParam(r, notificationIDParam))
	if err != nil {
		// An unparseable id and somebody else's id get the same answer, so the
		// endpoint cannot be used to find out which ids are real.
		httpx.WriteError(w, r, httpx.ErrNotFound("The requested resource does not exist."))
		return
	}

	notification, err := h.service.MarkRead(r.Context(), principal.UserID, notificationID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, r, http.StatusOK, ToNotificationResponse(notification))
}

// readAll marks every arrived notification of the caller's as read.
func (h *Handler) readAll(w http.ResponseWriter, r *http.Request) {
	principal := auth.MustPrincipal(r.Context())
	now := time.Now()

	marked, err := h.service.MarkAllRead(r.Context(), principal.UserID, now)
	if err != nil {
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return
	}

	unread, err := h.service.UnreadCount(r.Context(), principal.UserID, now)
	if err != nil {
		httpx.WriteError(w, r, httpx.ErrInternal(err))
		return
	}

	httpx.WriteJSON(w, r, http.StatusOK, ReadAllResponse{MarkedRead: marked, UnreadCount: unread})
}

// writeError maps this package's errors onto responses.
func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var invalid ErrInvalidDevice
	if errors.As(err, &invalid) {
		httpx.WriteError(w, r, httpx.ErrValidation(
			"That device registration is not valid.",
			map[string]string{invalid.Field: invalid.Message},
		))
		return
	}

	// A device belonging to somebody else and a device that does not exist get
	// the same answer, so registrations cannot be probed for
	// (plans/phase8.md §40).
	if errors.Is(err, ErrUnknownDevice) || errors.Is(err, ErrUnknownNotification) {
		httpx.WriteError(w, r, httpx.ErrNotFound("The requested resource does not exist."))
		return
	}

	httpx.WriteError(w, r, httpx.ErrInternal(err))
}
