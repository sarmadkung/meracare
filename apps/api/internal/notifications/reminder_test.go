package notifications

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/genxcare/api/internal/care"
)

// These tests exercise the plan directly rather than over HTTP. The questions
// they ask — is the identifier stable, does a lead time land where it should —
// are about the calculation, and a router in front of it would only make a
// failure harder to read. The authorization and persistence questions are asked
// over the real stack in handler_integration_test.go.

// stubSource returns a fixed set of due items for every senior.
type stubSource struct {
	due []Due
	// window records the range the plan asked for, so a test can assert the
	// lead time widened it.
	window [2]time.Time
	calls  int
}

func (s *stubSource) Upcoming(_ context.Context, _ uuid.UUID, from, to time.Time) ([]Due, error) {
	s.calls++
	s.window = [2]time.Time{from, to}

	inWindow := make([]Due, 0, len(s.due))
	for _, item := range s.due {
		if !item.At.Before(from) && item.At.Before(to) {
			inWindow = append(inWindow, item)
		}
	}
	return inWindow, nil
}

func allPermissions() care.PermissionSet {
	return care.Normalise(care.Permissions)
}

func testSenior() Senior {
	return Senior{
		ID:          uuid.MustParse("11111111-1111-4111-8111-111111111111"),
		DisplayName: "Amma",
		Timezone:    "Asia/Karachi",
	}
}

// plan is a small helper: one senior, full permissions, default preferences.
func plan(t *testing.T, sources map[ReminderType]ScheduleSource, userID uuid.UUID, now time.Time) []Reminder {
	t.Helper()

	memberships := []Membership{{Senior: testSenior(), Permissions: allPermissions()}}

	reminders, err := planFor(
		context.Background(), sources, memberships, userID,
		DefaultPreferences(userID), now, now.Add(horizon),
	)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	return reminders
}

func TestLeadTimesMatchTheDocumentedExamples(t *testing.T) {
	// The three worked examples in plans/phase8.md §§12, 13, 14. They are the
	// closest thing to a specification of the offsets, so they are asserted
	// literally: a dose at 08:00 reminds at 07:45, a task due 09:00 at 08:45,
	// an appointment at 14:00 at 13:00.
	now := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	userID := uuid.New()

	cases := []struct {
		name       string
		reminder   ReminderType
		dueAt      time.Time
		wantFireAt time.Time
	}{
		{
			name:       "a dose at 08:00 reminds at 07:45",
			reminder:   ReminderMedicationReminder,
			dueAt:      time.Date(2026, 8, 19, 8, 0, 0, 0, time.UTC),
			wantFireAt: time.Date(2026, 8, 19, 7, 45, 0, 0, time.UTC),
		},
		{
			name:       "a task due at 09:00 reminds at 08:45",
			reminder:   ReminderTaskReminder,
			dueAt:      time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC),
			wantFireAt: time.Date(2026, 8, 19, 8, 45, 0, 0, time.UTC),
		},
		{
			name:       "an appointment at 14:00 reminds at 13:00",
			reminder:   ReminderAppointmentReminder,
			dueAt:      time.Date(2026, 8, 19, 14, 0, 0, 0, time.UTC),
			wantFireAt: time.Date(2026, 8, 19, 13, 0, 0, 0, time.UTC),
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			source := &stubSource{due: []Due{{EntityID: uuid.New(), At: testCase.dueAt}}}
			sources := emptySources()
			sources[testCase.reminder] = source

			reminders := plan(t, sources, userID, now)

			if len(reminders) != 1 {
				t.Fatalf("got %d reminders, want 1", len(reminders))
			}
			if !reminders[0].FireAt.Equal(testCase.wantFireAt) {
				t.Errorf("fireAt = %s, want %s", reminders[0].FireAt, testCase.wantFireAt)
			}
			if !reminders[0].DueAt.Equal(testCase.dueAt) {
				t.Errorf("dueAt = %s, want %s", reminders[0].DueAt, testCase.dueAt)
			}
		})
	}
}

// emptySources builds a source map where every type returns nothing.
func emptySources() map[ReminderType]ScheduleSource {
	sources := make(map[ReminderType]ScheduleSource, len(ReminderTypes))
	for _, reminderType := range ReminderTypes {
		sources[reminderType] = &stubSource{}
	}
	return sources
}

