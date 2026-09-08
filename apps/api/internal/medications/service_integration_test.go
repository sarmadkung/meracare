package medications_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/genxcare/api/internal/auth"
	"github.com/genxcare/api/internal/careevents"
	"github.com/genxcare/api/internal/medications"
	"github.com/genxcare/api/internal/recurrence"
	"github.com/genxcare/api/internal/relationships"
	"github.com/genxcare/api/internal/seniors"
	"github.com/genxcare/api/internal/testsupport"
	"github.com/genxcare/api/internal/users"
)

type fixture struct {
	medications   *medications.Service
	repo          *medications.Repository
	seniors       *seniors.Service
	relationships *relationships.Repository
	users         *users.Repository
}

func newFixture(t *testing.T) fixture {
	t.Helper()

	pool := testsupport.RequireDatabase(t)
	relationshipRepo := relationships.NewRepository(pool)
	seniorRepo := seniors.NewRepository(pool)
	medicationRepo := medications.NewRepository(pool)
	events := careevents.NewRepository(pool)
	recorder := careevents.NewRecorder(pool, events)

	return fixture{
		medications:   medications.NewService(medicationRepo, seniorRepo, recorder),
		repo:          medicationRepo,
		seniors:       seniors.NewService(seniorRepo, relationshipRepo),
		relationships: relationshipRepo,
		users:         users.NewRepository(pool),
	}
}

func (f fixture) newUser(t *testing.T, email string) auth.Principal {
	t.Helper()
	user, err := f.users.EnsureByAuthUserID(
		context.Background(), uuid.New(), email, users.DefaultDisplayName(email))
	if err != nil {
		t.Fatalf("create user %s: %v", email, err)
	}
	return auth.Principal{UserID: user.ID, AuthUserID: user.AuthUserID, Email: user.Email}
}

// newCircle creates a senior in a named timezone, so scheduling is tested in a
// zone that is not the machine's.
func (f fixture) newCircle(
	t *testing.T,
	owner auth.Principal,
	name, timezone string,
) seniors.Membership {
	t.Helper()

	membership, err := f.seniors.Create(context.Background(), owner, seniors.CreateInput{
		Mode:        seniors.CreateModeFamily,
		DisplayName: name,
		Timezone:    timezone,
	})
	if err != nil {
		t.Fatalf("create circle: %v", err)
	}
	return membership
}

func mustLoad(t *testing.T, name string) *time.Location {
	t.Helper()
	location, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return location
}

// dueLaterToday returns a wall-clock time still ahead on the senior's current
// local day.
//
// Doses are only generated from a schedule's creation onwards, and that
// timestamp comes from the database clock. A test that pinned "now" to a fixed
// date would agree with the database on the day it was written and disagree on
// every other one, which is how Phase 4's suite first passed at half past
// midnight and failed the same morning. Everything here derives from the real
// clock instead.
func dueLaterToday(now time.Time, location *time.Location) recurrence.TimeOfDay {
	local := now.In(location)

	if local.Hour() >= 23 {
		return recurrence.TimeOfDay{Hour: 23, Minute: 59}
	}
	return recurrence.TimeOfDay{Hour: local.Hour() + 1}
}

// startOfDay is midnight at the start of the senior's current local day.
func startOfDay(now time.Time, location *time.Location) time.Time {
	local := now.In(location)
	year, month, day := local.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, location)
}

func daily(at recurrence.TimeOfDay) medications.ScheduleInput {
	return medications.ScheduleInput{Recurrence: recurrence.Daily(), ScheduledTime: at}
}

// --- Creating a medication ---------------------------------------------------

func TestCreateMedicationWithNoSchedule(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	created, err := f.medications.Create(ctx, medications.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Name:            "Metformin",
		Dosage:          "500 mg",
		Form:            medications.FormTablet,
		Instructions:    "With food",
	}, now)
	if err != nil {
		t.Fatalf("create medication: %v", err)
	}

	if created.Medication.Name != "Metformin" {
		t.Errorf("name = %q", created.Medication.Name)
	}
	if created.Medication.Dosage != "500 mg" {
		t.Errorf("dosage = %q", created.Medication.Dosage)
	}
	if created.Medication.Form != medications.FormTablet {
		t.Errorf("form = %q", created.Medication.Form)
	}
	if !created.Medication.Active {
		t.Error("a new medication should be active")
	}
	if created.Medication.CreatedByUserID != owner.UserID {
		t.Error("the creator was not recorded")
	}
	// A medicine can be written down before anybody has decided when it is
	// taken; nothing is due until a time is added.
	if len(created.Instances) != 0 {
		t.Errorf("got %d doses with no schedule, want 0", len(created.Instances))
	}
}

