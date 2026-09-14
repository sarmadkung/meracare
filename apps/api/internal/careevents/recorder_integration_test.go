package careevents_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/genxcare/api/internal/appointments"
	"github.com/genxcare/api/internal/auth"
	"github.com/genxcare/api/internal/care"
	"github.com/genxcare/api/internal/careevents"
	"github.com/genxcare/api/internal/database"
	"github.com/genxcare/api/internal/medications"
	"github.com/genxcare/api/internal/members"
	"github.com/genxcare/api/internal/recurrence"
	"github.com/genxcare/api/internal/relationships"
	"github.com/genxcare/api/internal/seniors"
	"github.com/genxcare/api/internal/tasks"
	"github.com/genxcare/api/internal/testsupport"
	"github.com/genxcare/api/internal/users"
)

// These tests drive the real domain services against a real database, because
// the property under test is not "an event can be written" but "the event and
// the change it describes commit together" — which only a transaction against
// PostgreSQL can demonstrate.

type fixture struct {
	pool *database.Pool

	events        *careevents.Service
	eventRepo     *careevents.Repository
	tasks         *tasks.Service
	medications   *medications.Service
	appointments  *appointments.Service
	members       *members.Service
	seniors       *seniors.Service
	relationships *relationships.Repository
	users         *users.Repository
}

func newFixture(t *testing.T) fixture {
	t.Helper()

	pool := testsupport.RequireDatabase(t)
	eventRepo := careevents.NewRepository(pool)
	recorder := careevents.NewRecorder(pool, eventRepo)
	relationshipRepo := relationships.NewRepository(pool)
	seniorRepo := seniors.NewRepository(pool)

	return fixture{
		pool:          pool,
		events:        careevents.NewService(eventRepo),
		eventRepo:     eventRepo,
		tasks:         tasks.NewService(tasks.NewRepository(pool), seniorRepo, relationshipRepo, recorder),
		medications:   medications.NewService(medications.NewRepository(pool), seniorRepo, recorder),
		appointments:  appointments.NewService(appointments.NewRepository(pool), seniorRepo, relationshipRepo, recorder),
		members:       members.NewService(relationshipRepo, recorder),
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

func (f fixture) newCircle(t *testing.T, owner auth.Principal, name string) seniors.Membership {
	t.Helper()
	membership, err := f.seniors.Create(context.Background(), owner, seniors.CreateInput{
		Mode:        seniors.CreateModeFamily,
		DisplayName: name,
		Timezone:    "Asia/Karachi",
	})
	if err != nil {
		t.Fatalf("create circle: %v", err)
	}
	return membership
}

// timeline reads a senior's whole activity, newest first.
func (f fixture) timeline(t *testing.T, seniorID uuid.UUID) []careevents.Event {
	t.Helper()
	page, err := f.events.Activity(context.Background(), seniorID, "", 100)
	if err != nil {
		t.Fatalf("read activity: %v", err)
	}
	return page.Items
}

func typesOf(events []careevents.Event) []careevents.Type {
	found := make([]careevents.Type, 0, len(events))
	for _, event := range events {
		found = append(found, event.Type)
	}
	return found
}

func countOf(events []careevents.Event, wanted careevents.Type) int {
	total := 0
	for _, event := range events {
		if event.Type == wanted {
			total++
		}
	}
	return total
}

func findEvent(t *testing.T, events []careevents.Event, wanted careevents.Type) careevents.Event {
	t.Helper()
	for _, event := range events {
		if event.Type == wanted {
			return event
		}
	}
	t.Fatalf("no %s in the timeline; found %v", wanted, typesOf(events))
	return careevents.Event{}
}

// --- Tasks -------------------------------------------------------------------

func TestTaskActionsAppearInTheTimeline(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan")

	at := now.Add(time.Hour)
	created, err := f.tasks.Create(ctx, tasks.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Title:           "Morning walk",
		ScheduledFor:    &at,
	}, now)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	instance := created.Instances[0]

	if _, err := f.tasks.Act(ctx, tasks.ActInput{
		InstanceID: instance.ID,
		Action:     tasks.ActionComplete,
		ActorID:    owner.UserID,
	}); err != nil {
		t.Fatalf("complete: %v", err)
	}

	timeline := f.timeline(t, circle.Senior.ID)

	completed := findEvent(t, timeline, careevents.TypeTaskCompleted)
	if completed.ActorUserID == nil || *completed.ActorUserID != owner.UserID {
		t.Errorf("actor = %v, want the authenticated caller %v", completed.ActorUserID, owner.UserID)
	}
	if completed.EntityType != careevents.EntityTask || completed.EntityID != instance.ID {
		t.Errorf("event points at %s %v, want task %v",
			completed.EntityType, completed.EntityID, instance.ID)
	}
	if completed.Metadata[careevents.MetaTaskTitle] != "Morning walk" {
		t.Errorf("metadata = %v, want the task title", completed.Metadata)
	}
	if completed.SeniorID != circle.Senior.ID {
		t.Errorf("senior = %v, want %v", completed.SeniorID, circle.Senior.ID)
	}

	findEvent(t, timeline, careevents.TypeTaskCreated)
}

func TestSkippingATaskIsItsOwnEvent(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan")

	at := now.Add(time.Hour)
	created, err := f.tasks.Create(ctx, tasks.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Title:           "Evening care",
		ScheduledFor:    &at,
	}, now)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	if _, err := f.tasks.Act(ctx, tasks.ActInput{
		InstanceID: created.Instances[0].ID,
		Action:     tasks.ActionSkip,
		ActorID:    owner.UserID,
	}); err != nil {
		t.Fatalf("skip: %v", err)
	}

	findEvent(t, f.timeline(t, circle.Senior.ID), careevents.TypeTaskSkipped)
}

