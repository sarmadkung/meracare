package notifications

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/meracare/api/internal/care"
)

// These tests drive the decision half of the scheduler directly: given a
// roster, some preferences, and some care falling due, which notifications
// should exist? It is a pure function of its input, so there is no database and
// no clock — the instant is an argument, which is what lets a daylight-saving
// boundary or a 24-hour appointment lead be tested exactly rather than
// approximately (plans/phase11.md §63).

var (
	kolkata = mustLoad("Asia/Kolkata")
	london  = mustLoad("Europe/London")
)

func mustLoad(name string) *time.Location {
	location, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return location
}

// fakeSchedule returns fixed due items for every senior.
type fakeSchedule struct {
	due []Due
	// window records the last range asked for, so a test can check the query
	// was widened enough to find a 24-hour reminder.
	from, to time.Time
}

func (f *fakeSchedule) Upcoming(_ context.Context, _ uuid.UUID, from, to time.Time) ([]Due, error) {
	f.from, f.to = from, to

	found := make([]Due, 0, len(f.due))
	for _, item := range f.due {
		if !item.At.Before(from) && item.At.Before(to) {
			found = append(found, item)
		}
	}
	return found, nil
}

// fakeOverdue returns fixed overdue items for every senior.
type fakeOverdue struct {
	due []Due
}

func (f fakeOverdue) Overdue(_ context.Context, _ uuid.UUID, from, to time.Time) ([]Due, error) {
	found := make([]Due, 0, len(f.due))
	for _, item := range f.due {
		if !item.At.Before(from) && item.At.Before(to) {
			found = append(found, item)
		}
	}
	return found, nil
}

// fakeActivity returns fixed care events.
type fakeActivity struct {
	activities []Activity
}

func (f fakeActivity) RecentActivity(_ context.Context, from, to time.Time) ([]Activity, error) {
	found := make([]Activity, 0, len(f.activities))
	for _, activity := range f.activities {
		if !activity.OccurredAt.Before(from) && activity.OccurredAt.Before(to) {
			found = append(found, activity)
		}
	}
	return found, nil
}

var everything = care.Normalise([]care.Permission{
	care.PermissionTasksView,
	care.PermissionMedicationsView,
	care.PermissionAppointmentsView,
	care.PermissionActivityView,
})

func senior(name, timezone string) Senior {
	return Senior{ID: uuid.New(), DisplayName: name, Timezone: timezone}
}

func membership(userID uuid.UUID, s Senior, permissions care.PermissionSet) UserMembership {
	return UserMembership{UserID: userID, Senior: s, Permissions: permissions}
}

func run(t *testing.T, in materialiseInput) []pending {
	t.Helper()

	found, err := materialise(context.Background(), in)
	if err != nil {
		t.Fatalf("materialise: %v", err)
	}
	return found
}

func typesOf(items []pending) []Type {
	found := make([]Type, 0, len(items))
	for _, item := range items {
		found = append(found, item.Type)
	}
	return found
}

func countOfType(items []pending, t Type) int {
	count := 0
	for _, item := range items {
		if item.Type == t {
			count++
		}
	}
	return count
}

func TestMedicationDoseProducesOneReminderFifteenMinutesBefore(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 1, 7, 0, 0, 0, kolkata)
	dose := uuid.New()
	amma := senior("Amma", "Asia/Kolkata")
	daughter := uuid.New()

	found := run(t, materialiseInput{
		Memberships: []UserMembership{membership(daughter, amma, everything)},
		Reminders: map[Type]ScheduleSource{
			TypeMedicationReminder: &fakeSchedule{due: []Due{
				{EntityID: dose, At: time.Date(2026, 3, 1, 7, 30, 0, 0, kolkata)},
			}},
		},
		Now: now,
	})

	if len(found) != 1 {
		t.Fatalf("got %d notifications (%v), want 1", len(found), typesOf(found))
	}

	want := time.Date(2026, 3, 1, 7, 15, 0, 0, kolkata)
	if !found[0].ScheduledFor.Equal(want) {
		t.Errorf("ScheduledFor = %s, want %s", found[0].ScheduledFor, want)
	}
	if found[0].RecipientUserID != daughter {
		t.Errorf("recipient = %s, want the daughter", found[0].RecipientUserID)
	}
	if found[0].EntityType != EntityMedicationDose || found[0].EntityID != dose {
		t.Errorf("points at %s/%s, want medication_dose/%s",
			found[0].EntityType, found[0].EntityID, dose)
	}
}