func TestMistakenMedicationCanBeDeletedBeforeAnyDoseOutcome(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	owner := f.newUser(t, "delete-mistake@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")
	created, err := f.medications.Create(ctx, medications.CreateInput{
		SeniorID: circle.Senior.ID, CreatedByUserID: owner.UserID, Name: "Wrong medicine",
	}, time.Now())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := f.medications.DeleteMistaken(ctx, created.Medication.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := f.medications.Get(ctx, created.Medication.ID); !errors.Is(err, medications.ErrNotFound) {
		t.Fatalf("Get error = %v", err)
	}
}

func TestMedicationWithRecordedDoseCannotBeDeleted(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	owner := f.newUser(t, "keep-history@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")
	created, err := f.medications.Create(ctx, medications.CreateInput{
		SeniorID: circle.Senior.ID, CreatedByUserID: owner.UserID, Name: "Metformin",
		Schedules: []medications.ScheduleInput{daily(dueLaterToday(time.Now(), mustLoad(t, "Asia/Karachi")))},
	}, time.Now())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(created.Instances) == 0 {
		t.Fatal("expected a generated dose")
	}
	if _, err := f.medications.Act(ctx, medications.ActInput{
		InstanceID: created.Instances[0].ID, Action: medications.ActionTake, ActorID: owner.UserID,
	}); err != nil {
		t.Fatalf("take: %v", err)
	}
	if err := f.medications.DeleteMistaken(ctx, created.Medication.ID); !errors.Is(err, medications.ErrRecordedHistory) {
		t.Fatalf("DeleteMistaken error = %v", err)
	}
}

func TestCreateMedicationGeneratesTodaysDoses(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	karachi := mustLoad(t, "Asia/Karachi")
	now := time.Now()
	at := dueLaterToday(now, karachi)

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	created, err := f.medications.Create(ctx, medications.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Name:            "Metformin",
		Dosage:          "500 mg",
		Schedules:       []medications.ScheduleInput{daily(at)},
	}, now)
	if err != nil {
		t.Fatalf("create medication: %v", err)
	}

	if len(created.Schedules) != 1 {
		t.Fatalf("got %d schedules, want 1", len(created.Schedules))
	}
	if len(created.Instances) < 7 {
		t.Fatalf("got %d doses over the coming weeks, want at least 7", len(created.Instances))
	}

	first := created.Instances[0]
	local := first.ScheduledFor.In(karachi)
	if local.Hour() != at.Hour || local.Minute() != at.Minute {
		t.Errorf("dose at %02d:%02d local, want %s", local.Hour(), local.Minute(), at)
	}
	// The dose carries what was to be taken, so history is not rewritten by a
	// later change to the prescription.
	if first.Name != "Metformin" || first.Dosage != "500 mg" {
		t.Errorf("dose = %q %q, want the medicine's name and dosage", first.Name, first.Dosage)
	}
	if !first.Recurring() {
		t.Error("a dose from a schedule should report itself recurring")
	}
}

// "Every day at 08:00 and 20:00" is two schedules, not one rule with two times.
func TestTwiceDailyIsTwoSchedules(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	karachi := mustLoad(t, "Asia/Karachi")
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	created, err := f.medications.Create(ctx, medications.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Name:            "Metformin",
		Schedules: []medications.ScheduleInput{
			daily(recurrence.TimeOfDay{Hour: 8}),
			daily(recurrence.TimeOfDay{Hour: 20}),
		},
	}, now)
	if err != nil {
		t.Fatalf("create medication: %v", err)
	}

	if len(created.Schedules) != 2 {
		t.Fatalf("got %d schedules, want 2", len(created.Schedules))
	}

	// Tomorrow rather than today: a schedule never produces doses from before
	// it existed, so whether today's 08:00 exists depends on the hour this runs
	// at. Tomorrow has both, whenever it runs.
	doses, err := f.medications.ListDoses(ctx, medications.ListDosesInput{
		SeniorID: circle.Senior.ID,
		Scope:    medications.ScopeUpcoming,
	}, now)
	if err != nil {
		t.Fatalf("list upcoming: %v", err)
	}

	tomorrow := startOfDay(now, karachi).AddDate(0, 0, 1)
	hours := map[int]bool{}
	for _, dose := range doses {
		local := dose.ScheduledFor.In(karachi)
		if !local.Before(tomorrow) && local.Before(tomorrow.AddDate(0, 0, 1)) {
			hours[local.Hour()] = true
		}
	}
	if !hours[8] || !hours[20] {
		t.Errorf("tomorrow's doses are at %v, want both 08 and 20", hours)
	}
}

