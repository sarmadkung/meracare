// Package appointments implements the appointment and care-schedule domain:
// where somebody has to be, who is taking them, and whether they got there
// (docs/03-domain-model.md, Appointment).
//
// An appointment is deliberately not a care task. A task is a routine the
// circle carries out, repeats, and can generate occurrences from; an
// appointment is a single commitment at a place and a time that somebody
// booked. Modelling one as the other would mean either giving appointments a
// recurrence engine they have no use for, or asking a task to carry a provider
// and a location it has no meaning for (plans/phase6.md, objective).
//
// This is care coordination, not clinical software. GenxCare records that a
// visit is on Thursday at half past nine; it never reasons about whether the
// visit is needed (plans/phase6.md §28).
//
// Keep the vocabulary in sync with packages/contracts/src/appointment.ts.
package appointments

import (
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
)

// Status is the state of one appointment.
//
// Exactly the vocabulary in docs/03 and plans/phase6.md §3, and no more. There
// is no derived status here, unlike a task's `overdue` or a dose's `missed`: a
// dose whose hour has passed has genuinely been missed, but an appointment
// whose hour has passed has not become anything — nobody knows yet whether the
// person went. Inventing "past" or "missed" would be the app guessing at care
// it cannot observe. Clients separate upcoming from past by comparing the time
// with the clock, which is a question about the calendar rather than about the
// appointment's state.
type Status string

const (
	// StatusScheduled is booked and not yet settled, whether it is next week or
	// last week.
	StatusScheduled Status = "scheduled"
	// StatusCompleted means somebody recorded that it happened.
	StatusCompleted Status = "completed"
	// StatusCancelled means it was called off. The record is kept: knowing a
	// visit was cancelled is part of the care history (plans/phase6.md §9).
	StatusCancelled Status = "cancelled"
)

// Statuses lists every status the MVP recognises. The CHECK constraint on
// appointments.status mirrors this list.
var Statuses = []Status{StatusScheduled, StatusCompleted, StatusCancelled}

// Valid reports whether the status is recognised.
func (s Status) Valid() bool { return slices.Contains(Statuses, s) }

// Settled reports whether the appointment has reached a final state.
func (s Status) Settled() bool { return s == StatusCompleted || s == StatusCancelled }

// Kind is the sort of visit an appointment is.
//
// A short recognised list, matching plans/phase6.md §2, so the create screen
// can offer choices instead of asking somebody to type. Deliberately not a
// medical taxonomy: GenxCare coordinates appointments and does not classify
// care. The database CHECK on appointments.kind mirrors it.
type Kind string

const (
	KindDoctorVisit   Kind = "doctor_visit"
	KindHospitalVisit Kind = "hospital_visit"
	KindTherapy       Kind = "therapy"
	KindLaboratory    Kind = "laboratory"
	KindCareMeeting   Kind = "care_meeting"
	KindOther         Kind = "other"
)

// Kinds lists every kind the MVP recognises.
var Kinds = []Kind{
	KindDoctorVisit, KindHospitalVisit, KindTherapy,
	KindLaboratory, KindCareMeeting, KindOther,
}

// Valid reports whether the kind is recognised.
func (k Kind) Valid() bool { return slices.Contains(Kinds, k) }

// Appointment is one commitment in a senior's calendar.
type Appointment struct {
	ID              uuid.UUID
	SeniorID        uuid.UUID
	CreatedByUserID uuid.UUID

	Title string
	// Kind is empty when nobody said what sort of visit it is.
	Kind         Kind
	ProviderName string
	Location     string
	Notes        string

	// AssignedUserID is the circle member taking them, when one has been named.
	AssignedUserID *uuid.UUID

	// ScheduledAt is the absolute instant it starts. It is rendered in the
	// senior's timezone, never the reader's device zone (plans/phase6.md §4).
	ScheduledAt time.Time
	// EndsAt is nil when nobody said how long it would take. Because both ends
	// are instants, an appointment running past midnight needs no special case.
	EndsAt *time.Time

	Status Status

	CompletedAt *time.Time
	CompletedBy *uuid.UUID

	CancelledAt *time.Time
	CancelledBy *uuid.UUID

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Upcoming reports whether the appointment has not started yet. It is a
// question about the calendar, not about the appointment's state, which is why
// it takes the clock rather than reading a stored field.
func (a Appointment) Upcoming(now time.Time) bool {
	return a.ScheduledAt.After(now)
}

// Errors the domain raises. Both are sentinels so the handler can choose a
// status code without inspecting a message.
var (
	// ErrInvalidTransition is returned when the requested action contradicts
	// what has already happened to the appointment.
	//
	// Deliberately distinct from "already in that state": completing an
	// appointment twice is a retry and succeeds, whereas completing one that
	// somebody cancelled would overwrite a decision that was actually made
	// (plans/phase6.md §§24–25).
	ErrInvalidTransition = errors.New("this appointment already has a different outcome")

	// ErrSettled is returned when somebody tries to edit an appointment that has
	// already been completed or cancelled.
	//
	// Once settled, the row is history rather than a plan, and history is not
	// rewritten (plans/phase6.md §8).
	ErrSettled = errors.New("this appointment has already happened or been called off")
)

// Action is something a user does to an appointment.
type Action string

const (
	// ActionComplete records that the appointment happened.
	ActionComplete Action = "complete"
	// ActionCancel records that it was called off.
	ActionCancel Action = "cancel"
)

// resultOf is the status each action produces.
var resultOf = map[Action]Status{
	ActionComplete: StatusCompleted,
	ActionCancel:   StatusCancelled,
}

// Transition decides what an action means for an appointment in its current
// state.
//
// It returns the resulting status and whether the action repeats one that has
// already happened. A repeat is not an error: a request that timed out on a
// train may be retried, and telling somebody the cancellation failed for an
// appointment that is already cancelled would invite them to try something else
// (plans/phase6.md §24).
//
// The one transition the specification names as forbidden — cancelled becoming
// completed — falls out of the same rule that forbids its opposite: whichever
// outcome was recorded first stands, and the server is the authority on which
// that was (plans/phase6.md §25).
func Transition(current Status, action Action) (next Status, repeat bool, err error) {
	target, ok := resultOf[action]
	if !ok {
		return "", false, ErrInvalidTransition
	}

	switch current {
	case StatusScheduled:
		return target, false, nil

	// The same action again: nothing changes, and the caller is told so it can
	// leave the original actor and timestamp alone.
	case target:
		return current, true, nil

	// The other settled state.
	default:
		return "", false, ErrInvalidTransition
	}
}
