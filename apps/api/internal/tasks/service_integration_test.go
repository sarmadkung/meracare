package tasks_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/genxcare/api/internal/auth"
	"github.com/genxcare/api/internal/care"
	"github.com/genxcare/api/internal/careevents"
	"github.com/genxcare/api/internal/relationships"
	"github.com/genxcare/api/internal/seniors"
	"github.com/genxcare/api/internal/tasks"
	"github.com/genxcare/api/internal/testsupport"
	"github.com/genxcare/api/internal/users"
)

type fixture struct {
	tasks         *tasks.Service
	repo          *tasks.Repository
	seniors       *seniors.Service
	seniorRepo    *seniors.Repository
	relationships *relationships.Repository
	users         *users.Repository
}

func newFixture(t *testing.T) fixture {
	t.Helper()

	pool := testsupport.RequireDatabase(t)
	relationshipRepo := relationships.NewRepository(pool)
	seniorRepo := seniors.NewRepository(pool)
	taskRepo := tasks.NewRepository(pool)
	events := careevents.NewRepository(pool)
	recorder := careevents.NewRecorder(pool, events)

	return fixture{
		tasks:         tasks.NewService(taskRepo, seniorRepo, relationshipRepo, recorder),
		repo:          taskRepo,
		seniors:       seniors.NewService(seniorRepo, relationshipRepo),
		seniorRepo:    seniorRepo,
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
func (f fixture) newCircle(t *testing.T, owner auth.Principal, name, timezone string) seniors.Membership {
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

func (f fixture) addMember(
	t *testing.T,
	seniorID uuid.UUID,
	user auth.Principal,
	role care.Role,
	status care.RelationshipStatus,
) relationships.Relationship {
	t.Helper()

	relationship, err := f.relationships.Create(context.Background(), relationships.CreateParams{
		SeniorID:    seniorID,
		UserID:      user.UserID,
		Role:        role,
		Permissions: care.Normalise(care.DefaultPermissions(role)),
		Status:      status,
	})
	if err != nil {
		t.Fatalf("add member: %v", err)
	}
	return relationship
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
// Occurrences are only generated from a template's creation onwards, and that
// timestamp comes from the database clock. A test that pinned "now" to a fixed
// date would therefore agree with the database on the day it was written and
// disagree on every other one — which is exactly how the suite first passed at
// half past midnight and failed the same morning. Everything here is derived
// from the real clock instead.
func dueLaterToday(now time.Time, location *time.Location) tasks.TimeOfDay {
	local := now.In(location)

	if local.Hour() >= 23 {
		// The last minute of the day is the one moment this cannot find room
		// ahead of itself; nothing else in the suite depends on the hour.
		return tasks.TimeOfDay{Hour: 23, Minute: 59}
	}
	return tasks.TimeOfDay{Hour: local.Hour() + 1}
}

// startOfDay is midnight at the start of the senior's current local day.
func startOfDay(now time.Time, location *time.Location) time.Time {
	local := now.In(location)
	year, month, day := local.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, location)
}

// --- Creation ---------------------------------------------------------------

func TestCreateOneTimeTask(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")
	due := now.Add(48 * time.Hour)

	created, err := f.tasks.Create(ctx, tasks.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Title:           "Call Dr Ahmed",
		Description:     "Follow up on the blood test",
		ScheduledFor:    &due,
	}, now)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	if created.Template != nil {
		t.Error("a one-time task should not create a template")
	}
	if len(created.Instances) != 1 {
		t.Fatalf("got %d occurrences, want 1", len(created.Instances))
	}

	task := created.Instances[0]
	if task.Title != "Call Dr Ahmed" {
		t.Errorf("title = %q", task.Title)
	}
	if task.Recurring() {
		t.Error("a one-time task reported itself recurring")
	}
	if task.Status != tasks.StatusPending {
		t.Errorf("status = %q, want pending", task.Status)
	}
	if task.CreatedByUserID != owner.UserID {
		t.Error("the creator was not recorded")
	}
}

func TestCreateRecurringTaskProducesOccurrences(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	karachi := mustLoad(t, "Asia/Karachi")
	now := time.Now()
	dueTime := dueLaterToday(now, karachi)

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	recurrence := tasks.Weekly(time.Monday, time.Wednesday, time.Friday)

	created, err := f.tasks.Create(ctx, tasks.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Title:           "Morning walk",
		Recurrence:      &recurrence,
		DueTime:         &dueTime,
	}, now)
	if err != nil {
		t.Fatalf("create recurring task: %v", err)
	}

	if created.Template == nil {
		t.Fatal("a recurring task should create a template")
	}
	if len(created.Instances) == 0 {
		t.Fatal("a recurring task should produce occurrences straight away")
	}

	for _, instance := range created.Instances {
		local := instance.ScheduledFor.In(karachi)
		if local.Hour() != dueTime.Hour || local.Minute() != dueTime.Minute {
			t.Errorf("occurrence at %s is not %s in Karachi",
				local.Format(time.RFC3339), dueTime)
		}
		switch local.Weekday() {
		case time.Monday, time.Wednesday, time.Friday:
		default:
			t.Errorf("occurrence fell on a %s", local.Weekday())
		}
		if !instance.Recurring() {
			t.Error("an occurrence of a template reported itself one-time")
		}
	}
}

// §17: a task cannot be handed to somebody outside the circle.
func TestCreateRefusesAnAssigneeOutsideTheCircle(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	stranger := f.newUser(t, "stranger@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "UTC")
	due := now.Add(time.Hour)

	_, err := f.tasks.Create(ctx, tasks.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Title:           "Morning walk",
		AssignedUserID:  &stranger.UserID,
		ScheduledFor:    &due,
	}, now)

	if !errors.Is(err, tasks.ErrInvalidAssignee) {
		t.Fatalf("err = %v, want ErrInvalidAssignee", err)
	}
}

// A revoked caregiver must not be handed new work.
func TestCreateRefusesARevokedMemberAsAssignee(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	former := f.newUser(t, "former@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "UTC")
	f.addMember(t, circle.Senior.ID, former, care.RoleProfessionalCaregiver, care.StatusRevoked)

	due := now.Add(time.Hour)
	_, err := f.tasks.Create(ctx, tasks.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Title:           "Morning walk",
		AssignedUserID:  &former.UserID,
		ScheduledFor:    &due,
	}, now)

	if !errors.Is(err, tasks.ErrInvalidAssignee) {
		t.Fatalf("err = %v, want ErrInvalidAssignee", err)
	}
}

func TestCreateAcceptsAnActiveMemberAsAssignee(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	caregiver := f.newUser(t, "caregiver@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "UTC")
	f.addMember(t, circle.Senior.ID, caregiver, care.RoleProfessionalCaregiver, care.StatusActive)

	due := now.Add(time.Hour)
	created, err := f.tasks.Create(ctx, tasks.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Title:           "Morning walk",
		AssignedUserID:  &caregiver.UserID,
		ScheduledFor:    &due,
	}, now)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	if !created.Instances[0].AssignedTo(caregiver.UserID) {
		t.Error("the task was not assigned to the caregiver")
	}
}

// --- Retrieval --------------------------------------------------------------

func TestTodayReturnsTheSeniorsOwnDay(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	karachi := mustLoad(t, "Asia/Karachi")
	now := time.Now()
	dueTime := dueLaterToday(now, karachi)

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	recurrence := tasks.Daily()
	if _, err := f.tasks.Create(ctx, tasks.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Title:           "Morning walk",
		Recurrence:      &recurrence,
		DueTime:         &dueTime,
	}, now); err != nil {
		t.Fatalf("create recurring task: %v", err)
	}

	today, err := f.tasks.List(ctx, tasks.ListInput{
		SeniorID: circle.Senior.ID,
		Scope:    tasks.ScopeToday,
	}, now)
	if err != nil {
		t.Fatalf("list today: %v", err)
	}

	if len(today) != 1 {
		t.Fatalf("got %d tasks today, want 1", len(today))
	}

	// The day boundary is the senior's, not the server's: the occurrence must
	// fall on their local date, at their local time.
	local := today[0].ScheduledFor.In(karachi)
	if local.Day() != now.In(karachi).Day() {
		t.Errorf("today's task is on %s, want the senior's current day",
			local.Format(time.RFC3339))
	}
	if local.Hour() != dueTime.Hour || local.Minute() != dueTime.Minute {
		t.Errorf("today's task is at %s, want %s in Karachi", local.Format(time.RFC3339), dueTime)
	}
}