// The same medicine cannot be due twice at the same time under the same rule.
func TestARepeatedTimeIsRefused(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	created, err := f.medications.Create(ctx, medications.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Name:            "Metformin",
		Schedules:       []medications.ScheduleInput{daily(recurrence.TimeOfDay{Hour: 8})},
	}, now)
	if err != nil {
		t.Fatalf("create medication: %v", err)
	}

	_, err = f.medications.AddSchedule(ctx, created.Medication,
		daily(recurrence.TimeOfDay{Hour: 8}), now)

	if !errors.Is(err, medications.ErrDuplicateSchedule) {
		t.Errorf("err = %v, want ErrDuplicateSchedule", err)
	}
}

// --- Scheduling and timezones -----------------------------------------------

func TestTodayIsTheSeniorsOwnDay(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	karachi := mustLoad(t, "Asia/Karachi")
	now := time.Now()
	at := dueLaterToday(now, karachi)

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	if _, err := f.medications.Create(ctx, medications.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Name:            "Metformin",
		Schedules:       []medications.ScheduleInput{daily(at)},
	}, now); err != nil {
		t.Fatalf("create medication: %v", err)
	}

	doses, err := f.medications.ListDoses(ctx, medications.ListDosesInput{
		SeniorID: circle.Senior.ID,
		Scope:    medications.ScopeToday,
	}, now)
	if err != nil {
		t.Fatalf("list today: %v", err)
	}

	if len(doses) != 1 {
		t.Fatalf("got %d doses today, want 1", len(doses))
	}

	// Every dose returned must fall inside the senior's own calendar day, not
	// the server's.
	dayStart := startOfDay(now, karachi)
	dayEnd := dayStart.AddDate(0, 0, 1)
	for _, dose := range doses {
		if dose.ScheduledFor.Before(dayStart) || !dose.ScheduledFor.Before(dayEnd) {
			t.Errorf("dose at %s falls outside the senior's day", dose.ScheduledFor)
		}
	}
}

func TestWeeklyScheduleOnlyProducesTheChosenDays(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	london := mustLoad(t, "Europe/London")
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mr Ali", "Europe/London")

	created, err := f.medications.Create(ctx, medications.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Name:            "Vitamin D",
		Schedules: []medications.ScheduleInput{{
			Recurrence:    recurrence.Weekly(time.Monday, time.Wednesday, time.Friday),
			ScheduledTime: recurrence.TimeOfDay{Hour: 9},
		}},
	}, now)
	if err != nil {
		t.Fatalf("create medication: %v", err)
	}

	if len(created.Instances) == 0 {
		t.Fatal("a weekly schedule produced no doses")
	}
	for _, dose := range created.Instances {
		switch dose.ScheduledFor.In(london).Weekday() {
		case time.Monday, time.Wednesday, time.Friday:
		default:
			t.Errorf("dose on %s, want only Mon/Wed/Fri", dose.ScheduledFor.In(london).Weekday())
		}
	}
}

// Reading a window twice must not double-book somebody's medicine.
func TestGeneratingTheSameWindowTwiceIsIdempotent(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	karachi := mustLoad(t, "Asia/Karachi")
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	if _, err := f.medications.Create(ctx, medications.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Name:            "Metformin",
		Schedules:       []medications.ScheduleInput{daily(dueLaterToday(now, karachi))},
	}, now); err != nil {
		t.Fatalf("create medication: %v", err)
	}

	input := medications.ListDosesInput{SeniorID: circle.Senior.ID, Scope: medications.ScopeToday}

	first, err := f.medications.ListDoses(ctx, input, now)
	if err != nil {
		t.Fatalf("first read: %v", err)
	}
	second, err := f.medications.ListDoses(ctx, input, now)
	if err != nil {
		t.Fatalf("second read: %v", err)
	}

	if len(first) != len(second) {
		t.Errorf("reading twice produced %d then %d doses", len(first), len(second))
	}
}

// --- Recording a dose --------------------------------------------------------