// A recurring task produces one event for the routine somebody decided on, not
// one for each occurrence the server later writes: nobody did anything when
// tomorrow's row appeared (plans/phase7.md §2).
func TestGeneratedOccurrencesAreNotEvents(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan")

	rule := recurrence.Daily()
	dueTime := recurrence.TimeOfDay{Hour: 9}
	created, err := f.tasks.Create(ctx, tasks.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Title:           "Morning medication reminder",
		Recurrence:      &rule,
		DueTime:         &dueTime,
	}, now)
	if err != nil {
		t.Fatalf("create recurring task: %v", err)
	}
	if len(created.Instances) < 2 {
		t.Fatalf("expected several occurrences, got %d", len(created.Instances))
	}

	timeline := f.timeline(t, circle.Senior.ID)
	if got := countOf(timeline, careevents.TypeTaskCreated); got != 1 {
		t.Errorf("TASK_CREATED appeared %d times for %d occurrences, want exactly 1",
			got, len(created.Instances))
	}
}

// --- Medication --------------------------------------------------------------

func TestMedicationActionsAppearInTheTimeline(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan")

	created, err := f.medications.Create(ctx, medications.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Name:            "Metformin",
		Dosage:          "500 mg",
	}, now)
	if err != nil {
		t.Fatalf("create medication: %v", err)
	}

	dose, err := f.medications.AddDose(ctx, created.Medication, owner.UserID, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("add dose: %v", err)
	}

	if _, err := f.medications.Act(ctx, medications.ActInput{
		InstanceID: dose.ID,
		Action:     medications.ActionTake,
		ActorID:    owner.UserID,
	}); err != nil {
		t.Fatalf("take: %v", err)
	}

	timeline := f.timeline(t, circle.Senior.ID)

	findEvent(t, timeline, careevents.TypeMedicationCreated)

	taken := findEvent(t, timeline, careevents.TypeMedicationTaken)
	if taken.Metadata[careevents.MetaMedicationName] != "Metformin" ||
		taken.Metadata[careevents.MetaDosage] != "500 mg" {
		t.Errorf("metadata = %v, want the medicine and its dosage", taken.Metadata)
	}
	if taken.EntityID != dose.ID {
		t.Errorf("event points at %v, want the dose %v", taken.EntityID, dose.ID)
	}
}

func TestSkippingADoseIsItsOwnEvent(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan")

	created, err := f.medications.Create(ctx, medications.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Name:            "Metformin",
		Dosage:          "500 mg",
	}, now)
	if err != nil {
		t.Fatalf("create medication: %v", err)
	}

	dose, err := f.medications.AddDose(ctx, created.Medication, owner.UserID, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("add dose: %v", err)
	}

	if _, err := f.medications.Act(ctx, medications.ActInput{
		InstanceID: dose.ID,
		Action:     medications.ActionSkip,
		ActorID:    owner.UserID,
	}); err != nil {
		t.Fatalf("skip: %v", err)
	}

	findEvent(t, f.timeline(t, circle.Senior.ID), careevents.TypeMedicationSkipped)
}

// --- Appointments ------------------------------------------------------------