func TestOneMedicationWithTwoSchedulesProducesTwoReminders(t *testing.T) {
	t.Parallel()

	// Two doses of the same medicine an hour apart, both inside the window.
	now := time.Date(2026, 3, 1, 7, 0, 0, 0, kolkata)
	amma := senior("Amma", "Asia/Kolkata")
	daughter := uuid.New()

	found := run(t, materialiseInput{
		Memberships: []UserMembership{membership(daughter, amma, everything)},
		Reminders: map[Type]ScheduleSource{
			TypeMedicationReminder: &fakeSchedule{due: []Due{
				{EntityID: uuid.New(), At: time.Date(2026, 3, 1, 7, 30, 0, 0, kolkata)},
				{EntityID: uuid.New(), At: time.Date(2026, 3, 1, 7, 45, 0, 0, kolkata)},
			}},
		},
		Now: now,
	})

	if len(found) != 2 {
		t.Fatalf("got %d reminders, want one per dose", len(found))
	}
	if found[0].DedupeKey == found[1].DedupeKey {
		t.Error("two doses share a dedupe key; one would silently replace the other")
	}
}

func TestAppointmentRemindsADayAheadAndAnHourAhead(t *testing.T) {
	t.Parallel()

	appointment := uuid.New()
	amma := senior("Amma", "Asia/Kolkata")
	daughter := uuid.New()
	at := time.Date(2026, 3, 2, 10, 0, 0, 0, kolkata)

	source := &fakeSchedule{due: []Due{{EntityID: appointment, At: at}}}

	// The day before, the 24-hour reminder falls in the window.
	dayBefore := run(t, materialiseInput{
		Memberships: []UserMembership{membership(daughter, amma, everything)},
		Reminders:   map[Type]ScheduleSource{TypeAppointmentReminder: source},
		Now:         time.Date(2026, 3, 1, 9, 30, 0, 0, kolkata),
	})
	if len(dayBefore) != 1 {
		t.Fatalf("got %d notifications a day before, want the 24-hour reminder", len(dayBefore))
	}
	if want := at.Add(-24 * time.Hour); !dayBefore[0].ScheduledFor.Equal(want) {
		t.Errorf("24-hour reminder at %s, want %s", dayBefore[0].ScheduledFor, want)
	}

	// The domain must have been asked about tomorrow, or the reminder could not
	// have been found at all.
	if !source.to.After(at) {
		t.Errorf("queried up to %s, which does not reach the appointment at %s", source.to, at)
	}

	// On the day, the 1-hour reminder falls in the window and the 24-hour one is
	// long past.
	sameDay := run(t, materialiseInput{
		Memberships: []UserMembership{membership(daughter, amma, everything)},
		Reminders:   map[Type]ScheduleSource{TypeAppointmentReminder: source},
		Now:         time.Date(2026, 3, 2, 8, 30, 0, 0, kolkata),
	})
	if len(sameDay) != 1 {
		t.Fatalf("got %d notifications on the day, want the 1-hour reminder", len(sameDay))
	}
	if want := at.Add(-time.Hour); !sameDay[0].ScheduledFor.Equal(want) {
		t.Errorf("1-hour reminder at %s, want %s", sameDay[0].ScheduledFor, want)
	}

	// The two are different notifications, so neither suppresses the other.
	if dayBefore[0].DedupeKey == sameDay[0].DedupeKey {
		t.Error("the two appointment reminders share a dedupe key")
	}
}