func TestTakingADoseRecordsWhoAndWhen(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")
	dose := f.aDose(t, circle, owner, now)

	result, err := f.medications.Act(ctx, medications.ActInput{
		InstanceID: dose.ID,
		Action:     medications.ActionTake,
		ActorID:    owner.UserID,
	})
	if err != nil {
		t.Fatalf("take: %v", err)
	}

	if result.Instance.Status != medications.StatusTaken {
		t.Errorf("status = %q, want taken", result.Instance.Status)
	}
	if result.Instance.TakenBy == nil || *result.Instance.TakenBy != owner.UserID {
		t.Error("the actor was not recorded")
	}
	if result.Instance.TakenAt == nil {
		t.Error("the time was not recorded")
	}
	if result.Repeat {
		t.Error("a first action is not a repeat")
	}
}

func TestSkippingADoseKeepsItInTheRecord(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")
	dose := f.aDose(t, circle, owner, now)

	notes := "She was asleep"
	result, err := f.medications.Act(ctx, medications.ActInput{
		InstanceID: dose.ID,
		Action:     medications.ActionSkip,
		ActorID:    owner.UserID,
		Notes:      &notes,
	})
	if err != nil {
		t.Fatalf("skip: %v", err)
	}

	if result.Instance.Status != medications.StatusSkipped {
		t.Errorf("status = %q, want skipped", result.Instance.Status)
	}
	if result.Instance.SkippedBy == nil || *result.Instance.SkippedBy != owner.UserID {
		t.Error("the actor was not recorded")
	}
	if result.Instance.Notes != notes {
		t.Errorf("notes = %q", result.Instance.Notes)
	}

	// A skipped dose is care history, not an absence of one.
	kept, err := f.medications.GetInstance(ctx, dose.ID)
	if err != nil {
		t.Fatalf("the skipped dose was not kept: %v", err)
	}
	if kept.Status != medications.StatusSkipped {
		t.Errorf("status = %q after re-reading", kept.Status)
	}
}

// The offline queue replays what it holds; a duplicate must not record a second
// dose or move the timestamp.
func TestTakingTheSameDoseTwiceIsSafe(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")
	dose := f.aDose(t, circle, owner, now)

	input := medications.ActInput{
		InstanceID: dose.ID,
		Action:     medications.ActionTake,
		ActorID:    owner.UserID,
	}

	first, err := f.medications.Act(ctx, input)
	if err != nil {
		t.Fatalf("first take: %v", err)
	}
	second, err := f.medications.Act(ctx, input)
	if err != nil {
		t.Fatalf("second take should succeed quietly: %v", err)
	}

	if !second.Repeat {
		t.Error("the second take should report itself a repeat")
	}
	if !second.Instance.TakenAt.Equal(*first.Instance.TakenAt) {
		t.Error("a replayed action moved the time the medicine was taken")
	}
}

// Caregiver A marks it taken, caregiver B marks it skipped: the server is
// authoritative and the first outcome stands (plans/phase5.md §22).
func TestASecondCaregiverCannotOverwriteTheOutcome(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	other := f.newUser(t, "caregiver@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")
	dose := f.aDose(t, circle, owner, now)

	if _, err := f.medications.Act(ctx, medications.ActInput{
		InstanceID: dose.ID,
		Action:     medications.ActionTake,
		ActorID:    owner.UserID,
	}); err != nil {
		t.Fatalf("take: %v", err)
	}

	_, err := f.medications.Act(ctx, medications.ActInput{
		InstanceID: dose.ID,
		Action:     medications.ActionSkip,
		ActorID:    other.UserID,
	})
	if !errors.Is(err, medications.ErrInvalidTransition) {
		t.Fatalf("err = %v, want ErrInvalidTransition", err)
	}

	kept, err := f.medications.GetInstance(ctx, dose.ID)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if kept.Status != medications.StatusTaken {
		t.Errorf("status = %q, want the first outcome to stand", kept.Status)
	}
	if kept.TakenBy == nil || *kept.TakenBy != owner.UserID {
		t.Error("the original actor was overwritten")
	}
}

// --- Missed doses ------------------------------------------------------------

