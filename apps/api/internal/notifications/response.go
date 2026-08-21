package notifications

import "time"

// PreferencesResponse is the JSON form of one user's settings.
type PreferencesResponse struct {
	TaskReminders        bool   `json:"taskReminders"`
	MedicationReminders  bool   `json:"medicationReminders"`
	AppointmentReminders bool   `json:"appointmentReminders"`
	OverdueTaskAlerts    bool   `json:"overdueTaskAlerts"`
	CareActivity         bool   `json:"careActivity"`
	UpdatedAt            string `json:"updatedAt"`
}

// ToPreferencesResponse converts preferences for the wire.
//
// The user id is not included. The only preferences any request can reach are
// the caller's own, so returning the id would add a field whose only possible
// value the client already knows.
func ToPreferencesResponse(preferences Preferences) PreferencesResponse {
	updatedAt := ""
	if !preferences.UpdatedAt.IsZero() {
		updatedAt = preferences.UpdatedAt.UTC().Format(time.RFC3339)
	}

	return PreferencesResponse{
		TaskReminders:        preferences.TaskReminders,
		MedicationReminders:  preferences.MedicationReminders,
		AppointmentReminders: preferences.AppointmentReminders,
		OverdueTaskAlerts:    preferences.OverdueTaskAlerts,
		CareActivity:         preferences.CareActivity,
		UpdatedAt:            updatedAt,
	}
}

// DeviceResponse is the JSON form of a registered installation.
//
// There is no push token field, and adding one would be a security regression
// rather than a feature: the token is a credential for making somebody's phone
// buzz, the client that registered it already has it, and no other client has
// any business reading it (plans/phase8.md §8).
type DeviceResponse struct {
	ID         string `json:"id"`
	DeviceID   string `json:"deviceId"`
	Platform   string `json:"platform"`
	AppVersion string `json:"appVersion"`
	Active     bool   `json:"active"`
	LastSeenAt string `json:"lastSeenAt"`
	// PushTokenRegistered says whether we hold a token for this device,
	// without saying what it is — enough for the settings screen to explain
	// that notifications are set up.
	PushTokenRegistered bool `json:"pushTokenRegistered"`
}

// ToDeviceResponse converts a device for the wire.
func ToDeviceResponse(device Device) DeviceResponse {
	return DeviceResponse{
		ID:                  device.ID.String(),
		DeviceID:            device.DeviceID,
		Platform:            string(device.Platform),
		AppVersion:          device.AppVersion,
		Active:              device.Active,
		LastSeenAt:          device.LastSeenAt.UTC().Format(time.RFC3339),
		PushTokenRegistered: device.PushToken != "",
	}
}

// ReminderResponse is one notification the device should schedule.
//
// There is no title and no body. The device composes both from this and the
// shared wording in packages/contracts, which is what keeps a medicine's name
// out of anything that can appear on a lock screen (plans/phase8.md §§17, 47).
type ReminderResponse struct {
	ID   string `json:"id"`
	Type string `json:"type"`

	SeniorID       string `json:"seniorId"`
	SeniorName     string `json:"seniorName"`
	SeniorTimezone string `json:"seniorTimezone"`

	EntityType string `json:"entityType"`
	EntityID   string `json:"entityId"`

	DueAt  string `json:"dueAt"`
	FireAt string `json:"fireAt"`
}

// ToReminderResponse converts one reminder for the wire.
func ToReminderResponse(reminder Reminder) ReminderResponse {
	return ReminderResponse{
		ID:             reminder.ID.String(),
		Type:           string(reminder.Type),
		SeniorID:       reminder.SeniorID.String(),
		SeniorName:     reminder.SeniorName,
		SeniorTimezone: reminder.SeniorTimezone,
		EntityType:     string(reminder.EntityType),
		EntityID:       reminder.EntityID.String(),
		DueAt:          reminder.DueAt.UTC().Format(time.RFC3339),
		FireAt:         reminder.FireAt.UTC().Format(time.RFC3339),
	}
}

// PlanResponse is a complete reminder plan.
type PlanResponse struct {
	Reminders []ReminderResponse `json:"reminders"`
	// GeneratedAt and HorizonEndsAt let the device reason about what this plan
	// covers: everything scheduled beyond HorizonEndsAt is outside the plan and
	// must not be cancelled just because it is absent from it.
	GeneratedAt   string `json:"generatedAt"`
	HorizonEndsAt string `json:"horizonEndsAt"`
}

// ToPlanResponse converts a plan for the wire.
func ToPlanResponse(reminders []Reminder, now time.Time) PlanResponse {
	responses := make([]ReminderResponse, 0, len(reminders))
	for _, reminder := range reminders {
		responses = append(responses, ToReminderResponse(reminder))
	}

	return PlanResponse{
		Reminders:     responses,
		GeneratedAt:   now.UTC().Format(time.RFC3339),
		HorizonEndsAt: now.Add(horizon).UTC().Format(time.RFC3339),
	}
}

// NotificationResponse is one row of the inbox.
//
// It carries the words as they were sent, and identifiers for where to go. It
// carries no delivery state: whether a push reached a phone is an operational
// fact about MeraCare's infrastructure, not something the person reading their
// inbox has any use for, and exposing it would invite a client to render
// "failed" against a notification the user is looking at right now
// (plans/phase11.md §§6, 27).
type NotificationResponse struct {
	ID   string `json:"id"`
	Type string `json:"type"`

	Title string `json:"title"`
	Body  string `json:"body"`

	SeniorID   string `json:"seniorId"`
	EntityType string `json:"entityType"`
	EntityID   string `json:"entityId"`

	// OccurredAt is what the notification is for, which is also what the inbox
	// is sorted by. Named for the reader rather than for the scheduler.
	OccurredAt string `json:"occurredAt"`
	Read       bool   `json:"read"`
	// ReadAt is empty while unread.
	ReadAt string `json:"readAt"`
}

// ToNotificationResponse converts one notification for the wire.
func ToNotificationResponse(notification Notification) NotificationResponse {
	readAt := ""
	if notification.ReadAt != nil {
		readAt = notification.ReadAt.UTC().Format(time.RFC3339)
	}

	return NotificationResponse{
		ID:         notification.ID.String(),
		Type:       string(notification.Type),
		Title:      notification.Title,
		Body:       notification.Body,
		SeniorID:   notification.SeniorID.String(),
		EntityType: string(notification.EntityType),
		EntityID:   notification.EntityID.String(),
		OccurredAt: notification.ScheduledFor.UTC().Format(time.RFC3339),
		Read:       notification.Read(),
		ReadAt:     readAt,
	}
}

// InboxResponse is one page of notifications.
//
// The unread count travels with the page rather than living on its own
// endpoint, so the badge and the list a client renders always come from the
// same read and cannot disagree (plans/phase11.md §61).
type InboxResponse struct {
	Items       []NotificationResponse `json:"items"`
	NextCursor  *string                `json:"nextCursor"`
	UnreadCount int                    `json:"unreadCount"`
}

// ToInboxResponse converts a page for the wire.
func ToInboxResponse(page Page) InboxResponse {
	items := make([]NotificationResponse, 0, len(page.Items))
	for _, notification := range page.Items {
		items = append(items, ToNotificationResponse(notification))
	}

	var next *string
	if page.NextCursor != "" {
		cursor := page.NextCursor
		next = &cursor
	}

	return InboxResponse{Items: items, NextCursor: next, UnreadCount: page.Unread}
}

// ReadAllResponse reports what marking everything read changed.
type ReadAllResponse struct {
	MarkedRead  int `json:"markedRead"`
	UnreadCount int `json:"unreadCount"`
}
