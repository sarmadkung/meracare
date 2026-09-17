// Package medications implements medication management: what a person takes,
// the times they take it, and the record of each dose (docs/03-domain-model.md,
// Medication, MedicationSchedule and MedicationInstance).
//
// This is a care coordination domain, not a clinical one. GenxCare records what
// a family or caregiver entered and reminds them of it; it does not reason about
// whether a medicine or a dose is appropriate (plans/phase5.md §32).
//
// Keep the vocabulary in sync with packages/contracts/src/medication.ts.
package medications

import (
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/genxcare/api/internal/recurrence"
)

// Status is the state of one dose.
type Status string

const (
	// StatusPending is due, or coming up, and not yet acted on.
	StatusPending Status = "pending"
	// StatusTaken was taken, by somebody, at some time.
	StatusTaken Status = "taken"
	// StatusSkipped was deliberately not taken. It stays in the record: a skip
	// is a care decision, and often a medically interesting one
	// (plans/phase5.md §7).
	StatusSkipped Status = "skipped"

	// StatusMissed is derived from the clock and never stored.
	//
	// A dose is missed once its time and its grace period have passed with
	// nobody acting on it. Deriving it means the answer is right the first time
	// it is asked, with no sweep to fall behind and nothing to run overnight
	// (plans/phase5.md §8).
	StatusMissed Status = "missed"
)

// StoredStatuses are the values the database accepts. The CHECK constraint on
// medication_instances.status mirrors this list — StatusMissed is absent from
// both.
var StoredStatuses = []Status{StatusPending, StatusTaken, StatusSkipped}

// ValidStored reports whether the status is one that can be persisted.
func (s Status) ValidStored() bool { return slices.Contains(StoredStatuses, s) }

// Settled reports whether the dose has reached a final state.
func (s Status) Settled() bool { return s == StatusTaken || s == StatusSkipped }

// MissedAfter is how long a dose stays merely due before it counts as missed.
//
// A dose is not missed at one minute past eight. Somebody making breakfast is
// not late, and a screen that said so would train people to ignore it. Two
// hours is long enough to be a real window and short enough that a family
// checking in the same day still learns something (plans/phase5.md §8).
const MissedAfter = 2 * time.Hour

// Form is the physical shape a medicine takes.
//
// A short recognised list, so the create screen can offer choices rather than
// ask somebody to spell "capsule". The database CHECK on medications.form
// mirrors it.
type Form string

const (
	FormTablet    Form = "tablet"
	FormCapsule   Form = "capsule"
	FormLiquid    Form = "liquid"
	FormDrops     Form = "drops"
	FormInhaler   Form = "inhaler"
	FormInjection Form = "injection"
	FormPatch     Form = "patch"
	FormCream     Form = "cream"
	FormOther     Form = "other"
)

// Forms lists every form the MVP recognises.
var Forms = []Form{
	FormTablet, FormCapsule, FormLiquid, FormDrops, FormInhaler,
	FormInjection, FormPatch, FormCream, FormOther,
}

// Valid reports whether the form is recognised.
func (f Form) Valid() bool { return slices.Contains(Forms, f) }

// Medication is something a senior takes.
type Medication struct {
	ID              uuid.UUID
	SeniorID        uuid.UUID
	CreatedByUserID uuid.UUID

	Name string
	// Dosage is free text as entered: "500 mg", "1 tablet", "two puffs".
	Dosage string
	// Form is empty when nobody said.
	Form         Form
	Instructions string
	Notes        string

	// Active is false for a medicine that has been stopped. Its doses remain.
	Active bool

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Schedule is one time of day a medication is taken, and on which days.
//
// "Every day at 08:00 and 20:00" is two schedules, not one rule with two times.
// That costs a row and buys the ability to stop the evening dose without
// touching the morning one (plans/phase5.md §3).
type Schedule struct {
	ID           uuid.UUID
	MedicationID uuid.UUID
	SeniorID     uuid.UUID

	Recurrence recurrence.Rule
	// ScheduledTime is a wall-clock time in the senior's timezone.
	ScheduledTime recurrence.TimeOfDay

	Active bool

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Instance is one concrete dose.
type Instance struct {
	ID uuid.UUID
	// ScheduleID is nil for a one-off dose entered directly.
	ScheduleID      *uuid.UUID
	MedicationID    uuid.UUID
	SeniorID        uuid.UUID
	CreatedByUserID uuid.UUID

	// Name and Dosage are as they read when the dose was scheduled, so the
	// history is not rewritten by a later change to the prescription.
	Name   string
	Dosage string

	ScheduledFor time.Time
	Status       Status

	TakenAt *time.Time
	TakenBy *uuid.UUID

	SkippedAt *time.Time
	SkippedBy *uuid.UUID

	Notes string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Recurring reports whether the dose came from a schedule.
func (i Instance) Recurring() bool { return i.ScheduleID != nil }

// EffectiveStatus is the status as of now, including the derived missed state.
//
// Only a pending dose can be missed: once somebody has taken or skipped it, the
// clock stops mattering.
func (i Instance) EffectiveStatus(now time.Time) Status {
	if i.Status == StatusPending && now.After(i.ScheduledFor.Add(MissedAfter)) {
		return StatusMissed
	}
	return i.Status
}

// Missed reports whether the dose's window has passed with nobody acting on it.
func (i Instance) Missed(now time.Time) bool {
	return i.EffectiveStatus(now) == StatusMissed
}

// Due reports whether the dose is ready to be taken but not yet missed.
func (i Instance) Due(now time.Time) bool {
	return i.Status == StatusPending && !now.Before(i.ScheduledFor) && !i.Missed(now)
}

// ErrInvalidTransition is returned when the requested action contradicts what
// has already happened to the dose.
//
// It is deliberately distinct from "already in that state": recording the same
// dose as taken twice is a retry and succeeds, whereas skipping a dose somebody
// has already taken would overwrite a record of medicine that was actually
// swallowed (plans/phase5.md §22).
var ErrInvalidTransition = errors.New("this dose already has a different outcome")

// Action is something a user does to a dose.
type Action string

const (
	// ActionTake records the medicine as taken.
	ActionTake Action = "take"
	// ActionSkip records it as deliberately not taken.
	ActionSkip Action = "skip"
)

// resultOf is the status each action produces.
var resultOf = map[Action]Status{
	ActionTake: StatusTaken,
	ActionSkip: StatusSkipped,
}

// Transition decides what an action means for a dose in its current state.
//
// It returns the resulting status, and whether the action is a repeat of one
// that already happened. A repeat is not an error: the mobile client replays
// queued mutations when it comes back online, and a retry that failed loudly
// would leave somebody staring at an error for medicine they have taken
// (plans/phase5.md §21).
//
// A missed dose is still pending underneath, so it can be taken late — which is
// what people do, and what the record should say.
func Transition(current Status, action Action) (next Status, repeat bool, err error) {
	target, ok := resultOf[action]
	if !ok {
		return "", false, ErrInvalidTransition
	}

	switch current {
	case StatusPending:
		return target, false, nil

	// The same action again: nothing changes, and the caller is told so it can
	// leave the original actor and timestamp alone.
	case target:
		return current, true, nil

	// The other settled state: the dose already has a different outcome, and
	// the server is authoritative over which one stands.
	default:
		return "", false, ErrInvalidTransition
	}
}