func TestAppointmentActionsAppearInTheTimeline(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan")

	appointment, err := f.appointments.Create(ctx, appointments.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Title:           "Cardiology review",
		ScheduledAt:     time.Now().Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("create appointment: %v", err)
	}

	if _, err := f.appointments.Act(ctx, appointments.ActInput{
		AppointmentID: appointment.ID,
		Action:        appointments.ActionCancel,
		ActorID:       owner.UserID,
	}); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	timeline := f.timeline(t, circle.Senior.ID)

	created := findEvent(t, timeline, careevents.TypeAppointmentCreated)
	if created.Metadata[careevents.MetaAppointmentName] != "Cardiology review" {
		t.Errorf("metadata = %v, want the appointment title", created.Metadata)
	}
	findEvent(t, timeline, careevents.TypeAppointmentCancelled)
}

func TestCompletingAnAppointmentIsItsOwnEvent(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan")

	appointment, err := f.appointments.Create(ctx, appointments.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Title:           "Blood test",
		ScheduledAt:     time.Now().Add(-2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("create appointment: %v", err)
	}

	if _, err := f.appointments.Act(ctx, appointments.ActInput{
		AppointmentID: appointment.ID,
		Action:        appointments.ActionComplete,
		ActorID:       owner.UserID,
	}); err != nil {
		t.Fatalf("complete: %v", err)
	}

	findEvent(t, f.timeline(t, circle.Senior.ID), careevents.TypeAppointmentCompleted)
}

// --- Care circle -------------------------------------------------------------

func TestRevokingAMemberAppearsInTheTimeline(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	owner := f.newUser(t, "owner@example.com")
	caregiver := f.newUser(t, "caregiver@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan")

	membership, err := f.relationships.Create(ctx, relationships.CreateParams{
		SeniorID:    circle.Senior.ID,
		UserID:      caregiver.UserID,
		Role:        care.RoleProfessionalCaregiver,
		Permissions: care.Normalise(care.DefaultPermissions(care.RoleProfessionalCaregiver)),
		Status:      care.StatusActive,
	})
	if err != nil {
		t.Fatalf("add member: %v", err)
	}

	if _, err := f.members.Revoke(ctx, circle.Senior.ID, membership.ID, owner.UserID); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	revoked := findEvent(t, f.timeline(t, circle.Senior.ID), careevents.TypeMemberRevoked)
	if revoked.ActorUserID == nil || *revoked.ActorUserID != owner.UserID {
		t.Errorf("actor = %v, want the member who did the removing", revoked.ActorUserID)
	}
	if revoked.Metadata[careevents.MetaRole] != string(care.RoleProfessionalCaregiver) {
		t.Errorf("metadata = %v, want the role that was removed", revoked.Metadata)
	}
}

// --- Idempotency -------------------------------------------------------------

// The offline queue replays a completion when the phone comes back online, and
// the state machine already recognises the second attempt as a repeat. Reusing
// that decision is what keeps one action out of the timeline twice
// (plans/phase7.md §20).
func TestRetryingAnActionDoesNotDuplicateItsEvent(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan")

	at := now.Add(time.Hour)
	created, err := f.tasks.Create(ctx, tasks.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Title:           "Morning walk",
		ScheduledFor:    &at,
	}, now)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	for attempt := 0; attempt < 3; attempt++ {
		result, err := f.tasks.Act(ctx, tasks.ActInput{
			InstanceID: created.Instances[0].ID,
			Action:     tasks.ActionComplete,
			ActorID:    owner.UserID,
		})
		if err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		if attempt > 0 && !result.Repeat {
			t.Errorf("attempt %d was not recognised as a repeat", attempt)
		}
	}

	if got := countOf(f.timeline(t, circle.Senior.ID), careevents.TypeTaskCompleted); got != 1 {
		t.Errorf("TASK_COMPLETED appeared %d times after three attempts, want exactly 1", got)
	}
}

