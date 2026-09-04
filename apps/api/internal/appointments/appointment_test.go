package appointments_test

import (
	"errors"
	"testing"
	"time"

	"github.com/genxcare/api/internal/appointments"
)

// The state machine is the whole of the conflict rule (plans/phase6.md §§3,
// 24–25), so it is worth testing on its own, without a database in the way.

func TestAScheduledAppointmentCanBeSettledEitherWay(t *testing.T) {
	cases := map[appointments.Action]appointments.Status{
		appointments.ActionComplete: appointments.StatusCompleted,
		appointments.ActionCancel:   appointments.StatusCancelled,
	}

	for action, want := range cases {
		next, repeat, err := appointments.Transition(appointments.StatusScheduled, action)
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		if next != want {
			t.Errorf("%s: status = %q, want %q", action, next, want)
		}
		if repeat {
			t.Errorf("%s: reported as a repeat of an action that had not happened", action)
		}
	}
}

// A retried request must succeed unchanged. The mobile app can send the same
// cancellation twice on a bad connection, and an error there would tell somebody
// their cancellation failed when it had not.
func TestRepeatingAnActionSucceedsAndChangesNothing(t *testing.T) {
	cases := map[appointments.Status]appointments.Action{
		appointments.StatusCompleted: appointments.ActionComplete,
		appointments.StatusCancelled: appointments.ActionCancel,
	}

	for current, action := range cases {
		next, repeat, err := appointments.Transition(current, action)
		if err != nil {
			t.Fatalf("%s again: %v", action, err)
		}
		if next != current {
			t.Errorf("%s again: status = %q, want it unchanged at %q", action, next, current)
		}
		if !repeat {
			t.Errorf("%s again: not reported as a repeat", action)
		}
	}
}

// The transition the specification names as forbidden, and its mirror image.
// Whichever outcome was recorded first stands.
func TestASettledAppointmentKeepsItsOutcome(t *testing.T) {
	cases := map[string]struct {
		current appointments.Status
		action  appointments.Action
	}{
		"completing a cancelled appointment": {
			current: appointments.StatusCancelled,
			action:  appointments.ActionComplete,
		},
		"cancelling a completed appointment": {
			current: appointments.StatusCompleted,
			action:  appointments.ActionCancel,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, err := appointments.Transition(tc.current, tc.action)
			if !errors.Is(err, appointments.ErrInvalidTransition) {
				t.Errorf("err = %v, want ErrInvalidTransition", err)
			}
		})
	}
}

func TestAnUnknownActionIsRefused(t *testing.T) {
	_, _, err := appointments.Transition(appointments.StatusScheduled, appointments.Action("reschedule"))
	if !errors.Is(err, appointments.ErrInvalidTransition) {
		t.Errorf("err = %v, want ErrInvalidTransition", err)
	}
}

// 'missed' and 'overdue' exist for tasks and doses and deliberately do not
// exist here: nobody knows whether a person went to a visit whose hour has
// passed. This pins that the vocabulary stays the documented three.
func TestTheStatusVocabularyIsExactlyTheDocumentedThree(t *testing.T) {
	if len(appointments.Statuses) != 3 {
		t.Fatalf("statuses = %v, want exactly scheduled, completed and cancelled",
			appointments.Statuses)
	}

	for _, invented := range []appointments.Status{"missed", "overdue", "past", "pending"} {
		if invented.Valid() {
			t.Errorf("%q is recognised, but the domain defines no such status", invented)
		}
	}
}

func TestOnlySettledStatusesCountAsSettled(t *testing.T) {
	if appointments.StatusScheduled.Settled() {
		t.Error("a scheduled appointment reports as settled")
	}
	if !appointments.StatusCompleted.Settled() || !appointments.StatusCancelled.Settled() {
		t.Error("a completed or cancelled appointment does not report as settled")
	}
}

// Upcoming is a reading of the clock, not a stored field: the same row is
// upcoming this morning and past this evening without anything being written.
func TestUpcomingIsDecidedByTheClock(t *testing.T) {
	now := time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC)
	appointment := appointments.Appointment{ScheduledAt: now.Add(time.Hour)}

	if !appointment.Upcoming(now) {
		t.Error("an appointment an hour away is not reported as upcoming")
	}
	if appointment.Upcoming(now.Add(2 * time.Hour)) {
		t.Error("an appointment an hour ago is still reported as upcoming")
	}
	if appointment.Upcoming(appointment.ScheduledAt) {
		t.Error("an appointment starting exactly now is reported as still to come")
	}
}

func TestOnlyRecognisedKindsAreValid(t *testing.T) {
	if !appointments.KindDoctorVisit.Valid() {
		t.Error("doctor_visit is not recognised")
	}
	for _, invented := range []appointments.Kind{"surgery", "chemotherapy", ""} {
		if invented.Valid() {
			t.Errorf("%q is recognised, but the domain defines no such kind", invented)
		}
	}
}
