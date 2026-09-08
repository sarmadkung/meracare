// Package careevents implements the care event record and the activity
// timeline: one chronological account of what has happened in a senior's care,
// across every domain (docs/03-domain-model.md, CareEvent;
// docs/04-care-events-and-workflows.md).
//
// One timeline, not four. A family member asking "what happened yesterday?" is
// not asking about tasks, or about medication, or about appointments — they are
// asking about their relative. Per-domain activity feeds would answer a
// question nobody has and would have to be joined in the client to answer the
// one everybody does (plans/phase7.md, objective).
//
// Events are written by trusted domain operations only. There is deliberately
// no endpoint that creates one: an activity timeline somebody can write to
// directly is a record of what people claimed rather than of what happened
// (plans/phase7.md §21).
//
// Keep the vocabulary in sync with packages/contracts/src/care-event.ts.
package careevents

import (
	"slices"
	"time"

	"github.com/google/uuid"
)

// Type is what happened.
//
// The vocabulary is the documentation's, not ours: docs/03-domain-model.md
// names the events and plans/phase7.md §2 adds the remaining domain actions.
// The CHECK constraint on care_events.event_type mirrors this list exactly, so
// a name that is not here cannot reach the table (plans/phase7.md §25).
type Type string

const (
	// Care circle.
	TypeMemberInvited Type = "MEMBER_INVITED"
	TypeMemberJoined  Type = "MEMBER_JOINED"
	TypeMemberRevoked Type = "MEMBER_REVOKED"

	// Tasks.
	TypeTaskCreated   Type = "TASK_CREATED"
	TypeTaskCompleted Type = "TASK_COMPLETED"
	TypeTaskSkipped   Type = "TASK_SKIPPED"
	// TypeTaskMissed is documented but not yet emitted — see NotYetEmitted.
	TypeTaskMissed Type = "TASK_MISSED"

	// Medication.
	TypeMedicationCreated Type = "MEDICATION_CREATED"
	TypeMedicationTaken   Type = "MEDICATION_TAKEN"
	TypeMedicationSkipped Type = "MEDICATION_SKIPPED"
	// TypeMedicationMissed is documented but not emitted as a care event.
	TypeMedicationMissed Type = "MEDICATION_MISSED"

	// Appointments.
	TypeAppointmentCreated   Type = "APPOINTMENT_CREATED"
	TypeAppointmentCompleted Type = "APPOINTMENT_COMPLETED"
	TypeAppointmentCancelled Type = "APPOINTMENT_CANCELLED"

	// Notes.
	TypeNoteAdded Type = "NOTE_ADDED"
)

// Types lists every event the documentation names.
var Types = []Type{
	TypeMemberInvited, TypeMemberJoined, TypeMemberRevoked,
	TypeTaskCreated, TypeTaskCompleted, TypeTaskSkipped, TypeTaskMissed,
	TypeMedicationCreated, TypeMedicationTaken, TypeMedicationSkipped, TypeMedicationMissed,
	TypeAppointmentCreated, TypeAppointmentCompleted, TypeAppointmentCancelled,
	TypeNoteAdded,
}

// Valid reports whether the type is one the documentation names.
func (t Type) Valid() bool { return slices.Contains(Types, t) }

// NotYetEmitted are documented event types that no code path produces, and the
// reason each does not.
//
// They are in the vocabulary because the vocabulary is the documentation's; they
// are unemitted because nothing has happened that anybody could record:
//
//   - TASK_MISSED and MEDICATION_MISSED are derived from the clock rather than
//     performed by a person. Nothing writes "missed" anywhere in the system —
//     it is computed at read time precisely so that no background sweep has to
//     be alive for the care data to be true. MEDICATION_MISSED can be emitted
//     independently as a notification decision after the medication domain
//     derives it, but it is still never fabricated as a timeline event.
//
// A test pins that none of these is produced, so "not yet" cannot quietly
// become "never noticed".
var NotYetEmitted = []Type{TypeTaskMissed, TypeMedicationMissed}

// EntityType names the kind of thing an event is about, so a client can route
// to the right screen without parsing the event type.
type EntityType string

const (
	EntityTask         EntityType = "task"
	EntityMedication   EntityType = "medication"
	EntityAppointment  EntityType = "appointment"
	EntityRelationship EntityType = "relationship"
	EntityInvitation   EntityType = "invitation"
	EntityNote         EntityType = "note"
)

// EntityTypes lists every entity kind an event can point at. The CHECK
// constraint on care_events.entity_type mirrors it.
var EntityTypes = []EntityType{
	EntityTask, EntityMedication, EntityAppointment,
	EntityRelationship, EntityInvitation, EntityNote,
}

// Valid reports whether the entity type is recognised.
func (e EntityType) Valid() bool { return slices.Contains(EntityTypes, e) }

// Event is one thing that happened in a senior's care.
//
// It is a historical record. Nothing edits or deletes one: if the task it
// describes is later renamed, or the appointment cancelled, the event still
// says what was true when it was written (plans/phase7.md §5). That is why the
// metadata carries a copy of the name rather than a reference to it.
type Event struct {
	ID       uuid.UUID
	SeniorID uuid.UUID

	// ActorUserID is the authenticated user who performed the action, and is
	// nil only for an event no person performed. It is never read from a
	// request body (plans/phase7.md §4).
	ActorUserID *uuid.UUID

	Type Type

	EntityType EntityType
	EntityID   uuid.UUID

	// Metadata is the small amount of structured detail the timeline needs to
	// render a sentence — a task's title, a medicine's name and dosage. It is
	// deliberately not a copy of the row (plans/phase7.md §9).
	Metadata Metadata

	OccurredAt time.Time
	CreatedAt  time.Time
}

// Metadata is the structured detail carried with an event.
//
// A map of strings rather than a free-form document: everything the timeline
// renders is a short label, and a shape that cannot nest is a shape that cannot
// quietly grow into a copy of the record. Nothing sensitive belongs here — no
// token, no credential, and no care detail the sentence does not need
// (plans/phase7.md §§9, 10).
type Metadata map[string]string

// Common metadata keys. Named so the domains and the mobile renderer cannot
// disagree about spelling.
const (
	MetaTaskTitle       = "taskTitle"
	MetaMedicationName  = "medicationName"
	MetaDosage          = "dosage"
	MetaAppointmentName = "appointmentTitle"
	MetaMemberName      = "memberName"
	MetaRole            = "role"
)