// A refused action leaves nothing behind: no state change, and no event
// claiming one happened.
func TestARefusedActionRecordsNothing(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan")

	at := now.Add(time.Hour)
	created, err := f.tasks.Create(ctx, tasks.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Title:           "Morning walk",
		ScheduledFor:    &at,
	}, now)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	if _, err := f.tasks.Act(ctx, tasks.ActInput{
		InstanceID: created.Instances[0].ID,
		Action:     tasks.ActionComplete,
		ActorID:    owner.UserID,
	}); err != nil {
		t.Fatalf("complete: %v", err)
	}

	// Skipping something already completed contradicts the record and is refused.
	if _, err := f.tasks.Act(ctx, tasks.ActInput{
		InstanceID: created.Instances[0].ID,
		Action:     tasks.ActionSkip,
		ActorID:    owner.UserID,
	}); !errors.Is(err, tasks.ErrInvalidTransition) {
		t.Fatalf("err = %v, want ErrInvalidTransition", err)
	}

	timeline := f.timeline(t, circle.Senior.ID)
	if got := countOf(timeline, careevents.TypeTaskSkipped); got != 0 {
		t.Errorf("a refused skip wrote %d TASK_SKIPPED events, want none", got)
	}
	if got := countOf(timeline, careevents.TypeTaskCompleted); got != 1 {
		t.Errorf("TASK_COMPLETED count = %d, want the original 1", got)
	}
}

// --- Transactional consistency -----------------------------------------------

// The guarantee this phase rests on, demonstrated directly: when recording the
// event fails, the domain change must not be committed either. Without it, a
// completion could succeed while its event vanished, and nobody would ever
// know the timeline had a hole (plans/phase7.md §26).
func TestAFailedEventRollsBackTheDomainChange(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan")

	recorder := careevents.NewRecorder(f.pool, careevents.NewRepository(f.pool))
	appointmentRepo := appointments.NewRepository(f.pool)

	var created appointments.Appointment
	err := recorder.InTransaction(ctx, func(tx pgx.Tx, events *careevents.Repository) error {
		var err error
		created, err = appointmentRepo.WithTx(tx).Create(ctx, appointments.CreateParams{
			SeniorID:        circle.Senior.ID,
			CreatedByUserID: owner.UserID,
			Title:           "Should not survive",
			ScheduledAt:     time.Now().Add(24 * time.Hour),
		})
		if err != nil {
			return err
		}

		// An event type the database refuses, standing in for any failure on
		// the recording path.
		_, err = events.Record(ctx, careevents.RecordParams{
			SeniorID:   circle.Senior.ID,
			Type:       careevents.Type("NOT_A_REAL_EVENT"),
			EntityType: careevents.EntityAppointment,
			EntityID:   created.ID,
		})
		return err
	})
	if err == nil {
		t.Fatal("recording an invalid event succeeded, so the CHECK constraint is not doing its job")
	}

	// The appointment must not exist: it was written and rolled back.
	if _, err := appointmentRepo.Get(ctx, created.ID); !errors.Is(err, appointments.ErrNotFound) {
		t.Errorf("the appointment survived a failed event: err = %v, want ErrNotFound", err)
	}
	if events := f.timeline(t, circle.Senior.ID); len(events) != 0 {
		t.Errorf("timeline = %v, want it empty", typesOf(events))
	}
}

// And the mirror image: a domain failure must not leave an event behind
// claiming something happened.
func TestAFailedDomainChangeRecordsNoEvent(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan")

	// An appointment that ends before it starts, which the CHECK constraint
	// refuses.
	start := time.Now().Add(24 * time.Hour)
	before := start.Add(-time.Hour)
	if _, err := f.appointments.Create(ctx, appointments.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Title:           "Impossible",
		ScheduledAt:     start,
		EndsAt:          &before,
	}); err == nil {
		t.Fatal("an impossible appointment was accepted")
	}

	if events := f.timeline(t, circle.Senior.ID); len(events) != 0 {
		t.Errorf("a failed creation left %v behind, want nothing", typesOf(events))
	}
}

// --- Integrity ---------------------------------------------------------------

func TestTheDatabaseRefusesAnUndocumentedEventType(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan")

	for _, invented := range []careevents.Type{"TASK_UPDATED", "task_completed", "ANYTHING"} {
		if _, err := f.eventRepo.Record(ctx, careevents.RecordParams{
			SeniorID:    circle.Senior.ID,
			ActorUserID: &owner.UserID,
			Type:        invented,
			EntityType:  careevents.EntityTask,
			EntityID:    uuid.New(),
		}); err == nil {
			t.Errorf("%q was accepted, but it is not in the documented vocabulary", invented)
		}
	}
}

func TestTheDatabaseRefusesAnUnrecognisedEntityType(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan")

	if _, err := f.eventRepo.Record(ctx, careevents.RecordParams{
		SeniorID:    circle.Senior.ID,
		ActorUserID: &owner.UserID,
		Type:        careevents.TypeTaskCompleted,
		EntityType:  careevents.EntityType("senior"),
		EntityID:    uuid.New(),
	}); err == nil {
		t.Error("an unrecognised entity type was accepted")
	}
}

