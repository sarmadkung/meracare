package summary_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/genxcare/api/internal/appointments"
	"github.com/genxcare/api/internal/auth"
	"github.com/genxcare/api/internal/care"
	"github.com/genxcare/api/internal/medications"
	"github.com/genxcare/api/internal/seniors"
	"github.com/genxcare/api/internal/summary"
	"github.com/genxcare/api/internal/tasks"
)

// Today is the screen somebody opens first. It has to show the care that is
// actually happening, across every circle they belong to — not only the work
// somebody remembered to assign them, which for a lone family carer is nothing.

var me = uuid.MustParse("99999999-9999-9999-9999-999999999999")

type stubAppointments struct {
	bySenior map[uuid.UUID][]appointments.Appointment
}

func (s stubAppointments) List(
	_ context.Context,
	seniorID uuid.UUID,
	_ appointments.Scope,
	_ time.Time,
) ([]appointments.Appointment, error) {
	return s.bySenior[seniorID], nil
}

func named(seniorID uuid.UUID, name string, permissions ...care.Permission) seniors.Membership {
	m := membership(seniorID, permissions...)
	m.Senior.DisplayName = name
	return m
}

func today(
	t *testing.T,
	s stubSeniors,
	tk stubTasks,
	d stubDoses,
	ap stubAppointments,
) []summary.Item {
	t.Helper()

	result, err := summary.NewTodayService(s, tk, d, ap).
		ForUser(context.Background(), auth.Principal{UserID: me}, now)
	if err != nil {
		t.Fatalf("ForUser: %v", err)
	}
	return result
}

func TestTodayGathersEveryDomainInTimeOrder(t *testing.T) {
	morning := now.Add(-3 * time.Hour)
	afternoon := now.Add(time.Hour)
	evening := now.Add(4 * time.Hour)

	items := today(t,
		stubSeniors{memberships: []seniors.Membership{named(
			seniorA, "James",
			care.PermissionTasksView, care.PermissionMedicationsView, care.PermissionAppointmentsView,
		)}},
		stubTasks{bySenior: map[uuid.UUID][]tasks.Instance{
			seniorA: {{ID: uuid.New(), Title: "Morning walk", Status: tasks.StatusPending, ScheduledFor: morning}},
		}},
		stubDoses{bySenior: map[uuid.UUID][]medications.Instance{
			seniorA: {{ID: uuid.New(), Name: "Metformin", Dosage: "500 mg", Status: medications.StatusPending, ScheduledFor: afternoon}},
		}},
		stubAppointments{bySenior: map[uuid.UUID][]appointments.Appointment{
			seniorA: {{ID: uuid.New(), Title: "Dr. Okafor", ScheduledAt: evening, Status: appointments.StatusScheduled}},
		}},
	)

	if len(items) != 3 {
		t.Fatalf("want one item per domain, got %d", len(items))
	}

	wantOrder := []string{"Morning walk", "Metformin", "Dr. Okafor"}
	for i, want := range wantOrder {
		if items[i].Title != want {
			t.Errorf("position %d: want %q, got %q", i, want, items[i].Title)
		}
	}

	// Every row has to say whose care it is: this list spans circles, and
	// "Metformin 2pm" means nothing without knowing who takes it.
	if items[1].SeniorName != "James" {
		t.Errorf("want the senior named on each row, got %q", items[1].SeniorName)
	}
}

/**/
func TestTodayMarksWhatIsAssignedToTheReader(t *testing.T) {
	someoneElse := uuid.New()

	items := today(t,
		stubSeniors{memberships: []seniors.Membership{named(seniorA, "James", care.PermissionTasksView)}},
		stubTasks{bySenior: map[uuid.UUID][]tasks.Instance{
			seniorA: {
				{ID: uuid.New(), Title: "Mine", Status: tasks.StatusPending, ScheduledFor: now, AssignedUserID: &me},
				{ID: uuid.New(), Title: "Theirs", Status: tasks.StatusPending, ScheduledFor: now.Add(time.Minute), AssignedUserID: &someoneElse},
				{ID: uuid.New(), Title: "Nobody's", Status: tasks.StatusPending, ScheduledFor: now.Add(2 * time.Minute)},
			},
		}},
		stubDoses{}, stubAppointments{},
	)

	byTitle := map[string]bool{}
	for _, item := range items {
		byTitle[item.Title] = item.AssignedToMe
	}

	if !byTitle["Mine"] {
		t.Error("want a task assigned to the reader marked as theirs")
	}
	if byTitle["Theirs"] || byTitle["Nobody's"] {
		t.Error("want only the reader's own assignment marked")
	}

	// Unassigned work still appears. A lone family carer assigns nothing to
	// themselves, and a Today that hid it would always be empty.
	if len(items) != 3 {
		t.Errorf("want every task today, got %d", len(items))
	}
}

func TestTodayOmitsDomainsTheReaderCannotSee(t *testing.T) {
	items := today(t,
		stubSeniors{memberships: []seniors.Membership{named(seniorA, "James", care.PermissionTasksView)}},
		stubTasks{bySenior: map[uuid.UUID][]tasks.Instance{
			seniorA: {{ID: uuid.New(), Title: "Morning walk", Status: tasks.StatusPending, ScheduledFor: now}},
		}},
		stubDoses{bySenior: map[uuid.UUID][]medications.Instance{
			seniorA: {{ID: uuid.New(), Name: "Metformin", Status: medications.StatusPending, ScheduledFor: now}},
		}},
		stubAppointments{bySenior: map[uuid.UUID][]appointments.Appointment{
			seniorA: {{ID: uuid.New(), Title: "Dr. Okafor", ScheduledAt: now}},
		}},
	)

	if len(items) != 1 || items[0].Title != "Morning walk" {
		t.Fatalf("want only the visible domain, got %+v", items)
	}
}

func TestTodaySpansEveryCircle(t *testing.T) {
	items := today(t,
		stubSeniors{memberships: []seniors.Membership{
			named(seniorA, "James", care.PermissionTasksView),
			named(seniorB, "Ada", care.PermissionTasksView),
		}},
		stubTasks{bySenior: map[uuid.UUID][]tasks.Instance{
			seniorA: {{ID: uuid.New(), Title: "Walk", Status: tasks.StatusPending, ScheduledFor: now.Add(time.Hour)}},
			seniorB: {{ID: uuid.New(), Title: "Lunch", Status: tasks.StatusPending, ScheduledFor: now}},
		}},
		stubDoses{}, stubAppointments{},
	)

	// Sorted by time across circles, not grouped by person: a caregiver works
	// through their day in order, not one client at a time.
	if len(items) != 2 || items[0].Title != "Lunch" || items[1].Title != "Walk" {
		t.Fatalf("want both circles interleaved by time, got %+v", items)
	}
}