func TestADosePastItsWindowIsReportedMissed(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	created, err := f.medications.Create(ctx, medications.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Name:            "Metformin",
	}, now)
	if err != nil {
		t.Fatalf("create medication: %v", err)
	}

	// A dose whose window ran out three hours ago.
	stale := now.Add(-medications.MissedAfter - time.Hour)
	if _, err := f.medications.AddDose(ctx, created.Medication, owner.UserID, stale); err != nil {
		t.Fatalf("add dose: %v", err)
	}
	// And one that has only just fallen due, which is not missed yet.
	if _, err := f.medications.AddDose(
		ctx, created.Medication, owner.UserID, now.Add(-time.Minute),
	); err != nil {
		t.Fatalf("add recent dose: %v", err)
	}

	missed, err := f.medications.ListDoses(ctx, medications.ListDosesInput{
		SeniorID: circle.Senior.ID,
		Scope:    medications.ScopeMissed,
	}, now)
	if err != nil {
		t.Fatalf("list missed: %v", err)
	}

	if len(missed) != 1 {
		t.Fatalf("got %d missed doses, want 1", len(missed))
	}
	if !missed[0].Missed(now) {
		t.Error("the reported dose is not one whose window has passed")
	}
	if missed[0].EffectiveStatus(now) != medications.StatusMissed {
		t.Errorf("status = %q, want missed", missed[0].EffectiveStatus(now))
	}
	// Missed is never written down: the row is still pending underneath, which
	// is what lets it be taken late.
	if missed[0].Status != medications.StatusPending {
		t.Errorf("stored status = %q, want pending", missed[0].Status)
	}
}

func TestATakenOrSkippedDoseIsNeverMissed(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	created, err := f.medications.Create(ctx, medications.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Name:            "Metformin",
	}, now)
	if err != nil {
		t.Fatalf("create medication: %v", err)
	}

	stale := now.Add(-medications.MissedAfter - time.Hour)

	taken, err := f.medications.AddDose(ctx, created.Medication, owner.UserID, stale)
	if err != nil {
		t.Fatalf("add dose: %v", err)
	}
	skipped, err := f.medications.AddDose(
		ctx, created.Medication, owner.UserID, stale.Add(time.Minute))
	if err != nil {
		t.Fatalf("add dose: %v", err)
	}

	for _, act := range []struct {
		id     uuid.UUID
		action medications.Action
	}{
		{taken.ID, medications.ActionTake},
		{skipped.ID, medications.ActionSkip},
	} {
		if _, err := f.medications.Act(ctx, medications.ActInput{
			InstanceID: act.id,
			Action:     act.action,
			ActorID:    owner.UserID,
		}); err != nil {
			t.Fatalf("%s: %v", act.action, err)
		}
	}

	missed, err := f.medications.ListDoses(ctx, medications.ListDosesInput{
		SeniorID: circle.Senior.ID,
		Scope:    medications.ScopeMissed,
	}, now)
	if err != nil {
		t.Fatalf("list missed: %v", err)
	}

	if len(missed) != 0 {
		t.Errorf("got %d missed doses, want 0 — both were acted on", len(missed))
	}
}

// The gap the missed list has to close: nobody opened the app, so yesterday's
// dose was never written, and a query that only read existing rows would report
// nothing wrong.
func TestMissedDosesAreFoundWithoutAnybodyHavingLooked(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	karachi := mustLoad(t, "Asia/Karachi")
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	if _, err := f.medications.Create(ctx, medications.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Name:            "Metformin",
		Schedules:       []medications.ScheduleInput{daily(recurrence.TimeOfDay{Hour: 0})},
	}, now); err != nil {
		t.Fatalf("create medication: %v", err)
	}

	// Three days pass with nobody opening the app, so none of those doses were
	// ever written. Nothing reads today or any other window first: the missed
	// query has to generate the recent past itself.
	later := now.AddDate(0, 0, 3)

	missed, err := f.medications.ListDoses(ctx, medications.ListDosesInput{
		SeniorID: circle.Senior.ID,
		Scope:    medications.ScopeMissed,
	}, later)
	if err != nil {
		t.Fatalf("list missed: %v", err)
	}

	if len(missed) == 0 {
		t.Fatal("no missed doses found; the past was never generated")
	}
	for _, dose := range missed {
		if !dose.Missed(later) {
			t.Errorf("dose at %s is in the missed list but is not missed", dose.ScheduledFor)
		}
		if dose.ScheduledFor.In(karachi).Hour() != 0 {
			t.Errorf("dose at %s local, want midnight", dose.ScheduledFor.In(karachi))
		}
	}
}

// --- Editing -----------------------------------------------------------------

