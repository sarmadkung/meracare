package appointments_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/genxcare/api/internal/appointments"
	"github.com/genxcare/api/internal/auth"
	"github.com/genxcare/api/internal/care"
	"github.com/genxcare/api/internal/careevents"
	"github.com/genxcare/api/internal/relationships"
	"github.com/genxcare/api/internal/seniors"
	"github.com/genxcare/api/internal/testsupport"
	"github.com/genxcare/api/internal/users"
)

type fixture struct {
	appointments  *appointments.Service
	repo          *appointments.Repository
	seniors       *seniors.Service
	relationships *relationships.Repository
	users         *users.Repository
}

func newFixture(t *testing.T) fixture {
	t.Helper()

	pool := testsupport.RequireDatabase(t)
	relationshipRepo := relationships.NewRepository(pool)
	seniorRepo := seniors.NewRepository(pool)
	appointmentRepo := appointments.NewRepository(pool)
	events := careevents.NewRepository(pool)
	recorder := careevents.NewRecorder(pool, events)

	return fixture{
		appointments:  appointments.NewService(appointmentRepo, seniorRepo, relationshipRepo, recorder),
		repo:          appointmentRepo,
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

// newCircle creates a senior in a named timezone, so the day boundary is tested
// in a zone that is not the machine's.
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

// book creates an appointment at an instant, failing the test if it will not
// save.
func (f fixture) book(
	t *testing.T,
	owner auth.Principal,
	seniorID uuid.UUID,
	title string,
	at time.Time,
) appointments.Appointment {
	t.Helper()

	appointment, err := f.appointments.Create(context.Background(), appointments.CreateInput{
		SeniorID:        seniorID,
		CreatedByUserID: owner.UserID,
		Title:           title,
		ScheduledAt:     at,
	})
	if err != nil {
		t.Fatalf("book %q: %v", title, err)
	}
	return appointment
}

func mustLoad(t *testing.T, name string) *time.Location {
	t.Helper()
	location, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return location
}

func titles(found []appointments.Appointment) []string {
	names := make([]string, 0, len(found))
	for _, appointment := range found {
		names = append(names, appointment.Title)
	}
	return names
}

// --- Booking ----------------------------------------------------------------

func TestBookingAnAppointmentRecordsWhoBookedIt(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	ends := time.Now().Add(25 * time.Hour)
	appointment, err := f.appointments.Create(ctx, appointments.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Title:           "Cardiology review",
		Kind:            appointments.KindDoctorVisit,
		ProviderName:    "Dr Ahmed",
		Location:        "City Hospital",
		Notes:           "Bring the last blood test.",
		ScheduledAt:     time.Now().Add(24 * time.Hour),
		EndsAt:          &ends,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if appointment.CreatedByUserID != owner.UserID {
		t.Errorf("createdBy = %v, want the authenticated caller %v",
			appointment.CreatedByUserID, owner.UserID)
	}
	if appointment.Status != appointments.StatusScheduled {
		t.Errorf("status = %q, want scheduled", appointment.Status)
	}
	if appointment.ProviderName != "Dr Ahmed" || appointment.Location != "City Hospital" {
		t.Errorf("provider/location not stored: %+v", appointment)
	}
	if appointment.EndsAt == nil {
		t.Error("end time was not stored")
	}
	if appointment.AssignedUserID != nil {
		t.Error("a newly booked appointment was assigned to somebody")
	}
}

// An appointment may be assigned to a circle member, and to nobody else.
// Assigning it outside the circle would name somebody who cannot see it.
func TestAnAppointmentCanOnlyBeAssignedInsideTheCircle(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	owner := f.newUser(t, "owner@example.com")
	caregiver := f.newUser(t, "caregiver@example.com")
	stranger := f.newUser(t, "stranger@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	if _, err := f.relationships.Create(ctx, relationships.CreateParams{
		SeniorID:    circle.Senior.ID,
		UserID:      caregiver.UserID,
		Role:        care.RoleProfessionalCaregiver,
		Permissions: care.Normalise(care.DefaultPermissions(care.RoleProfessionalCaregiver)),
		Status:      care.StatusActive,
	}); err != nil {
		t.Fatalf("add caregiver: %v", err)
	}

	assigned, err := f.appointments.Create(ctx, appointments.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Title:           "Physiotherapy",
		AssignedUserID:  &caregiver.UserID,
		ScheduledAt:     time.Now().Add(48 * time.Hour),
	})
	if err != nil {
		t.Fatalf("assign to a member: %v", err)
	}
	if assigned.AssignedUserID == nil || *assigned.AssignedUserID != caregiver.UserID {
		t.Errorf("assignee = %v, want %v", assigned.AssignedUserID, caregiver.UserID)
	}

	_, err = f.appointments.Create(ctx, appointments.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Title:           "Physiotherapy",
		AssignedUserID:  &stranger.UserID,
		ScheduledAt:     time.Now().Add(48 * time.Hour),
	})
	if !errors.Is(err, appointments.ErrInvalidAssignee) {
		t.Errorf("err = %v, want ErrInvalidAssignee", err)
	}
}