func TestTheSamePassRunTwiceDecidesTheSameThing(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 1, 7, 0, 0, 0, kolkata)
	amma := senior("Amma", "Asia/Kolkata")
	daughter := uuid.New()

	input := materialiseInput{
		Memberships: []UserMembership{membership(daughter, amma, everything)},
		Reminders: map[Type]ScheduleSource{
			TypeMedicationReminder: &fakeSchedule{due: []Due{
				{EntityID: uuid.New(), At: time.Date(2026, 3, 1, 7, 30, 0, 0, kolkata)},
			}},
		},
		Now: now,
	}

	first := run(t, input)
	second := run(t, input)

	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("got %d then %d notifications, want 1 each", len(first), len(second))
	}
	// Identical keys are what make the insert a no-op the second time, which is
	// the whole of deduplication (plans/phase11.md §20).
	if first[0].DedupeKey != second[0].DedupeKey {
		t.Errorf("dedupe key changed between passes: %s then %s",
			first[0].DedupeKey, second[0].DedupeKey)
	}
}

func TestASilencedCategoryProducesNothing(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 1, 7, 0, 0, 0, kolkata)
	amma := senior("Amma", "Asia/Kolkata")
	daughter := uuid.New()

	preferences := DefaultPreferences(daughter)
	preferences.MedicationReminders = false

	found := run(t, materialiseInput{
		Memberships: []UserMembership{membership(daughter, amma, everything)},
		Preferences: map[uuid.UUID]Preferences{daughter: preferences},
		Reminders: map[Type]ScheduleSource{
			TypeMedicationReminder: &fakeSchedule{due: []Due{
				{EntityID: uuid.New(), At: time.Date(2026, 3, 1, 7, 30, 0, 0, kolkata)},
			}},
		},
		Now: now,
	})

	if len(found) != 0 {
		t.Errorf("got %d notifications for a silenced category, want none", len(found))
	}
}

func TestAMemberWithoutThePermissionIsNotNotified(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 1, 7, 0, 0, 0, kolkata)
	amma := senior("Amma", "Asia/Kolkata")

	// A neighbour who can see tasks but not medication.
	neighbour := uuid.New()
	tasksOnly := care.Normalise([]care.Permission{care.PermissionTasksView})

	found := run(t, materialiseInput{
		Memberships: []UserMembership{membership(neighbour, amma, tasksOnly)},
		Reminders: map[Type]ScheduleSource{
			TypeMedicationReminder: &fakeSchedule{due: []Due{
				{EntityID: uuid.New(), At: time.Date(2026, 3, 1, 7, 30, 0, 0, kolkata)},
			}},
		},
		Now: now,
	})

	if len(found) != 0 {
		t.Errorf("got %d notifications, want none — the reader cannot see medication", len(found))
	}
}

func TestAnAssignedTaskNotifiesOnlyTheAssignee(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 1, 8, 0, 0, 0, kolkata)
	amma := senior("Amma", "Asia/Kolkata")
	daughter, son := uuid.New(), uuid.New()

	found := run(t, materialiseInput{
		Memberships: []UserMembership{
			membership(daughter, amma, everything),
			membership(son, amma, everything),
		},
		Reminders: map[Type]ScheduleSource{
			TypeTaskReminder: &fakeSchedule{due: []Due{{
				EntityID:   uuid.New(),
				At:         time.Date(2026, 3, 1, 8, 30, 0, 0, kolkata),
				AssigneeID: &daughter,
			}}},
		},
		Now: now,
	})

	if len(found) != 1 {
		t.Fatalf("got %d notifications, want only the assignee's", len(found))
	}
	if found[0].RecipientUserID != daughter {
		t.Errorf("recipient = %s, want the assigned daughter", found[0].RecipientUserID)
	}
}

func TestAnUnassignedDoseRemindsEveryPermittedMember(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 1, 7, 0, 0, 0, kolkata)
	amma := senior("Amma", "Asia/Kolkata")
	daughter, son := uuid.New(), uuid.New()

	found := run(t, materialiseInput{
		Memberships: []UserMembership{
			membership(daughter, amma, everything),
			membership(son, amma, everything),
		},
		Reminders: map[Type]ScheduleSource{
			TypeMedicationReminder: &fakeSchedule{due: []Due{
				{EntityID: uuid.New(), At: time.Date(2026, 3, 1, 7, 30, 0, 0, kolkata)},
			}},
		},
		Now: now,
	})

	if len(found) != 2 {
		t.Fatalf("got %d notifications, want one for each member who can see medication", len(found))
	}
	if found[0].DedupeKey == found[1].DedupeKey {
		t.Error("two recipients share a dedupe key; only one would ever be told")
	}
}