func TestEditingAMedicationLeavesHistoryAlone(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	created, err := f.medications.Create(ctx, medications.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Name:            "Metformin",
		Dosage:          "500 mg",
	}, now)
	if err != nil {
		t.Fatalf("create medication: %v", err)
	}

	past, err := f.medications.AddDose(
		ctx, created.Medication, owner.UserID, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("add past dose: %v", err)
	}
	if _, err := f.medications.Act(ctx, medications.ActInput{
		InstanceID: past.ID,
		Action:     medications.ActionTake,
		ActorID:    owner.UserID,
	}); err != nil {
		t.Fatalf("take past dose: %v", err)
	}

	dosage := "1000 mg"
	if _, err := f.medications.Update(ctx, medications.UpdateInput{
		MedicationID: created.Medication.ID,
		Dosage:       &dosage,
	}, now); err != nil {
		t.Fatalf("update medication: %v", err)
	}

	kept, err := f.medications.GetInstance(ctx, past.ID)
	if err != nil {
		t.Fatalf("re-read the past dose: %v", err)
	}

	// What was swallowed yesterday was 500 mg, and no edit today changes that.
	if kept.Dosage != "500 mg" {
		t.Errorf("historical dosage = %q, want it unchanged at 500 mg", kept.Dosage)
	}
	if kept.Status != medications.StatusTaken {
		t.Errorf("historical status = %q, want taken", kept.Status)
	}
}

func TestEditingAMedicationRefreshesFutureDoses(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	karachi := mustLoad(t, "Asia/Karachi")
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	created, err := f.medications.Create(ctx, medications.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Name:            "Metformin",
		Dosage:          "500 mg",
		Schedules:       []medications.ScheduleInput{daily(dueLaterToday(now, karachi))},
	}, now)
	if err != nil {
		t.Fatalf("create medication: %v", err)
	}

	dosage := "1000 mg"
	if _, err := f.medications.Update(ctx, medications.UpdateInput{
		MedicationID: created.Medication.ID,
		Dosage:       &dosage,
	}, now); err != nil {
		t.Fatalf("update medication: %v", err)
	}

	upcoming, err := f.medications.ListDoses(ctx, medications.ListDosesInput{
		SeniorID: circle.Senior.ID,
		Scope:    medications.ScopeUpcoming,
	}, now)
	if err != nil {
		t.Fatalf("list upcoming: %v", err)
	}

	if len(upcoming) == 0 {
		t.Fatal("no upcoming doses after the edit")
	}
	for _, dose := range upcoming {
		if dose.Dosage != "1000 mg" {
			t.Errorf("upcoming dose says %q, want the new dosage", dose.Dosage)
		}
	}
}

func TestStoppingAMedicationEndsFutureDosesAndKeepsThePast(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	karachi := mustLoad(t, "Asia/Karachi")
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	created, err := f.medications.Create(ctx, medications.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Name:            "Metformin",
		Schedules:       []medications.ScheduleInput{daily(dueLaterToday(now, karachi))},
	}, now)
	if err != nil {
		t.Fatalf("create medication: %v", err)
	}

	past, err := f.medications.AddDose(
		ctx, created.Medication, owner.UserID, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("add past dose: %v", err)
	}
	if _, err := f.medications.Act(ctx, medications.ActInput{
		InstanceID: past.ID,
		Action:     medications.ActionTake,
		ActorID:    owner.UserID,
	}); err != nil {
		t.Fatalf("take past dose: %v", err)
	}

	stopped := false
	updated, err := f.medications.Update(ctx, medications.UpdateInput{
		MedicationID: created.Medication.ID,
		Active:       &stopped,
	}, now)
	if err != nil {
		t.Fatalf("stop medication: %v", err)
	}
	if updated.Active {
		t.Fatal("the medication is still active")
	}

	upcoming, err := f.medications.ListDoses(ctx, medications.ListDosesInput{
		SeniorID: circle.Senior.ID,
		Scope:    medications.ScopeUpcoming,
	}, now)
	if err != nil {
		t.Fatalf("list upcoming: %v", err)
	}
	if len(upcoming) != 0 {
		t.Errorf("got %d upcoming doses after stopping, want 0", len(upcoming))
	}

	// The medicine itself, and what was taken, are still there.
	if _, err := f.medications.Get(ctx, created.Medication.ID); err != nil {
		t.Errorf("a stopped medication should still be readable: %v", err)
	}
	kept, err := f.medications.GetInstance(ctx, past.ID)
	if err != nil {
		t.Fatalf("re-read the past dose: %v", err)
	}
	if kept.Status != medications.StatusTaken {
		t.Errorf("historical status = %q, want taken", kept.Status)
	}
}