// --- Pagination --------------------------------------------------------------

// The property that matters for a feed: walking it must visit every event
// exactly once. Events sharing an instant are the case a timestamp-only cursor
// gets wrong, and they are routine here — a domain change and its event are
// written in one transaction, and a drained offline queue writes several at
// once (plans/phase7.md §12).
func TestTheTimelinePagesThroughWithoutRepeatingOrSkipping(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan")

	// Seven events, four of which deliberately share one instant.
	shared := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	written := map[uuid.UUID]bool{}

	for i := 0; i < 7; i++ {
		occurredAt := shared
		if i >= 4 {
			occurredAt = shared.Add(time.Duration(i) * time.Minute)
		}

		event, err := f.eventRepo.Record(ctx, careevents.RecordParams{
			SeniorID:    circle.Senior.ID,
			ActorUserID: &owner.UserID,
			Type:        careevents.TypeTaskCompleted,
			EntityType:  careevents.EntityTask,
			EntityID:    uuid.New(),
			OccurredAt:  occurredAt,
		})
		if err != nil {
			t.Fatalf("record event %d: %v", i, err)
		}
		written[event.ID] = true
	}

	seen := map[uuid.UUID]int{}
	cursor := ""
	for page := 0; ; page++ {
		if page > 10 {
			t.Fatal("the timeline never reached its end")
		}

		result, err := f.events.Activity(ctx, circle.Senior.ID, cursor, 2)
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		for _, event := range result.Items {
			seen[event.ID]++
		}
		if result.NextCursor == "" {
			break
		}
		cursor = result.NextCursor
	}

	for id := range written {
		if seen[id] != 1 {
			t.Errorf("event %v appeared %d times across the pages, want exactly once", id, seen[id])
		}
	}
	if len(seen) != len(written) {
		t.Errorf("saw %d distinct events, want %d", len(seen), len(written))
	}
}

func TestTheTimelineIsNewestFirst(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan")

	base := time.Now().Add(-24 * time.Hour)
	for i, offset := range []time.Duration{0, 2 * time.Hour, time.Hour} {
		if _, err := f.eventRepo.Record(ctx, careevents.RecordParams{
			SeniorID:    circle.Senior.ID,
			ActorUserID: &owner.UserID,
			Type:        careevents.TypeTaskCompleted,
			EntityType:  careevents.EntityTask,
			EntityID:    uuid.New(),
			OccurredAt:  base.Add(offset),
			Metadata:    careevents.Metadata{careevents.MetaTaskTitle: string(rune('A' + i))},
		}); err != nil {
			t.Fatalf("record event %d: %v", i, err)
		}
	}

	timeline := f.timeline(t, circle.Senior.ID)
	for i := 1; i < len(timeline); i++ {
		if timeline[i].OccurredAt.After(timeline[i-1].OccurredAt) {
			t.Fatalf("event %d is newer than the one before it, so the feed is not newest-first", i)
		}
	}
}

// One circle's activity must never appear in another's, whatever else is going
// on in the database.
func TestActivityDoesNotLeakBetweenSeniors(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	mine := f.newCircle(t, owner, "Mrs Khan")
	theirs := f.newCircle(t, owner, "Mr Ali")

	at := now.Add(time.Hour)
	if _, err := f.tasks.Create(ctx, tasks.CreateInput{
		SeniorID:        mine.Senior.ID,
		CreatedByUserID: owner.UserID,
		Title:           "Only for Mrs Khan",
		ScheduledFor:    &at,
	}, now); err != nil {
		t.Fatalf("create task: %v", err)
	}

	for _, event := range f.timeline(t, theirs.Senior.ID) {
		t.Errorf("Mr Ali's timeline contains %s from another circle", event.Type)
	}
	if len(f.timeline(t, mine.Senior.ID)) == 0 {
		t.Error("Mrs Khan's own timeline is empty")
	}
}

func TestAnUnreadableCursorIsRefused(t *testing.T) {
	f := newFixture(t)

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan")

	if _, err := f.events.Activity(
		context.Background(), circle.Senior.ID, "not-a-cursor", 10,
	); !errors.Is(err, careevents.ErrBadCursor) {
		t.Errorf("err = %v, want ErrBadCursor", err)
	}
}