func TestPlanningTwiceProducesTheSameIdentifiers(t *testing.T) {
	// The whole idempotency mechanism. If this ever stops holding, every device
	// schedules a duplicate of every reminder on every refresh
	// (plans/phase8.md §§25, 26).
	now := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	userID := uuid.New()
	morning, evening := uuid.New(), uuid.New()

	build := func() []Reminder {
		sources := emptySources()
		sources[ReminderMedicationReminder] = &stubSource{due: []Due{
			{EntityID: morning, At: now.Add(3 * time.Hour)},
			{EntityID: evening, At: now.Add(5 * time.Hour)},
		}}
		return plan(t, sources, userID, now)
	}

	first, second := build(), build()

	if len(first) != 2 {
		t.Fatalf("got %d reminders, want 2", len(first))
	}
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Errorf("reminder %d: id changed between plans, %s then %s", i, first[i].ID, second[i].ID)
		}
	}
}

func TestTwoUsersGetDifferentIdentifiersForTheSameDose(t *testing.T) {
	// Two caregivers reminded about one dose must schedule on their own
	// devices independently. Sharing an identifier would be harmless today but
	// wrong the moment anything is recorded against it.
	now := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	entityID := uuid.New()

	build := func(userID uuid.UUID) Reminder {
		sources := emptySources()
		sources[ReminderMedicationReminder] = &stubSource{
			due: []Due{{EntityID: entityID, At: now.Add(3 * time.Hour)}},
		}
		reminders := plan(t, sources, userID, now)
		if len(reminders) != 1 {
			t.Fatalf("got %d reminders, want 1", len(reminders))
		}
		return reminders[0]
	}

	if a, b := build(uuid.New()), build(uuid.New()); a.ID == b.ID {
		t.Errorf("two users share reminder id %s", a.ID)
	}
}

func TestMovingTheDoseChangesTheReminderIdentity(t *testing.T) {
	// A rescheduled dose must produce a different reminder, so the device
	// cancels the old one instead of leaving a stale alert
	// (plans/phase8.md §22).
	now := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	userID := uuid.New()
	entityID := uuid.New()

	build := func(dueAt time.Time) Reminder {
		sources := emptySources()
		sources[ReminderMedicationReminder] = &stubSource{due: []Due{{EntityID: entityID, At: dueAt}}}
		reminders := plan(t, sources, userID, now)
		if len(reminders) != 1 {
			t.Fatalf("got %d reminders, want 1", len(reminders))
		}
		return reminders[0]
	}

	original := build(now.Add(3 * time.Hour))
	moved := build(now.Add(4 * time.Hour))

	if original.ID == moved.ID {
		t.Errorf("moving the dose kept reminder id %s", original.ID)
	}
}

func TestPreferencesSilenceOneCategoryOnly(t *testing.T) {
	now := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	userID := uuid.New()

	sources := emptySources()
	sources[ReminderMedicationReminder] = &stubSource{
		due: []Due{{EntityID: uuid.New(), At: now.Add(2 * time.Hour)}},
	}
	sources[ReminderTaskReminder] = &stubSource{
		due: []Due{{EntityID: uuid.New(), At: now.Add(2 * time.Hour)}},
	}

	preferences := DefaultPreferences(userID)
	preferences.MedicationReminders = false

	reminders, err := planFor(
		context.Background(), sources,
		[]Membership{{Senior: testSenior(), Permissions: allPermissions()}},
		userID, preferences, now, now.Add(horizon),
	)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	if len(reminders) != 1 {
		t.Fatalf("got %d reminders, want 1", len(reminders))
	}
	if reminders[0].Type != ReminderTaskReminder {
		t.Errorf("type = %s, want the task reminder to survive", reminders[0].Type)
	}
}