// A member who has been removed from the circle is no longer a valid assignee,
// even though the relationship row survives to keep their history attributed.
func TestARevokedMemberCannotBeAssignedAnAppointment(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	owner := f.newUser(t, "owner@example.com")
	former := f.newUser(t, "former@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	relationship, err := f.relationships.Create(ctx, relationships.CreateParams{
		SeniorID:    circle.Senior.ID,
		UserID:      former.UserID,
		Role:        care.RoleProfessionalCaregiver,
		Permissions: care.Normalise(care.DefaultPermissions(care.RoleProfessionalCaregiver)),
		Status:      care.StatusActive,
	})
	if err != nil {
		t.Fatalf("add member: %v", err)
	}
	if _, err := f.relationships.RevokeMembership(ctx, relationship.ID); err != nil {
		t.Fatalf("revoke member: %v", err)
	}

	_, err = f.appointments.Create(ctx, appointments.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Title:           "Blood test",
		AssignedUserID:  &former.UserID,
		ScheduledAt:     time.Now().Add(24 * time.Hour),
	})
	if !errors.Is(err, appointments.ErrInvalidAssignee) {
		t.Errorf("err = %v, want ErrInvalidAssignee", err)
	}
}

// --- Reading ----------------------------------------------------------------

func TestUpcomingAppointmentsComeBackSoonestFirst(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	f.book(t, owner, circle.Senior.ID, "Next week", now.Add(7*24*time.Hour))
	f.book(t, owner, circle.Senior.ID, "Tomorrow", now.Add(24*time.Hour))
	f.book(t, owner, circle.Senior.ID, "Last week", now.Add(-7*24*time.Hour))

	found, err := f.appointments.List(ctx, circle.Senior.ID, appointments.ScopeUpcoming, now)
	if err != nil {
		t.Fatalf("list upcoming: %v", err)
	}

	want := []string{"Tomorrow", "Next week"}
	if got := titles(found); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("upcoming = %v, want %v", got, want)
	}
}

// A cancelled appointment stays in the upcoming list. One that vanished would
// look like an appointment nobody had told you about (plans/phase6.md §31).
func TestACancelledAppointmentStaysVisible(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")
	appointment := f.book(t, owner, circle.Senior.ID, "Dentist", now.Add(24*time.Hour))

	if _, err := f.appointments.Act(ctx, appointments.ActInput{
		AppointmentID: appointment.ID,
		Action:        appointments.ActionCancel,
		ActorID:       owner.UserID,
	}); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	found, err := f.appointments.List(ctx, circle.Senior.ID, appointments.ScopeUpcoming, now)
	if err != nil {
		t.Fatalf("list upcoming: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("upcoming = %v, want the cancelled appointment to still appear", titles(found))
	}
	if found[0].Status != appointments.StatusCancelled {
		t.Errorf("status = %q, want cancelled", found[0].Status)
	}
}

// "Today" is the senior's own day. A daughter in London reading her mother's
// calendar at 21:00 her time is looking at the small hours of the next day in
// Karachi, and the answer must be her mother's day, not hers.
func TestTodayIsTheSeniorsOwnDay(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	karachi := mustLoad(t, "Asia/Karachi")
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	local := now.In(karachi)
	year, month, day := local.Date()
	startOfDay := time.Date(year, month, day, 0, 0, 0, 0, karachi)

	// Ten past midnight and ten to midnight, where the senior lives. Both are
	// inside her day; the appointment ten minutes after it are not.
	f.book(t, owner, circle.Senior.ID, "Just after midnight", startOfDay.Add(10*time.Minute))
	f.book(t, owner, circle.Senior.ID, "Just before midnight", startOfDay.AddDate(0, 0, 1).Add(-10*time.Minute))
	f.book(t, owner, circle.Senior.ID, "Tomorrow", startOfDay.AddDate(0, 0, 1).Add(10*time.Minute))
	f.book(t, owner, circle.Senior.ID, "Yesterday", startOfDay.Add(-10*time.Minute))

	found, err := f.appointments.List(ctx, circle.Senior.ID, appointments.ScopeToday, now)
	if err != nil {
		t.Fatalf("list today: %v", err)
	}

	want := []string{"Just after midnight", "Just before midnight"}
	if got := titles(found); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("today = %v, want %v", got, want)
	}
}