func TestStoppingOneTimeOfDayLeavesTheOther(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	karachi := mustLoad(t, "Asia/Karachi")
	now := time.Now()
	start := startOfDay(now, karachi)

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	created, err := f.medications.Create(ctx, medications.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Name:            "Metformin",
		Schedules: []medications.ScheduleInput{
			daily(recurrence.TimeOfDay{Hour: 8}),
			daily(recurrence.TimeOfDay{Hour: 20}),
		},
	}, start)
	if err != nil {
		t.Fatalf("create medication: %v", err)
	}

	evening := created.Schedules[1]
	stopped := false
	if _, err := f.medications.UpdateSchedule(ctx, created.Medication,
		medications.UpdateScheduleInput{ScheduleID: evening.ID, Active: &stopped},
		start,
	); err != nil {
		t.Fatalf("stop the evening dose: %v", err)
	}

	doses, err := f.medications.ListDoses(ctx, medications.ListDosesInput{
		SeniorID: circle.Senior.ID,
		Scope:    medications.ScopeUpcoming,
	}, now)
	if err != nil {
		t.Fatalf("list upcoming: %v", err)
	}

	if len(doses) == 0 {
		t.Fatal("stopping the evening dose removed the morning one too")
	}
	for _, dose := range doses {
		if hour := dose.ScheduledFor.In(karachi).Hour(); hour != 8 {
			t.Errorf("dose at %02d:00 local, want only the 08:00 one", hour)
		}
	}
}

func TestMovingAScheduleRebuildsOnlyTheFuture(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	karachi := mustLoad(t, "Asia/Karachi")
	now := time.Now()
	original := dueLaterToday(now, karachi)

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	created, err := f.medications.Create(ctx, medications.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Name:            "Metformin",
		Schedules:       []medications.ScheduleInput{daily(original)},
	}, now)
	if err != nil {
		t.Fatalf("create medication: %v", err)
	}

	// Take the first dose, then move the schedule to a different hour.
	doses, err := f.medications.ListDoses(ctx, medications.ListDosesInput{
		SeniorID: circle.Senior.ID,
		Scope:    medications.ScopeToday,
	}, now)
	if err != nil {
		t.Fatalf("list today: %v", err)
	}
	if len(doses) == 0 {
		t.Fatal("no dose today to record")
	}
	if _, err := f.medications.Act(ctx, medications.ActInput{
		InstanceID: doses[0].ID,
		Action:     medications.ActionTake,
		ActorID:    owner.UserID,
	}); err != nil {
		t.Fatalf("take: %v", err)
	}

	moved := recurrence.TimeOfDay{Hour: (original.Hour + 3) % 24}
	if _, err := f.medications.UpdateSchedule(ctx, created.Medication,
		medications.UpdateScheduleInput{
			ScheduleID:    created.Schedules[0].ID,
			ScheduledTime: &moved,
		},
		now,
	); err != nil {
		t.Fatalf("move the schedule: %v", err)
	}

	// The dose that was already taken keeps its original time.
	kept, err := f.medications.GetInstance(ctx, doses[0].ID)
	if err != nil {
		t.Fatalf("re-read the taken dose: %v", err)
	}
	if kept.ScheduledFor.In(karachi).Hour() != original.Hour {
		t.Errorf("the recorded dose moved to %02d:00", kept.ScheduledFor.In(karachi).Hour())
	}
	if kept.Status != medications.StatusTaken {
		t.Errorf("status = %q, want taken", kept.Status)
	}

	// And everything still to come is at the new time.
	upcoming, err := f.medications.ListDoses(ctx, medications.ListDosesInput{
		SeniorID: circle.Senior.ID,
		Scope:    medications.ScopeUpcoming,
	}, now)
	if err != nil {
		t.Fatalf("list upcoming: %v", err)
	}
	if len(upcoming) == 0 {
		t.Fatal("no upcoming doses after moving the schedule")
	}
	for _, dose := range upcoming {
		if hour := dose.ScheduledFor.In(karachi).Hour(); hour != moved.Hour {
			t.Errorf("upcoming dose at %02d:00, want %s", hour, moved)
		}
	}
}

// --- History -----------------------------------------------------------------

