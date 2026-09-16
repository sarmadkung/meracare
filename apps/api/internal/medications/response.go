package medications

import (
	"time"

	"github.com/google/uuid"
	"github.com/genxcare/api/internal/recurrence"
)

// MedicationResponse is the JSON representation of a medication.
type MedicationResponse struct {
	ID       string `json:"id"`
	SeniorID string `json:"seniorId"`

	Name string `json:"name"`
	// Dosage is as it was entered: "500 mg", "1 tablet". Never computed.
	Dosage string `json:"dosage"`
	// Form is null when nobody said what shape it comes in.
	Form         *string `json:"form"`
	Instructions *string `json:"instructions"`
	Notes        *string `json:"notes"`

	Active bool `json:"active"`

	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// ToMedicationResponse renders a medication.
func ToMedicationResponse(medication Medication) MedicationResponse {
	return MedicationResponse{
		ID:           medication.ID.String(),
		SeniorID:     medication.SeniorID.String(),
		Name:         medication.Name,
		Dosage:       medication.Dosage,
		Form:         optionalString(string(medication.Form)),
		Instructions: optionalString(medication.Instructions),
		Notes:        optionalString(medication.Notes),
		Active:       medication.Active,
		CreatedAt:    medication.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:    medication.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// MedicationDetailResponse adds the times a medication is taken and when the
// next dose falls, which the detail screen needs and the list does not
// (plans/phase5.md §15).
type MedicationDetailResponse struct {
	MedicationResponse
	Schedules []ScheduleResponse `json:"schedules"`
	// NextDoseAt is null when the medicine is stopped or has no live schedule.
	NextDoseAt *string `json:"nextDoseAt"`
}

// ToMedicationDetailResponse renders a medication with its schedules.
func ToMedicationDetailResponse(
	medication Medication,
	schedules []Schedule,
	nextDose *Instance,
) MedicationDetailResponse {
	items := make([]ScheduleResponse, 0, len(schedules))
	for _, schedule := range schedules {
		items = append(items, ToScheduleResponse(schedule))
	}

	var nextDoseAt *string
	if nextDose != nil {
		rendered := nextDose.ScheduledFor.UTC().Format(time.RFC3339)
		nextDoseAt = &rendered
	}

	return MedicationDetailResponse{
		MedicationResponse: ToMedicationResponse(medication),
		Schedules:          items,
		NextDoseAt:         nextDoseAt,
	}
}

// ScheduleResponse is the JSON representation of one time of day.
type ScheduleResponse struct {
	ID           string `json:"id"`
	MedicationID string `json:"medicationId"`

	Recurrence recurrence.Response `json:"recurrence"`
	// ScheduledTime is a wall-clock "HH:MM" in the senior's timezone, not an
	// instant.
	ScheduledTime string `json:"scheduledTime"`

	Active bool `json:"active"`

	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// ToScheduleResponse renders one schedule.
func ToScheduleResponse(schedule Schedule) ScheduleResponse {
	return ScheduleResponse{
		ID:            schedule.ID.String(),
		MedicationID:  schedule.MedicationID.String(),
		Recurrence:    recurrence.ToResponse(schedule.Recurrence),
		ScheduledTime: schedule.ScheduledTime.String(),
		Active:        schedule.Active,
		CreatedAt:     schedule.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     schedule.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// InstanceResponse is the JSON representation of one dose.
type InstanceResponse struct {
	ID string `json:"id"`
	// ScheduleID is null for a one-off dose.
	ScheduleID   *string `json:"scheduleId"`
	MedicationID string  `json:"medicationId"`
	SeniorID     string  `json:"seniorId"`

	// Name and dosage are as they read when the dose was scheduled, so a
	// history entry is not rewritten by a later change to the prescription.
	Name   string `json:"name"`
	Dosage string `json:"dosage"`

	ScheduledFor string `json:"scheduledFor"`

	// Status is the state as of now, so it reports "missed" for a pending dose
	// whose window has passed. The client renders this directly and never has
	// to work the missed case out for itself to be correct.
	Status string `json:"status"`
	// Recurring says whether this came from a schedule.
	Recurring bool `json:"recurring"`

	TakenAt   *string `json:"takenAt"`
	TakenBy   *string `json:"takenBy"`
	SkippedAt *string `json:"skippedAt"`
	SkippedBy *string `json:"skippedBy"`

	Notes *string `json:"notes"`

	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// ToInstanceResponse renders one dose as of now.
func ToInstanceResponse(instance Instance, now time.Time) InstanceResponse {
	return InstanceResponse{
		ID:           instance.ID.String(),
		ScheduleID:   optionalUUID(instance.ScheduleID),
		MedicationID: instance.MedicationID.String(),
		SeniorID:     instance.SeniorID.String(),
		Name:         instance.Name,
		Dosage:       instance.Dosage,
		ScheduledFor: instance.ScheduledFor.UTC().Format(time.RFC3339),
		Status:       string(instance.EffectiveStatus(now)),
		Recurring:    instance.Recurring(),
		TakenAt:      optionalTime(instance.TakenAt),
		TakenBy:      optionalUUID(instance.TakenBy),
		SkippedAt:    optionalTime(instance.SkippedAt),
		SkippedBy:    optionalUUID(instance.SkippedBy),
		Notes:        optionalString(instance.Notes),
		CreatedAt:    instance.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:    instance.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func instanceResponses(instances []Instance, now time.Time) []InstanceResponse {
	items := make([]InstanceResponse, 0, len(instances))
	for _, instance := range instances {
		items = append(items, ToInstanceResponse(instance, now))
	}
	return items
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