func TestUpcomingStartsTomorrow(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	karachi := mustLoad(t, "Asia/Karachi")
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	recurrence := tasks.Daily()
	dueTime := dueLaterToday(now, karachi)
	if _, err := f.tasks.Create(ctx, tasks.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Title:           "Morning walk",
		Recurrence:      &recurrence,
		DueTime:         &dueTime,
	}, now); err != nil {
		t.Fatalf("create recurring task: %v", err)
	}

	upcoming, err := f.tasks.List(ctx, tasks.ListInput{
		SeniorID: circle.Senior.ID,
		Scope:    tasks.ScopeUpcoming,
	}, now)
	if err != nil {
		t.Fatalf("list upcoming: %v", err)
	}

	if len(upcoming) != 7 {
		t.Fatalf("got %d upcoming tasks, want 7", len(upcoming))
	}

	// "Upcoming" begins tomorrow: today belongs to the today view.
	tomorrow := startOfDay(now, karachi).AddDate(0, 0, 1)
	for _, instance := range upcoming {
		if instance.ScheduledFor.Before(tomorrow) {
			t.Errorf("upcoming included %s, which is today or earlier",
				instance.ScheduledFor.In(karachi).Format(time.RFC3339))
		}
	}
}

// §12: overdue is derived, so a task becomes overdue purely by the clock
// advancing — with nothing written and no job having run.
func TestOverdueIsDeterminedWithoutWritingAnything(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "UTC")

	due := now.Add(-2 * time.Hour)
	created, err := f.tasks.Create(ctx, tasks.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Title:           "Check blood pressure",
		ScheduledFor:    &due,
	}, now)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	overdue, err := f.tasks.List(ctx, tasks.ListInput{
		SeniorID: circle.Senior.ID,
		Scope:    tasks.ScopeOverdue,
	}, now)
	if err != nil {
		t.Fatalf("list overdue: %v", err)
	}

	if len(overdue) != 1 || overdue[0].ID != created.Instances[0].ID {
		t.Fatalf("got %d overdue tasks, want the one we created", len(overdue))
	}
	if overdue[0].EffectiveStatus(now) != tasks.StatusOverdue {
		t.Error("the task did not report itself overdue")
	}

	// The stored status is still pending: nothing transitioned it.
	stored, err := f.tasks.Get(ctx, created.Instances[0].ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if stored.Status != tasks.StatusPending {
		t.Errorf("stored status = %q, want pending — overdue must not be persisted", stored.Status)
	}
}

