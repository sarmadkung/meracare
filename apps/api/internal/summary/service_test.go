package summary_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/meracare/api/internal/auth"
	"github.com/meracare/api/internal/care"
	"github.com/meracare/api/internal/medications"
	"github.com/meracare/api/internal/relationships"
	"github.com/meracare/api/internal/seniors"
	"github.com/meracare/api/internal/summary"
	"github.com/meracare/api/internal/tasks"
)

// The care circle list answers "who am I looking after". It cannot answer "does
// anyone need me right now", which is the question somebody actually opens the
// app with. These pin what the summary is allowed to say — and, more
// importantly, what it must stay silent about.

var (
	seniorA = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	seniorB = uuid.MustParse("22222222-2222-2222-2222-222222222222")
	now     = time.Date(2026, 9, 8, 15, 0, 0, 0, time.UTC)
)

type stubSeniors struct{ memberships []seniors.Membership }

func (s stubSeniors) List(context.Context, auth.Principal) ([]seniors.Membership, error) {
	return s.memberships, nil
}

type stubTasks struct {
	bySenior map[uuid.UUID][]tasks.Instance
}

func (s stubTasks) List(_ context.Context, in tasks.ListInput, _ time.Time) ([]tasks.Instance, error) {
	return s.bySenior[in.SeniorID], nil
}

type stubDoses struct {
	bySenior map[uuid.UUID][]medications.Instance
}

func (s stubDoses) ListDoses(
	_ context.Context,
	in medications.ListDosesInput,
	_ time.Time,
) ([]medications.Instance, error) {
	return s.bySenior[in.SeniorID], nil
}

func membership(seniorID uuid.UUID, permissions ...care.Permission) seniors.Membership {
	return seniors.Membership{
		Senior: seniors.Senior{ID: seniorID, Timezone: "UTC"},
		Relationship: relationships.Relationship{
			SeniorID:    seniorID,
			Status:      care.StatusActive,
			Permissions: care.PermissionSet(permissions),
		},
	}
}

func task(status tasks.Status, scheduledFor time.Time) tasks.Instance {
	return tasks.Instance{ID: uuid.New(), Status: status, ScheduledFor: scheduledFor}
}

func dose(status medications.Status, scheduledFor time.Time) medications.Instance {
	return medications.Instance{ID: uuid.New(), Status: status, ScheduledFor: scheduledFor}
}

func build(t *testing.T, s stubSeniors, tk stubTasks, d stubDoses) []summary.Summary {
	t.Helper()

	result, err := summary.NewService(s, tk, d).ForUser(context.Background(), auth.Principal{}, now)
	if err != nil {
		t.Fatalf("ForUser: %v", err)
	}
	return result
}

func TestCountsTodayForEachSenior(t *testing.T) {
	earlier := now.Add(-2 * time.Hour)

	result := build(t,
		stubSeniors{memberships: []seniors.Membership{
			membership(seniorA, care.PermissionTasksView, care.PermissionMedicationsView),
			membership(seniorB, care.PermissionTasksView, care.PermissionMedicationsView),
		}},
		stubTasks{bySenior: map[uuid.UUID][]tasks.Instance{
			seniorA: {
				task(tasks.StatusCompleted, earlier),
				task(tasks.StatusPending, now.Add(time.Hour)),
			},
		}},
		stubDoses{bySenior: map[uuid.UUID][]medications.Instance{
			seniorA: {
				dose(medications.StatusTaken, earlier),
				dose(medications.StatusTaken, earlier),
				dose(medications.StatusTaken, earlier),
				dose(medications.StatusPending, now.Add(time.Hour)),
			},
		}},
	)

	if len(result) != 2 {
		t.Fatalf("want a summary per senior, got %d", len(result))
	}

	first := result[0]
	if first.SeniorID != seniorA {
		t.Fatalf("want seniorA first, got %v", first.SeniorID)
	}
	if first.Medications == nil || first.Medications.Done != 3 || first.Medications.Total != 4 {
		t.Errorf("want 3 of 4 doses, got %+v", first.Medications)
	}
	if first.Tasks == nil || first.Tasks.Done != 1 || first.Tasks.Total != 2 {
		t.Errorf("want 1 of 2 tasks, got %+v", first.Tasks)
	}

	// A senior with nothing scheduled still gets an entry, with zeroes rather
	// than a missing row: an absent card would read as a loading failure.
	second := result[1]
	if second.Medications == nil || second.Medications.Total != 0 {
		t.Errorf("want an empty medication count, got %+v", second.Medications)
	}
}

func TestOmitsDomainsTheCallerCannotSee(t *testing.T) {
	result := build(t,
		stubSeniors{memberships: []seniors.Membership{membership(seniorA, care.PermissionTasksView)}},
		stubTasks{bySenior: map[uuid.UUID][]tasks.Instance{
			seniorA: {task(tasks.StatusPending, now.Add(time.Hour))},
		}},
		// Doses exist, but this caller has no medications.view. Reporting a
		// count would leak that the senior is medicated at all.
		stubDoses{bySenior: map[uuid.UUID][]medications.Instance{
			seniorA: {dose(medications.StatusPending, now.Add(time.Hour))},
		}},
	)

	if result[0].Medications != nil {
		t.Errorf("want no medication count without permission, got %+v", result[0].Medications)
	}
	if result[0].Tasks == nil {
		t.Error("want a task count with tasks.view")
	}
}

func TestNeedsAttentionCountsWhatIsLate(t *testing.T) {
	late := now.Add(-4 * time.Hour)

	result := build(t,
		stubSeniors{memberships: []seniors.Membership{
			membership(seniorA, care.PermissionTasksView, care.PermissionMedicationsView),
		}},
		stubTasks{bySenior: map[uuid.UUID][]tasks.Instance{
			seniorA: {
				task(tasks.StatusPending, late),               // overdue
				task(tasks.StatusPending, now.Add(time.Hour)), // not yet due
				task(tasks.StatusCompleted, late),             // done, not late
			},
		}},
		stubDoses{bySenior: map[uuid.UUID][]medications.Instance{
			seniorA: {
				dose(medications.StatusPending, late), // missed
				dose(medications.StatusTaken, late),   // taken, not missed
			},
		}},
	)

	if result[0].NeedsAttention != 2 {
		t.Errorf("want one overdue task and one missed dose, got %d", result[0].NeedsAttention)
	}
}

func TestNeedsAttentionIgnoresDomainsTheCallerCannotSee(t *testing.T) {
	late := now.Add(-4 * time.Hour)

	result := build(t,
		stubSeniors{memberships: []seniors.Membership{membership(seniorA, care.PermissionTasksView)}},
		stubTasks{bySenior: map[uuid.UUID][]tasks.Instance{
			seniorA: {task(tasks.StatusPending, late)},
		}},
		stubDoses{bySenior: map[uuid.UUID][]medications.Instance{
			seniorA: {dose(medications.StatusPending, late)},
		}},
	)

	// The missed dose is real, but this caller cannot see medication at all, so
	// counting it would tell them something the permission set withholds.
	if result[0].NeedsAttention != 1 {
		t.Errorf("want only the overdue task counted, got %d", result[0].NeedsAttention)
	}
}