func TestHistoryReturnsWhatHasAlreadyHappenedNewestFirst(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	f.book(t, owner, circle.Senior.ID, "Three weeks ago", now.Add(-21*24*time.Hour))
	f.book(t, owner, circle.Senior.ID, "Last week", now.Add(-7*24*time.Hour))
	f.book(t, owner, circle.Senior.ID, "Tomorrow", now.Add(24*time.Hour))

	page, err := f.appointments.History(ctx, circle.Senior.ID, "", 0, now)
	if err != nil {
		t.Fatalf("history: %v", err)
	}

	want := []string{"Last week", "Three weeks ago"}
	if got := titles(page.Items); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("history = %v, want %v", got, want)
	}
	if page.NextCursor != "" {
		t.Errorf("nextCursor = %q, want it empty when the history is exhausted", page.NextCursor)
	}
}

// The keyset cursor must walk the whole history without repeating or skipping,
// including where two appointments share an instant and only the id separates
// them.
func TestHistoryPagesThroughWithoutRepeatingOrSkipping(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	// Two of these share a scheduled instant on purpose.
	sameInstant := now.Add(-3 * 24 * time.Hour)
	f.book(t, owner, circle.Senior.ID, "A", now.Add(-5*24*time.Hour))
	f.book(t, owner, circle.Senior.ID, "B", sameInstant)
	f.book(t, owner, circle.Senior.ID, "C", sameInstant)
	f.book(t, owner, circle.Senior.ID, "D", now.Add(-1*24*time.Hour))
	f.book(t, owner, circle.Senior.ID, "E", now.Add(-2*time.Hour))

	seen := map[string]int{}
	cursor := ""
	for pages := 0; ; pages++ {
		if pages > 10 {
			t.Fatal("history never reached its end")
		}

		page, err := f.appointments.History(ctx, circle.Senior.ID, cursor, 2, now)
		if err != nil {
			t.Fatalf("history page %d: %v", pages, err)
		}
		for _, appointment := range page.Items {
			seen[appointment.Title]++
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}

	for _, title := range []string{"A", "B", "C", "D", "E"} {
		if seen[title] != 1 {
			t.Errorf("%q appeared %d times across the pages, want exactly once", title, seen[title])
		}
	}
}

func TestAnUnknownViewIsRefused(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	_, err := f.appointments.List(ctx, circle.Senior.ID, appointments.Scope("everything"), time.Now())
	if !errors.Is(err, appointments.ErrBadScope) {
		t.Errorf("err = %v, want ErrBadScope", err)
	}
}

// --- Editing ----------------------------------------------------------------

func TestEditingChangesOnlyWhatWasSent(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	original, err := f.appointments.Create(ctx, appointments.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Title:           "Cardiology review",
		Kind:            appointments.KindDoctorVisit,
		ProviderName:    "Dr Ahmed",
		Location:        "City Hospital",
		Notes:           "Bring the last blood test.",
		ScheduledAt:     time.Now().Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	moved := original.ScheduledAt.Add(2 * time.Hour)
	updated, err := f.appointments.Update(ctx, appointments.UpdateInput{
		AppointmentID: original.ID,
		SeniorID:      circle.Senior.ID,
		ScheduledAt:   &moved,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	if !updated.ScheduledAt.Equal(moved) {
		t.Errorf("scheduledAt = %v, want %v", updated.ScheduledAt, moved)
	}
	if updated.Title != original.Title ||
		updated.ProviderName != original.ProviderName ||
		updated.Location != original.Location ||
		updated.Notes != original.Notes ||
		updated.Kind != original.Kind {
		t.Errorf("an edit that only moved the time changed something else:\n got %+v\nwant %+v",
			updated, original)
	}
	if updated.CreatedByUserID != original.CreatedByUserID {
		t.Error("editing rewrote who booked the appointment")
	}
}

// Once an appointment is settled it is the record of what happened, and the
// record is not rewritten (plans/phase6.md §8).
func TestASettledAppointmentCannotBeEdited(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	for name, action := range map[string]appointments.Action{
		"completed": appointments.ActionComplete,
		"cancelled": appointments.ActionCancel,
	} {
		t.Run(name, func(t *testing.T) {
			appointment := f.book(t, owner, circle.Senior.ID, "Dentist", time.Now().Add(24*time.Hour))

			if _, err := f.appointments.Act(ctx, appointments.ActInput{
				AppointmentID: appointment.ID,
				Action:        action,
				ActorID:       owner.UserID,
			}); err != nil {
				t.Fatalf("%s: %v", action, err)
			}

			title := "Rewritten"
			_, err := f.appointments.Update(ctx, appointments.UpdateInput{
				AppointmentID: appointment.ID,
				SeniorID:      circle.Senior.ID,
				Title:         &title,
			})
			if !errors.Is(err, appointments.ErrSettled) {
				t.Fatalf("err = %v, want ErrSettled", err)
			}

			// And the stored row is untouched.
			stored, err := f.appointments.Get(ctx, appointment.ID)
			if err != nil {
				t.Fatalf("reload: %v", err)
			}
			if stored.Title != "Dentist" {
				t.Errorf("title = %q, want the original %q", stored.Title, "Dentist")
			}
		})
	}
}

func TestEditingAnAppointmentThatDoesNotExistIsNotFound(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	title := "Anything"
	_, err := f.appointments.Update(ctx, appointments.UpdateInput{
		AppointmentID: uuid.New(),
		SeniorID:      circle.Senior.ID,
		Title:         &title,
	})
	if !errors.Is(err, appointments.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestClearingRemovesAValueThatAbsenceCannotExpress(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	ends := time.Now().Add(25 * time.Hour)
	original, err := f.appointments.Create(ctx, appointments.CreateInput{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Title:           "Physiotherapy",
		Kind:            appointments.KindTherapy,
		AssignedUserID:  &owner.UserID,
		ScheduledAt:     time.Now().Add(24 * time.Hour),
		EndsAt:          &ends,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	updated, err := f.appointments.Update(ctx, appointments.UpdateInput{
		AppointmentID: original.ID,
		SeniorID:      circle.Senior.ID,
		ClearKind:     true,
		ClearEndsAt:   true,
		ClearAssignee: true,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	if updated.Kind != "" {
		t.Errorf("kind = %q, want it cleared", updated.Kind)
	}
	if updated.EndsAt != nil {
		t.Errorf("endsAt = %v, want it cleared", updated.EndsAt)
	}
	if updated.AssignedUserID != nil {
		t.Errorf("assignee = %v, want it cleared", updated.AssignedUserID)
	}
}

// --- Settling ---------------------------------------------------------------

func TestCompletingRecordsTheActorAndTheTime(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")
	appointment := f.book(t, owner, circle.Senior.ID, "Blood test", time.Now().Add(-2*time.Hour))

	result, err := f.appointments.Act(ctx, appointments.ActInput{
		AppointmentID: appointment.ID,
		Action:        appointments.ActionComplete,
		ActorID:       owner.UserID,
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	if result.Repeat {
		t.Error("a first completion was reported as a repeat")
	}
	if result.Appointment.Status != appointments.StatusCompleted {
		t.Errorf("status = %q, want completed", result.Appointment.Status)
	}
	if result.Appointment.CompletedBy == nil || *result.Appointment.CompletedBy != owner.UserID {
		t.Errorf("completedBy = %v, want %v", result.Appointment.CompletedBy, owner.UserID)
	}
	if result.Appointment.CompletedAt == nil {
		t.Error("completedAt was not recorded")
	}
	if result.Appointment.CancelledAt != nil || result.Appointment.CancelledBy != nil {
		t.Error("completing an appointment recorded a cancellation as well")
	}
}

// A retried request succeeds and leaves the original attribution alone, which
// is what makes cancel and complete safe to send twice (plans/phase6.md §24).
func TestSettlingTwiceIsASuccessThatChangesNothing(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	owner := f.newUser(t, "owner@example.com")
	other := f.newUser(t, "other@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	if _, err := f.relationships.Create(ctx, relationships.CreateParams{
		SeniorID:    circle.Senior.ID,
		UserID:      other.UserID,
		Role:        care.RoleFamilyMember,
		Permissions: care.Normalise(care.DefaultPermissions(care.RoleFamilyMember)),
		Status:      care.StatusActive,
	}); err != nil {
		t.Fatalf("add member: %v", err)
	}

	appointment := f.book(t, owner, circle.Senior.ID, "Dentist", time.Now().Add(24*time.Hour))

	first, err := f.appointments.Act(ctx, appointments.ActInput{
		AppointmentID: appointment.ID,
		Action:        appointments.ActionCancel,
		ActorID:       owner.UserID,
	})
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}

	// Somebody else's phone replays the same cancellation.
	second, err := f.appointments.Act(ctx, appointments.ActInput{
		AppointmentID: appointment.ID,
		Action:        appointments.ActionCancel,
		ActorID:       other.UserID,
	})
	if err != nil {
		t.Fatalf("cancel again: %v", err)
	}

	if !second.Repeat {
		t.Error("the second cancellation was not reported as a repeat")
	}
	if second.Appointment.CancelledBy == nil || *second.Appointment.CancelledBy != owner.UserID {
		t.Errorf("cancelledBy = %v, want the first actor %v",
			second.Appointment.CancelledBy, owner.UserID)
	}
	if !second.Appointment.CancelledAt.Equal(*first.Appointment.CancelledAt) {
		t.Error("the second cancellation moved the timestamp")
	}
}

// The conflict the specification asks about: one member cancels while another
// completes. Whichever landed first stands, and the second is told.
func TestContradictoryOutcomesAreRefused(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	for name, tc := range map[string]struct{ first, second appointments.Action }{
		"complete then cancel": {appointments.ActionComplete, appointments.ActionCancel},
		"cancel then complete": {appointments.ActionCancel, appointments.ActionComplete},
	} {
		t.Run(name, func(t *testing.T) {
			appointment := f.book(t, owner, circle.Senior.ID, "Dentist", time.Now().Add(24*time.Hour))

			settled, err := f.appointments.Act(ctx, appointments.ActInput{
				AppointmentID: appointment.ID,
				Action:        tc.first,
				ActorID:       owner.UserID,
			})
			if err != nil {
				t.Fatalf("%s: %v", tc.first, err)
			}

			_, err = f.appointments.Act(ctx, appointments.ActInput{
				AppointmentID: appointment.ID,
				Action:        tc.second,
				ActorID:       owner.UserID,
			})
			if !errors.Is(err, appointments.ErrInvalidTransition) {
				t.Fatalf("err = %v, want ErrInvalidTransition", err)
			}

			stored, err := f.appointments.Get(ctx, appointment.ID)
			if err != nil {
				t.Fatalf("reload: %v", err)
			}
			if stored.Status != settled.Appointment.Status {
				t.Errorf("status = %q, want the first outcome %q",
					stored.Status, settled.Appointment.Status)
			}
		})
	}
}

// Cancelling preserves the record rather than deleting it, and the row stays
// readable in the history (plans/phase6.md §§6, 9).
func TestACancelledAppointmentRemainsInTheHistory(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")
	appointment := f.book(t, owner, circle.Senior.ID, "Cardiology review", now.Add(-24*time.Hour))

	if _, err := f.appointments.Act(ctx, appointments.ActInput{
		AppointmentID: appointment.ID,
		Action:        appointments.ActionCancel,
		ActorID:       owner.UserID,
	}); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	page, err := f.appointments.History(ctx, circle.Senior.ID, "", 0, now)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("history = %v, want the cancelled appointment", titles(page.Items))
	}
	if page.Items[0].Status != appointments.StatusCancelled {
		t.Errorf("status = %q, want cancelled", page.Items[0].Status)
	}
	if page.Items[0].Title != "Cardiology review" {
		t.Errorf("title = %q, want it preserved", page.Items[0].Title)
	}
}

// --- The database's own guarantees -------------------------------------------

// The CHECK constraints are the last line of defence: the handler validates
// these too, but a bug there must not be able to write an impossible row.
func TestTheDatabaseRefusesImpossibleAppointments(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	owner := f.newUser(t, "owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan", "Asia/Karachi")

	start := time.Now().Add(24 * time.Hour)
	before := start.Add(-time.Hour)

	if _, err := f.repo.Create(ctx, appointments.CreateParams{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Title:           "Ends before it starts",
		ScheduledAt:     start,
		EndsAt:          &before,
	}); err == nil {
		t.Error("an appointment that ends before it starts was accepted")
	}

	if _, err := f.repo.Create(ctx, appointments.CreateParams{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Title:           "   ",
		ScheduledAt:     start,
	}); err == nil {
		t.Error("an appointment with a blank title was accepted")
	}

	if _, err := f.repo.Create(ctx, appointments.CreateParams{
		SeniorID:        circle.Senior.ID,
		CreatedByUserID: owner.UserID,
		Title:           "Unrecognised kind",
		Kind:            appointments.Kind("chemotherapy"),
		ScheduledAt:     start,
	}); err == nil {
		t.Error("an appointment with an unrecognised kind was accepted")
	}
}