// Materialising a window twice must not duplicate anybody's care.
func TestListingTheSameWindowTwiceIsIdempotent(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	karachi := mustLoad(t, "Asia/Karachi")
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	recurrence := tasks.Daily()
	dueTime := dueLaterToday(now, karachi)
	if _, err := f.tasks.Create(ctx, tasks.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Title:           "Morning walk",
		Recurrence:      &recurrence,
		DueTime:         &dueTime,
	}, now); err != nil {
		t.Fatalf("create recurring task: %v", err)
	}

	input := tasks.ListInput{SeniorID: circle.Senior.ID, Scope: tasks.ScopeUpcoming}

	first, err := f.tasks.List(ctx, input, now)
	if err != nil {
		t.Fatalf("first list: %v", err)
	}
	second, err := f.tasks.List(ctx, input, now)
	if err != nil {
		t.Fatalf("second list: %v", err)
	}

	if len(first) != len(second) {
		t.Fatalf("listing twice produced %d then %d tasks", len(first), len(second))
	}
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Fatal("listing twice produced different occurrences")
		}
	}
}

// Asking for a window before the task existed must not invent history and then
// report all of it overdue.
func TestOccurrencesAreNotInventedBeforeTheTaskExisted(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "UTC")

	recurrence := tasks.Daily()
	dueTime := tasks.TimeOfDay{Hour: 9}
	if _, err := f.tasks.Create(ctx, tasks.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Title:           "Morning walk",
		Recurrence:      &recurrence,
		DueTime:         &dueTime,
	}, now); err != nil {
		t.Fatalf("create recurring task: %v", err)
	}

	lastMonth, err := f.tasks.List(ctx, tasks.ListInput{
		SeniorID: circle.Senior.ID,
		Scope:    tasks.ScopeWindow,
		From:     now.AddDate(0, 0, -30),
		To:       now.AddDate(0, 0, -1),
	}, now)
	if err != nil {
		t.Fatalf("list last month: %v", err)
	}

	if len(lastMonth) != 0 {
		t.Errorf("got %d tasks before the routine was created, want 0", len(lastMonth))
	}
}