func TestSilencedCategoriesAreNotEvenQueried(t *testing.T) {
	// Turning medication reminders off should not cost a database round trip
	// per senior to discover there is nothing to do (plans/phase8.md §46).
	now := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	userID := uuid.New()

	medication := &stubSource{}
	sources := emptySources()
	sources[ReminderMedicationReminder] = medication

	preferences := DefaultPreferences(userID)
	preferences.MedicationReminders = false

	if _, err := planFor(
		context.Background(), sources,
		[]Membership{{Senior: testSenior(), Permissions: allPermissions()}},
		userID, preferences, now, now.Add(horizon),
	); err != nil {
		t.Fatalf("plan: %v", err)
	}

	if medication.calls != 0 {
		t.Errorf("medication source called %d times, want 0", medication.calls)
	}
}

func TestAPermissionTheRelationshipLacksProducesNoReminders(t *testing.T) {
	// The reminder must not become a way around the permission set: a caregiver
	// who cannot see medication cannot be told about it either
	// (plans/phase8.md §5).
	now := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	userID := uuid.New()

	medication := &stubSource{due: []Due{{EntityID: uuid.New(), At: now.Add(2 * time.Hour)}}}
	sources := emptySources()
	sources[ReminderMedicationReminder] = medication

	reminders, err := planFor(
		context.Background(), sources,
		[]Membership{{
			Senior:      testSenior(),
			Permissions: care.Normalise([]care.Permission{care.PermissionTasksView}),
		}},
		userID, DefaultPreferences(userID), now, now.Add(horizon),
	)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	if len(reminders) != 0 {
		t.Fatalf("got %d reminders, want none", len(reminders))
	}
	if medication.calls != 0 {
		t.Errorf("medication source called %d times, want 0", medication.calls)
	}
}

func TestNoMembershipsMeansNoReminders(t *testing.T) {
	// What a revoked caregiver's plan looks like: the circle source returns no
	// active membership, so nothing is even asked about (plans/phase8.md §23).
	now := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	userID := uuid.New()

	medication := &stubSource{due: []Due{{EntityID: uuid.New(), At: now.Add(2 * time.Hour)}}}
	sources := emptySources()
	sources[ReminderMedicationReminder] = medication

	reminders, err := planFor(
		context.Background(), sources, nil, userID,
		DefaultPreferences(userID), now, now.Add(horizon),
	)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	if len(reminders) != 0 {
		t.Fatalf("got %d reminders, want none", len(reminders))
	}
	if medication.calls != 0 {
		t.Errorf("medication source called %d times, want 0", medication.calls)
	}
}

func TestAnAssignedTaskRemindsOnlyTheAssignee(t *testing.T) {
	now := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	assignee := uuid.New()
	other := uuid.New()

	build := func(userID uuid.UUID) []Reminder {
		sources := emptySources()
		sources[ReminderTaskReminder] = &stubSource{due: []Due{
			{EntityID: uuid.New(), At: now.Add(2 * time.Hour), AssigneeID: &assignee},
		}}
		return plan(t, sources, userID, now)
	}

	if got := len(build(assignee)); got != 1 {
		t.Errorf("assignee got %d reminders, want 1", got)
	}
	if got := len(build(other)); got != 0 {
		t.Errorf("a circle member who is not the assignee got %d reminders, want 0", got)
	}
}

func TestAnUnassignedTaskRemindsEveryoneWhoCanSeeIt(t *testing.T) {
	now := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)

	build := func(userID uuid.UUID) []Reminder {
		sources := emptySources()
		sources[ReminderTaskReminder] = &stubSource{due: []Due{
			{EntityID: uuid.New(), At: now.Add(2 * time.Hour)},
		}}
		return plan(t, sources, userID, now)
	}

	if got := len(build(uuid.New())); got != 1 {
		t.Errorf("got %d reminders, want 1", got)
	}
	if got := len(build(uuid.New())); got != 1 {
		t.Errorf("got %d reminders for a second member, want 1", got)
	}
}

func TestAReminderWhoseMomentHasPassedIsNotPlanned(t *testing.T) {
	// A dose due in five minutes reminds fifteen minutes before, which was ten
	// minutes ago. Scheduling that would fire immediately and tell somebody to
	// do something they are already meant to be doing.
	now := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	userID := uuid.New()

	sources := emptySources()
	sources[ReminderMedicationReminder] = &stubSource{due: []Due{
		{EntityID: uuid.New(), At: now.Add(5 * time.Minute)},
	}}

	if reminders := plan(t, sources, userID, now); len(reminders) != 0 {
		t.Fatalf("got %d reminders, want none", len(reminders))
	}
}