func TestAProfessionalCaregiverIsNotifiedForEveryClient(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 1, 7, 0, 0, 0, kolkata)
	caregiver := uuid.New()
	clients := []Senior{
		senior("Amma", "Asia/Kolkata"),
		senior("Mr Rao", "Asia/Kolkata"),
		senior("Mrs Iyer", "Asia/Kolkata"),
	}

	memberships := make([]UserMembership, 0, len(clients))
	for _, client := range clients {
		memberships = append(memberships, membership(caregiver, client, everything))
	}

	found := run(t, materialiseInput{
		Memberships: memberships,
		Reminders: map[Type]ScheduleSource{
			TypeMedicationReminder: &fakeSchedule{due: []Due{
				{EntityID: uuid.New(), At: time.Date(2026, 3, 1, 7, 30, 0, 0, kolkata)},
			}},
		},
		Now: now,
	})

	if len(found) != len(clients) {
		t.Fatalf("got %d notifications for %d clients, want one each", len(found), len(clients))
	}
	seen := make(map[uuid.UUID]bool)
	for _, item := range found {
		seen[item.SeniorID] = true
	}
	if len(seen) != len(clients) {
		t.Errorf("notifications cover %d seniors, want %d", len(seen), len(clients))
	}
}

func TestASeniorLookingAfterThemselfIsNotified(t *testing.T) {
	t.Parallel()

	// No caregiver anywhere: one person, their own circle, their own medicine.
	now := time.Date(2026, 3, 1, 7, 0, 0, 0, kolkata)
	self := uuid.New()
	own := senior("Ravi", "Asia/Kolkata")

	found := run(t, materialiseInput{
		Memberships: []UserMembership{membership(self, own, everything)},
		Reminders: map[Type]ScheduleSource{
			TypeMedicationReminder: &fakeSchedule{due: []Due{
				{EntityID: uuid.New(), At: time.Date(2026, 3, 1, 7, 30, 0, 0, kolkata)},
			}},
		},
		Now: now,
	})

	if len(found) != 1 || found[0].RecipientUserID != self {
		t.Fatalf("got %d notifications, want one addressed to the senior themself", len(found))
	}
}

func TestAnOverdueTaskAlertsAfterTheGracePeriod(t *testing.T) {
	t.Parallel()

	amma := senior("Amma", "Asia/Kolkata")
	daughter := uuid.New()
	task := uuid.New()
	dueAt := time.Date(2026, 3, 1, 9, 0, 0, 0, kolkata)

	overdue := fakeOverdue{due: []Due{{EntityID: task, At: dueAt}}}

	// Ten minutes late: still inside the grace period, so nothing is said.
	early := run(t, materialiseInput{
		Memberships: []UserMembership{membership(daughter, amma, everything)},
		Overdue:     overdue,
		Now:         dueAt.Add(10 * time.Minute),
	})
	if len(early) != 0 {
		t.Errorf("got %d alerts inside the grace period, want none", len(early))
	}

	// Forty minutes late: past the grace period.
	late := run(t, materialiseInput{
		Memberships: []UserMembership{membership(daughter, amma, everything)},
		Overdue:     overdue,
		Now:         dueAt.Add(40 * time.Minute),
	})
	if len(late) != 1 {
		t.Fatalf("got %d alerts past the grace period, want 1", len(late))
	}
	if late[0].Type != TypeTaskOverdue {
		t.Errorf("type = %s, want TASK_OVERDUE", late[0].Type)
	}
	if want := dueAt.Add(overdueGrace); !late[0].ScheduledFor.Equal(want) {
		t.Errorf("alert at %s, want %s", late[0].ScheduledFor, want)
	}
}