func TestWindowIsBounded(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "UTC")

	_, err := f.tasks.List(ctx, tasks.ListInput{
		SeniorID: circle.Senior.ID,
		Scope:    tasks.ScopeWindow,
		From:     now,
		To:       now.AddDate(1, 0, 0),
	}, now)

	if !errors.Is(err, tasks.ErrBadWindow) {
		t.Fatalf("err = %v, want ErrBadWindow", err)
	}
}

func TestBackwardsWindowIsRefused(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "UTC")

	_, err := f.tasks.List(ctx, tasks.ListInput{
		SeniorID: circle.Senior.ID,
		Scope:    tasks.ScopeWindow,
		From:     now,
		To:       now.Add(-time.Hour),
	}, now)

	if !errors.Is(err, tasks.ErrBadWindow) {
		t.Fatalf("err = %v, want ErrBadWindow", err)
	}
}

// --- Completion -------------------------------------------------------------

func TestCompleteRecordsTheAuthenticatedActor(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	caregiver := f.newUser(t, "caregiver@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "UTC")
	f.addMember(t, circle.Senior.ID, caregiver, care.RoleProfessionalCaregiver, care.StatusActive)

	task := f.oneTimeTask(t, circle.Senior.ID, owner, now.Add(time.Hour))

	before := time.Now()
	result, err := f.tasks.Act(ctx, tasks.ActInput{
		InstanceID: task.ID,
		Action:     tasks.ActionComplete,
		ActorID:    caregiver.UserID,
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	after := time.Now()

	if result.Repeat {
		t.Error("a first completion reported itself a repeat")
	}
	if result.Instance.Status != tasks.StatusCompleted {
		t.Errorf("status = %q, want completed", result.Instance.Status)
	}
	if result.Instance.CompletedBy == nil || *result.Instance.CompletedBy != caregiver.UserID {
		t.Error("the completing user was not recorded")
	}
	if result.Instance.CompletedAt == nil {
		t.Fatal("no completion timestamp was recorded")
	}
	// The timestamp comes from the database clock, so allow the window the
	// request actually spanned rather than an exact value.
	at := *result.Instance.CompletedAt
	if at.Before(before.Add(-time.Minute)) || at.After(after.Add(time.Minute)) {
		t.Errorf("completion time %s is outside the window %s..%s",
			at.Format(time.RFC3339), before.Format(time.RFC3339), after.Format(time.RFC3339))
	}
}

// §27: the offline queue retries, so the same completion arrives twice.
func TestCompletingTwiceIsSafeAndKeepsTheOriginalActor(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	caregiver := f.newUser(t, "caregiver@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "UTC")
	f.addMember(t, circle.Senior.ID, caregiver, care.RoleProfessionalCaregiver, care.StatusActive)

	task := f.oneTimeTask(t, circle.Senior.ID, owner, now.Add(time.Hour))

	first, err := f.tasks.Act(ctx, tasks.ActInput{
		InstanceID: task.ID, Action: tasks.ActionComplete, ActorID: caregiver.UserID,
	})
	if err != nil {
		t.Fatalf("first completion: %v", err)
	}

	// The retry arrives from a different member — the record must still name
	// whoever actually did the work.
	second, err := f.tasks.Act(ctx, tasks.ActInput{
		InstanceID: task.ID, Action: tasks.ActionComplete, ActorID: owner.UserID,
	})
	if err != nil {
		t.Fatalf("retried completion should succeed, got: %v", err)
	}

	if !second.Repeat {
		t.Error("the retry was not reported as a repeat")
	}
	if second.Instance.CompletedBy == nil || *second.Instance.CompletedBy != caregiver.UserID {
		t.Error("the retry overwrote the original actor")
	}
	if !second.Instance.CompletedAt.Equal(*first.Instance.CompletedAt) {
		t.Error("the retry moved the completion timestamp")
	}
}

// --- Skip -------------------------------------------------------------------

func TestSkipIsRecordedAsCareHistory(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "UTC")
	task := f.oneTimeTask(t, circle.Senior.ID, owner, now.Add(time.Hour))

	reason := "She was asleep"
	result, err := f.tasks.Act(ctx, tasks.ActInput{
		InstanceID: task.ID,
		Action:     tasks.ActionSkip,
		ActorID:    owner.UserID,
		Notes:      &reason,
	})
	if err != nil {
		t.Fatalf("skip: %v", err)
	}

	if result.Instance.Status != tasks.StatusSkipped {
		t.Errorf("status = %q, want skipped", result.Instance.Status)
	}
	if result.Instance.SkippedBy == nil || *result.Instance.SkippedBy != owner.UserID {
		t.Error("the skipping user was not recorded")
	}
	if result.Instance.SkippedAt == nil {
		t.Error("no skip timestamp was recorded")
	}
	if result.Instance.Notes != reason {
		t.Errorf("notes = %q, want %q", result.Instance.Notes, reason)
	}

	// The row survives: a skipped task is history, not an absence of one.
	stored, err := f.tasks.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("the skipped task should still exist: %v", err)
	}
	if stored.Status != tasks.StatusSkipped {
		t.Errorf("stored status = %q", stored.Status)
	}
}

func TestSkippingTwiceIsSafe(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "UTC")
	task := f.oneTimeTask(t, circle.Senior.ID, owner, now.Add(time.Hour))

	act := tasks.ActInput{InstanceID: task.ID, Action: tasks.ActionSkip, ActorID: owner.UserID}
	if _, err := f.tasks.Act(ctx, act); err != nil {
		t.Fatalf("first skip: %v", err)
	}

	result, err := f.tasks.Act(ctx, act)
	if err != nil {
		t.Fatalf("repeated skip should succeed, got: %v", err)
	}
	if !result.Repeat {
		t.Error("the repeat was not reported as one")
	}
}

// --- Conflicts --------------------------------------------------------------

// §28: one device completes, another skips. The server is authoritative.
func TestSkippingACompletedTaskIsRefused(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	other := f.newUser(t, "other@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "UTC")
	f.addMember(t, circle.Senior.ID, other, care.RoleFamilyMember, care.StatusActive)

	task := f.oneTimeTask(t, circle.Senior.ID, owner, now.Add(time.Hour))

	if _, err := f.tasks.Act(ctx, tasks.ActInput{
		InstanceID: task.ID, Action: tasks.ActionComplete, ActorID: owner.UserID,
	}); err != nil {
		t.Fatalf("complete: %v", err)
	}

	_, err := f.tasks.Act(ctx, tasks.ActInput{
		InstanceID: task.ID, Action: tasks.ActionSkip, ActorID: other.UserID,
	})
	if !errors.Is(err, tasks.ErrInvalidTransition) {
		t.Fatalf("err = %v, want ErrInvalidTransition", err)
	}

	// And the completion still stands.
	stored, err := f.tasks.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if stored.Status != tasks.StatusCompleted {
		t.Errorf("status = %q, want the completion to stand", stored.Status)
	}
}

func TestCompletingASkippedTaskIsRefused(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "UTC")
	task := f.oneTimeTask(t, circle.Senior.ID, owner, now.Add(time.Hour))

	if _, err := f.tasks.Act(ctx, tasks.ActInput{
		InstanceID: task.ID, Action: tasks.ActionSkip, ActorID: owner.UserID,
	}); err != nil {
		t.Fatalf("skip: %v", err)
	}

	_, err := f.tasks.Act(ctx, tasks.ActInput{
		InstanceID: task.ID, Action: tasks.ActionComplete, ActorID: owner.UserID,
	})
	if !errors.Is(err, tasks.ErrInvalidTransition) {
		t.Fatalf("err = %v, want ErrInvalidTransition", err)
	}
}

// --- Editing ----------------------------------------------------------------

// §18: changing the rule must not rewrite what already happened.
func TestEditingARecurrenceLeavesHistoryIntact(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	london := mustLoad(t, "Europe/London")
	created := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Europe/London")

	recurrence := tasks.Daily()
	dueTime := dueLaterToday(created, london)
	result, err := f.tasks.Create(ctx, tasks.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Title:           "Morning walk",
		Recurrence:      &recurrence,
		DueTime:         &dueTime,
	}, created)
	if err != nil {
		t.Fatalf("create recurring task: %v", err)
	}
	template := *result.Template

	// Three days later, complete one occurrence and skip another, then change
	// the rule to weekdays only.
	later := created.AddDate(0, 0, 3)

	past, err := f.tasks.List(ctx, tasks.ListInput{
		SeniorID: circle.Senior.ID,
		Scope:    tasks.ScopeWindow,
		From:     created,
		To:       later,
	}, later)
	if err != nil {
		t.Fatalf("list past: %v", err)
	}
	if len(past) < 2 {
		t.Fatalf("expected at least 2 past occurrences, got %d", len(past))
	}

	if _, err := f.tasks.Act(ctx, tasks.ActInput{
		InstanceID: past[0].ID, Action: tasks.ActionComplete, ActorID: owner.UserID,
	}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if _, err := f.tasks.Act(ctx, tasks.ActInput{
		InstanceID: past[1].ID, Action: tasks.ActionSkip, ActorID: owner.UserID,
	}); err != nil {
		t.Fatalf("skip: %v", err)
	}

	weekdays := tasks.Weekly(
		time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday)
	if _, err := f.tasks.UpdateTemplate(ctx, tasks.UpdateTemplateInput{
		TemplateID: template.ID,
		SeniorID:   circle.Senior.ID,
		Recurrence: &weekdays,
	}, later); err != nil {
		t.Fatalf("update template: %v", err)
	}

	// History is exactly as it was.
	completed, err := f.tasks.Get(ctx, past[0].ID)
	if err != nil {
		t.Fatalf("the completed occurrence should survive: %v", err)
	}
	if completed.Status != tasks.StatusCompleted {
		t.Errorf("completed occurrence is now %q", completed.Status)
	}

	skipped, err := f.tasks.Get(ctx, past[1].ID)
	if err != nil {
		t.Fatalf("the skipped occurrence should survive: %v", err)
	}
	if skipped.Status != tasks.StatusSkipped {
		t.Errorf("skipped occurrence is now %q", skipped.Status)
	}

	// And the future follows the new rule: no weekend occurrences.
	future, err := f.tasks.List(ctx, tasks.ListInput{
		SeniorID: circle.Senior.ID,
		Scope:    tasks.ScopeWindow,
		From:     later,
		To:       later.AddDate(0, 0, 14),
	}, later)
	if err != nil {
		t.Fatalf("list future: %v", err)
	}
	if len(future) == 0 {
		t.Fatal("the edited routine produced no future occurrences")
	}
	for _, instance := range future {
		switch instance.ScheduledFor.In(london).Weekday() {
		case time.Saturday, time.Sunday:
			t.Errorf("a weekend occurrence survived the edit: %s",
				instance.ScheduledFor.Format(time.RFC3339))
		default:
		}
	}
}

// §8: completing one occurrence must not touch the next.
func TestCompletingOneOccurrenceLeavesTomorrowPending(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	karachi := mustLoad(t, "Asia/Karachi")
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	recurrence := tasks.Daily()
	dueTime := dueLaterToday(now, karachi)
	created, err := f.tasks.Create(ctx, tasks.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Title:           "Morning walk",
		Recurrence:      &recurrence,
		DueTime:         &dueTime,
	}, now)
	if err != nil {
		t.Fatalf("create recurring task: %v", err)
	}
	if len(created.Instances) < 2 {
		t.Fatalf("expected several occurrences, got %d", len(created.Instances))
	}

	if _, err := f.tasks.Act(ctx, tasks.ActInput{
		InstanceID: created.Instances[0].ID,
		Action:     tasks.ActionComplete,
		ActorID:    owner.UserID,
	}); err != nil {
		t.Fatalf("complete today: %v", err)
	}

	tomorrow, err := f.tasks.Get(ctx, created.Instances[1].ID)
	if err != nil {
		t.Fatalf("get tomorrow: %v", err)
	}
	if tomorrow.Status != tasks.StatusPending {
		t.Errorf("tomorrow is %q, want pending", tomorrow.Status)
	}

	// The template is untouched too.
	template, err := f.tasks.GetTemplate(ctx, created.Template.ID)
	if err != nil {
		t.Fatalf("get template: %v", err)
	}
	if !template.Active {
		t.Error("completing one occurrence deactivated the routine")
	}
}