func TestTheWindowIsWidenedByTheLeadTime(t *testing.T) {
	// An appointment an hour past the horizon still reminds inside it, so the
	// source has to be asked past the horizon.
	now := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	userID := uuid.New()

	appointments := &stubSource{}
	sources := emptySources()
	sources[ReminderAppointmentReminder] = appointments

	plan(t, sources, userID, now)

	wantEnd := now.Add(horizon).Add(time.Hour)
	if !appointments.window[1].Equal(wantEnd) {
		t.Errorf("asked appointments up to %s, want %s", appointments.window[1], wantEnd)
	}
}

func TestSomethingDueJustPastTheHorizonStillReminds(t *testing.T) {
	now := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	userID := uuid.New()

	sources := emptySources()
	sources[ReminderAppointmentReminder] = &stubSource{due: []Due{
		// Half an hour after the horizon closes; its reminder is half an hour
		// before it closes.
		{EntityID: uuid.New(), At: now.Add(horizon).Add(30 * time.Minute)},
	}}

	if reminders := plan(t, sources, userID, now); len(reminders) != 1 {
		t.Fatalf("got %d reminders, want 1", len(reminders))
	}
}

func TestThePlanIsCappedAndSoonestFirst(t *testing.T) {
	// iOS drops pending local notifications past its own limit, so a plan
	// larger than the cap would be partly fictional. What survives must be the
	// soonest, not an arbitrary slice (plans/phase8.md §46).
	now := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	userID := uuid.New()

	due := make([]Due, 0, maxReminders*2)
	for i := range maxReminders * 2 {
		due = append(due, Due{
			EntityID: uuid.New(),
			At:       now.Add(time.Duration(i+1) * time.Hour),
		})
	}

	sources := emptySources()
	sources[ReminderMedicationReminder] = &stubSource{due: due}

	reminders := plan(t, sources, userID, now)

	if len(reminders) != maxReminders {
		t.Fatalf("got %d reminders, want %d", len(reminders), maxReminders)
	}
	for i := 1; i < len(reminders); i++ {
		if reminders[i].FireAt.Before(reminders[i-1].FireAt) {
			t.Fatalf("reminder %d fires before its predecessor", i)
		}
	}
	// The last kept reminder is the cap'th soonest, not something later.
	wantLast := now.Add(time.Duration(maxReminders) * time.Hour).Add(-15 * time.Minute)
	if !reminders[len(reminders)-1].FireAt.Equal(wantLast) {
		t.Errorf("last reminder fires at %s, want %s", reminders[len(reminders)-1].FireAt, wantLast)
	}
}

func TestTheSeniorsTimezoneTravelsWithTheReminder(t *testing.T) {
	// A daughter in London must see her mother's 08:00, not her own
	// (plans/phase8.md §32).
	now := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	userID := uuid.New()

	sources := emptySources()
	sources[ReminderMedicationReminder] = &stubSource{due: []Due{
		{EntityID: uuid.New(), At: now.Add(3 * time.Hour)},
	}}

	reminders := plan(t, sources, userID, now)
	if len(reminders) != 1 {
		t.Fatalf("got %d reminders, want 1", len(reminders))
	}
	if reminders[0].SeniorTimezone != "Asia/Karachi" {
		t.Errorf("timezone = %q, want the senior's", reminders[0].SeniorTimezone)
	}
	if reminders[0].SeniorName != "Amma" {
		t.Errorf("senior name = %q, want the senior's", reminders[0].SeniorName)
	}
}

func TestEveryReminderTypeHasExactlyOneEntityType(t *testing.T) {
	// The client switches on entityType to open a screen. A type that could
	// point at two kinds of thing would need a combination it never handles.
	seen := map[EntityType]ReminderType{}
	for _, reminderType := range ReminderTypes {
		entity := reminderType.entityFor()
		if existing, ok := seen[entity]; ok {
			t.Errorf("%s and %s both point at %s", existing, reminderType, entity)
		}
		seen[entity] = reminderType

		if !reminderType.Valid() {
			t.Errorf("%s is not in ReminderTypes", reminderType)
		}
	}
}