func TestACompletedTaskNeverBecomesAnOverdueAlert(t *testing.T) {
	t.Parallel()

	// The domain decides what is outstanding. A completed occurrence is simply
	// absent from its answer, so nothing here has to know what "completed"
	// means (plans/phase11.md §17).
	amma := senior("Amma", "Asia/Kolkata")
	daughter := uuid.New()

	found := run(t, materialiseInput{
		Memberships: []UserMembership{membership(daughter, amma, everything)},
		Overdue:     fakeOverdue{due: nil},
		Now:         time.Date(2026, 3, 1, 9, 40, 0, 0, kolkata),
	})

	if len(found) != 0 {
		t.Errorf("got %d alerts, want none", len(found))
	}
}

func TestCareActivityNotifiesTheCircleButNotTheActor(t *testing.T) {
	t.Parallel()

	amma := senior("Amma", "Asia/Kolkata")
	daughter, son := uuid.New(), uuid.New()
	now := time.Date(2026, 3, 1, 9, 0, 0, 0, kolkata)

	found := run(t, materialiseInput{
		Memberships: []UserMembership{
			membership(daughter, amma, everything),
			membership(son, amma, everything),
		},
		Activity: fakeActivity{activities: []Activity{{
			EventID:     uuid.New(),
			SeniorID:    amma.ID,
			Kind:        ActivityMedicationRecorded,
			ActorUserID: &daughter,
			ActorName:   "Priya",
			OccurredAt:  now.Add(-time.Minute),
		}}},
		Now: now,
	})

	if len(found) != 1 {
		t.Fatalf("got %d activity notifications, want only the son's", len(found))
	}
	if found[0].RecipientUserID != son {
		t.Error("the person who recorded the dose was told about their own action")
	}
	if found[0].EntityType != EntityCareEvent {
		t.Errorf("entity = %s, want care_event", found[0].EntityType)
	}
	if !strings.Contains(found[0].Body, "Priya") {
		t.Errorf("body %q does not say who did it", found[0].Body)
	}
}

func TestCareActivityRespectsTheCategoryAndThePermission(t *testing.T) {
	t.Parallel()

	amma := senior("Amma", "Asia/Kolkata")
	quiet, blind := uuid.New(), uuid.New()
	now := time.Date(2026, 3, 1, 9, 0, 0, 0, kolkata)

	silenced := DefaultPreferences(quiet)
	silenced.CareActivity = false

	// The second member can see medication but not the activity timeline.
	noActivity := care.Normalise([]care.Permission{care.PermissionMedicationsView})

	found := run(t, materialiseInput{
		Memberships: []UserMembership{
			membership(quiet, amma, everything),
			membership(blind, amma, noActivity),
		},
		Preferences: map[uuid.UUID]Preferences{quiet: silenced},
		Activity: fakeActivity{activities: []Activity{{
			EventID:    uuid.New(),
			SeniorID:   amma.ID,
			Kind:       ActivityTaskCompleted,
			ActorName:  "Priya",
			OccurredAt: now.Add(-time.Minute),
		}}},
		Now: now,
	})

	if len(found) != 0 {
		t.Errorf("got %d notifications (%v), want none", len(found), typesOf(found))
	}
}

func TestAnActivityInAnotherSeniorsCircleIsNotDelivered(t *testing.T) {
	t.Parallel()

	amma := senior("Amma", "Asia/Kolkata")
	stranger := senior("Mr Rao", "Asia/Kolkata")
	daughter := uuid.New()
	now := time.Date(2026, 3, 1, 9, 0, 0, 0, kolkata)

	found := run(t, materialiseInput{
		Memberships: []UserMembership{membership(daughter, amma, everything)},
		Activity: fakeActivity{activities: []Activity{{
			EventID:    uuid.New(),
			SeniorID:   stranger.ID,
			Kind:       ActivityTaskCompleted,
			ActorName:  "Somebody",
			OccurredAt: now.Add(-time.Minute),
		}}},
		Now: now,
	})

	if len(found) != 0 {
		t.Errorf("got %d notifications about a senior the reader has no relationship with", len(found))
	}
}