// §19: a routine is deactivated, never deleted, and its history stays.
func TestDeactivatingATemplateKeepsItsHistory(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	karachi := mustLoad(t, "Asia/Karachi")
	created := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	recurrence := tasks.Daily()
	dueTime := dueLaterToday(created, karachi)
	result, err := f.tasks.Create(ctx, tasks.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Title:           "Morning walk",
		Recurrence:      &recurrence,
		DueTime:         &dueTime,
	}, created)
	if err != nil {
		t.Fatalf("create recurring task: %v", err)
	}

	later := created.AddDate(0, 0, 3)
	done := result.Instances[0]
	if _, err := f.tasks.Act(ctx, tasks.ActInput{
		InstanceID: done.ID, Action: tasks.ActionComplete, ActorID: owner.UserID,
	}); err != nil {
		t.Fatalf("complete: %v", err)
	}

	inactive := false
	if _, err := f.tasks.UpdateTemplate(ctx, tasks.UpdateTemplateInput{
		TemplateID: result.Template.ID,
		SeniorID:   circle.Senior.ID,
		Active:     &inactive,
	}, later); err != nil {
		t.Fatalf("deactivate: %v", err)
	}

	// The completed occurrence survives.
	if _, err := f.tasks.Get(ctx, done.ID); err != nil {
		t.Errorf("the completed occurrence was lost: %v", err)
	}

	// It no longer appears among the live routines.
	templates, err := f.tasks.ListTemplates(ctx, circle.Senior.ID)
	if err != nil {
		t.Fatalf("list templates: %v", err)
	}
	if len(templates) != 0 {
		t.Errorf("got %d live routines, want 0", len(templates))
	}

	// And it produces nothing further.
	future, err := f.tasks.List(ctx, tasks.ListInput{
		SeniorID: circle.Senior.ID,
		Scope:    tasks.ScopeWindow,
		From:     later.AddDate(0, 0, 1),
		To:       later.AddDate(0, 0, 14),
	}, later)
	if err != nil {
		t.Fatalf("list future: %v", err)
	}
	if len(future) != 0 {
		t.Errorf("a deactivated routine produced %d future occurrences", len(future))
	}
}

