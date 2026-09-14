package tasks

import (
	"time"

	"github.com/google/uuid"
	"github.com/genxcare/api/internal/recurrence"
)

// InstanceResponse is the JSON representation of one task occurrence.
type InstanceResponse struct {
	ID string `json:"id"`
	// TemplateID is null for a one-time task.
	TemplateID *string `json:"templateId"`
	SeniorID   string  `json:"seniorId"`

	Title       string  `json:"title"`
	Description *string `json:"description"`

	AssignedUserID *string `json:"assignedUserId"`

	ScheduledFor string `json:"scheduledFor"`

	// Status is the state as of now, so it reports "overdue" for a pending task
	// whose time has passed. The client renders this directly and never has to
	// work the overdue case out for itself to be correct.
	Status string `json:"status"`
	// Recurring says whether this came from a repeating rule, which decides
	// whether the detail screen offers to edit the series.
	Recurring bool `json:"recurring"`

	CompletedAt *string `json:"completedAt"`
	CompletedBy *string `json:"completedBy"`
	SkippedAt   *string `json:"skippedAt"`
	SkippedBy   *string `json:"skippedBy"`

	Notes *string `json:"notes"`

	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// ToInstanceResponse renders one occurrence as of now.
func ToInstanceResponse(instance Instance, now time.Time) InstanceResponse {
	return InstanceResponse{
		ID:             instance.ID.String(),
		TemplateID:     optionalUUID(instance.TemplateID),
		SeniorID:       instance.SeniorID.String(),
		Title:          instance.Title,
		Description:    optionalString(instance.Description),
		AssignedUserID: optionalUUID(instance.AssignedUserID),
		ScheduledFor:   instance.ScheduledFor.UTC().Format(time.RFC3339),
		Status:         string(instance.EffectiveStatus(now)),
		Recurring:      instance.Recurring(),
		CompletedAt:    optionalTime(instance.CompletedAt),
		CompletedBy:    optionalUUID(instance.CompletedBy),
		SkippedAt:      optionalTime(instance.SkippedAt),
		SkippedBy:      optionalUUID(instance.SkippedBy),
		Notes:          optionalString(instance.Notes),
		CreatedAt:      instance.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:      instance.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// RecurrenceResponse describes a repeat rule in structured form.
//
// The stored RRULE string never leaves the server. The client turns this into
// wording like "Every weekday", which is what a user should read
// (plans/phase4.md §21).
type RecurrenceResponse = recurrence.Response

var (
	// ToRecurrenceResponse renders a rule for the client.
	ToRecurrenceResponse = recurrence.ToResponse
	// ParseRecurrenceRequest reads the structured form a client sends.
	ParseRecurrenceRequest = recurrence.ParseRequest
)

// TemplateResponse is the JSON representation of a recurring task definition.
type TemplateResponse struct {
	ID       string `json:"id"`
	SeniorID string `json:"seniorId"`

	Title       string  `json:"title"`
	Description *string `json:"description"`

	AssignedUserID *string `json:"assignedUserId"`

	Recurrence RecurrenceResponse `json:"recurrence"`
	// DueTime is a wall-clock "HH:MM" in the senior's timezone, not an instant.
	DueTime string `json:"dueTime"`

	Active bool `json:"active"`

	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// ToTemplateResponse renders a recurring task definition.
func ToTemplateResponse(template Template) TemplateResponse {
	return TemplateResponse{
		ID:             template.ID.String(),
		SeniorID:       template.SeniorID.String(),
		Title:          template.Title,
		Description:    optionalString(template.Description),
		AssignedUserID: optionalUUID(template.AssignedUserID),
		Recurrence:     ToRecurrenceResponse(template.Recurrence),
		DueTime:        template.DueTime.String(),
		Active:         template.Active,
		CreatedAt:      template.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:      template.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func optionalUUID(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	rendered := value.String()
	return &rendered
}

func optionalTime(value *time.Time) *string {
	if value == nil {
		return nil
	}
	rendered := value.UTC().Format(time.RFC3339)
	return &rendered
}