func TestTheDoseTimeIsReadInTheSeniorsTimezone(t *testing.T) {
	t.Parallel()

	// A daughter in London, her mother in Kolkata. The mother's 08:00 dose is
	// 02:30 in London, and telling the daughter "02:30" would be useless
	// (plans/phase11.md §33).
	amma := senior("Amma", "Asia/Kolkata")
	daughter := uuid.New()
	doseAt := time.Date(2026, 3, 1, 8, 0, 0, 0, kolkata)

	found := run(t, materialiseInput{
		Memberships: []UserMembership{membership(daughter, amma, everything)},
		Reminders: map[Type]ScheduleSource{
			TypeMedicationReminder: &fakeSchedule{due: []Due{{EntityID: uuid.New(), At: doseAt}}},
		},
		Now: doseAt.Add(-30 * time.Minute),
	})

	if len(found) != 1 {
		t.Fatalf("got %d notifications, want 1", len(found))
	}
	if !strings.Contains(found[0].Body, "08:00") {
		t.Errorf("body %q does not read the dose in the senior's clock", found[0].Body)
	}
	if strings.Contains(found[0].Body, "02:30") {
		t.Errorf("body %q reads the dose in the reader's clock", found[0].Body)
	}
	_ = london
}

func TestADayIsNotAssumedToBeTwentyFourHours(t *testing.T) {
	t.Parallel()

	// London's clocks go forward at 01:00 on 29 March 2026, so that day is 23
	// hours long by the wall clock. An appointment at 10:00 BST that morning is
	// 09:00 UTC; a lead of 24 hours lands at 09:00 UTC the previous day, which
	// is 09:00 GMT locally — one hour earlier on the clock than the appointment,
	// not the same time.
	//
	// That is the correct answer, and it is correct because nothing here does
	// calendar arithmetic. Recurrence — "every day at 08:00", which must stay
	// 08:00 across the boundary — belongs to internal/recurrence and the domains
	// that use it. This layer only ever subtracts an absolute duration from an
	// instant those domains already produced, so it can neither gain nor lose an
	// hour of its own (plans/phase11.md §34).
	dad := senior("Dad", "Europe/London")
	son := uuid.New()
	appointmentAt := time.Date(2026, 3, 29, 10, 0, 0, 0, london)

	source := &fakeSchedule{due: []Due{{EntityID: uuid.New(), At: appointmentAt}}}

	found := run(t, materialiseInput{
		Memberships: []UserMembership{membership(son, dad, everything)},
		Reminders:   map[Type]ScheduleSource{TypeAppointmentReminder: source},
		Now:         appointmentAt.Add(-24*time.Hour - 15*time.Minute),
	})

	if len(found) != 1 {
		t.Fatalf("got %d notifications, want the 24-hour reminder", len(found))
	}
	// Exactly a day of elapsed time, whatever the clocks did in between.
	if elapsed := appointmentAt.Sub(found[0].ScheduledFor); elapsed != 24*time.Hour {
		t.Errorf("reminder is %s before the appointment, want exactly 24h", elapsed)
	}
	if hour := found[0].ScheduledFor.In(london).Hour(); hour != 9 {
		t.Errorf("reminder fires at %02d:00 local, want 09:00 across the spring-forward boundary", hour)
	}
	// And the sentence still names the appointment's own wall-clock time.
	if !strings.Contains(found[0].Body, "10:00") {
		t.Errorf("body %q does not name the appointment's own wall-clock time", found[0].Body)
	}
}

func TestNothingIsMaterialisedBeyondTheHorizon(t *testing.T) {
	t.Parallel()

	// A dose three hours out is real, and will be materialised by a later pass.
	// Writing it now would be a decision taken with a three-hour-old view of the
	// care (plans/phase11.md §71).
	amma := senior("Amma", "Asia/Kolkata")
	daughter := uuid.New()
	now := time.Date(2026, 3, 1, 7, 0, 0, 0, kolkata)

	found := run(t, materialiseInput{
		Memberships: []UserMembership{membership(daughter, amma, everything)},
		Reminders: map[Type]ScheduleSource{
			TypeMedicationReminder: &fakeSchedule{due: []Due{
				{EntityID: uuid.New(), At: now.Add(3 * time.Hour)},
			}},
		},
		Now: now,
	})

	if countOfType(found, TypeMedicationReminder) != 0 {
		t.Errorf("got %d reminders beyond the horizon, want none", len(found))
	}
}