// --- Assigned to me ---------------------------------------------------------

func TestAssignedTasksSpanEveryCircle(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	family := f.newUser(t, "family@example.com")
	caregiver := f.newUser(t, "caregiver@example.com")

	first := f.newCircle(t, family, "Mrs Khan", "UTC")
	second := f.newCircle(t, family, "Mr Ali", "UTC")
	f.addMember(t, first.Senior.ID, caregiver, care.RoleProfessionalCaregiver, care.StatusActive)
	f.addMember(t, second.Senior.ID, caregiver, care.RoleProfessionalCaregiver, care.StatusActive)

	f.assignedTask(t, first.Senior.ID, family, caregiver, now.Add(time.Hour))
	f.assignedTask(t, second.Senior.ID, family, caregiver, now.Add(2*time.Hour))

	mine, err := f.tasks.ListAssigned(ctx, caregiver.UserID, now, 7)
	if err != nil {
		t.Fatalf("list assigned: %v", err)
	}

	if len(mine) != 2 {
		t.Fatalf("got %d assigned tasks, want 2 across both circles", len(mine))
	}
}

// Revoking somebody's membership empties their task list immediately, without
// any reassignment sweep having to run first.
func TestRevokedMemberSeesNoAssignedTasks(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	family := f.newUser(t, "family@example.com")
	caregiver := f.newUser(t, "caregiver@example.com")

	circle := f.newCircle(t, family, "Mrs Khan", "UTC")
	membership := f.addMember(
		t, circle.Senior.ID, caregiver, care.RoleProfessionalCaregiver, care.StatusActive)

	f.assignedTask(t, circle.Senior.ID, family, caregiver, now.Add(time.Hour))

	before, err := f.tasks.ListAssigned(ctx, caregiver.UserID, now, 7)
	if err != nil {
		t.Fatalf("list assigned: %v", err)
	}
	if len(before) != 1 {
		t.Fatalf("got %d assigned tasks before revocation, want 1", len(before))
	}

	if _, err := f.relationships.RevokeMembership(ctx, membership.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	after, err := f.tasks.ListAssigned(ctx, caregiver.UserID, now, 7)
	if err != nil {
		t.Fatalf("list assigned after revocation: %v", err)
	}
	if len(after) != 0 {
		t.Errorf("a revoked member still sees %d assigned tasks", len(after))
	}
}

// --- Helpers ----------------------------------------------------------------

func (f fixture) oneTimeTask(
	t *testing.T,
	seniorID uuid.UUID,
	creator auth.Principal,
	due time.Time,
) tasks.Instance {
	t.Helper()

	created, err := f.tasks.Create(context.Background(), tasks.CreateInput{
		SeniorID:        seniorID,
		CreatedByUserID: creator.UserID,
		Title:           "Check blood pressure",
		ScheduledFor:    &due,
	}, time.Now())
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	return created.Instances[0]
}

func (f fixture) assignedTask(
	t *testing.T,
	seniorID uuid.UUID,
	creator, assignee auth.Principal,
	due time.Time,
) tasks.Instance {
	t.Helper()

	created, err := f.tasks.Create(context.Background(), tasks.CreateInput{
		SeniorID:        seniorID,
		CreatedByUserID: creator.UserID,
		Title:           "Morning visit",
		AssignedUserID:  &assignee.UserID,
		ScheduledFor:    &due,
	}, time.Now())
	if err != nil {
		t.Fatalf("create assigned task: %v", err)
	}
	return created.Instances[0]
}