func TestHistoryIsPagedNewestFirst(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	created, err := f.medications.Create(ctx, medications.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Name:            "Metformin",
	}, now)
	if err != nil {
		t.Fatalf("create medication: %v", err)
	}

	for day := 1; day <= 5; day++ {
		if _, err := f.medications.AddDose(
			ctx, created.Medication, owner.UserID, now.AddDate(0, 0, -day),
		); err != nil {
			t.Fatalf("add dose: %v", err)
		}
	}

	first, err := f.medications.History(ctx, created.Medication, "", 2, now)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first.Items) != 2 {
		t.Fatalf("got %d doses on the first page, want 2", len(first.Items))
	}
	if first.NextCursor == "" {
		t.Fatal("there are more doses but no cursor was returned")
	}
	if first.Items[0].ScheduledFor.Before(first.Items[1].ScheduledFor) {
		t.Error("history is not newest first")
	}

	second, err := f.medications.History(ctx, created.Medication, first.NextCursor, 2, now)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second.Items) != 2 {
		t.Fatalf("got %d doses on the second page, want 2", len(second.Items))
	}

	// The pages must not overlap: a dose appearing twice would read as two
	// doses of the same medicine.
	for _, early := range first.Items {
		for _, later := range second.Items {
			if early.ID == later.ID {
				t.Errorf("dose %s appears on both pages", early.ID)
			}
		}
	}
	if !second.Items[0].ScheduledFor.Before(first.Items[1].ScheduledFor) {
		t.Error("the second page did not continue from the first")
	}
}

func TestHistoryEndsWithoutACursor(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	created, err := f.medications.Create(ctx, medications.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Name:            "Metformin",
	}, now)
	if err != nil {
		t.Fatalf("create medication: %v", err)
	}

	if _, err := f.medications.AddDose(
		ctx, created.Medication, owner.UserID, now.Add(-time.Hour),
	); err != nil {
		t.Fatalf("add dose: %v", err)
	}

	page, err := f.medications.History(ctx, created.Medication, "", 10, now)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("got %d doses, want 1", len(page.Items))
	}
	if page.NextCursor != "" {
		t.Error("a cursor was returned for a history with nothing after it")
	}
}

func TestHistoryRefusesANonsenseCursor(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	created, err := f.medications.Create(ctx, medications.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Name:            "Metformin",
	}, now)
	if err != nil {
		t.Fatalf("create medication: %v", err)
	}

	if _, err := f.medications.History(
		ctx, created.Medication, "not-a-cursor", 10, now,
	); !errors.Is(err, medications.ErrBadCursor) {
		t.Errorf("err = %v, want ErrBadCursor", err)
	}
}

// --- Next dose ---------------------------------------------------------------

func TestNextDoseIsTheEarliestOneStillWaiting(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	karachi := mustLoad(t, "Asia/Karachi")
	now := time.Now()
	at := dueLaterToday(now, karachi)

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	created, err := f.medications.Create(ctx, medications.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Name:            "Metformin",
		Schedules:       []medications.ScheduleInput{daily(at)},
	}, now)
	if err != nil {
		t.Fatalf("create medication: %v", err)
	}

	next, err := f.medications.NextDose(ctx, created.Medication, now)
	if err != nil {
		t.Fatalf("next dose: %v", err)
	}
	if next == nil {
		t.Fatal("no next dose for a live daily schedule")
	}
	if next.ScheduledFor.Before(now) {
		t.Error("the next dose is in the past")
	}

	stopped := false
	updated, err := f.medications.Update(ctx, medications.UpdateInput{
		MedicationID: created.Medication.ID,
		Active:       &stopped,
	}, now)
	if err != nil {
		t.Fatalf("stop medication: %v", err)
	}

	after, err := f.medications.NextDose(ctx, updated, now)
	if err != nil {
		t.Fatalf("next dose after stopping: %v", err)
	}
	if after != nil {
		t.Error("a stopped medication has no next dose")
	}
}

// --- Helpers -----------------------------------------------------------------

// aDose creates a medication with one dose already due, which most of the
// recording tests need and none of them care about the shape of.
func (f fixture) aDose(
	t *testing.T,
	circle seniors.Membership,
	owner auth.Principal,
	now time.Time,
) medications.Instance {
	t.Helper()
	ctx := context.Background()

	created, err := f.medications.Create(ctx, medications.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Name:            "Metformin",
		Dosage:          "500 mg",
	}, now)
	if err != nil {
		t.Fatalf("create medication: %v", err)
	}

	dose, err := f.medications.AddDose(ctx, created.Medication, owner.UserID, now)
	if err != nil {
		t.Fatalf("add dose: %v", err)
	}
	return dose
}
