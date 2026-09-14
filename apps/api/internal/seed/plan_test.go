package seed_test

import (
	"slices"
	"testing"
	"time"

	"github.com/genxcare/api/internal/care"
	"github.com/genxcare/api/internal/careevents"
	"github.com/genxcare/api/internal/seed"
)

/*
The plan is the point of the seeder: not "some rows", but four people who use
the app differently, so that a screen can be looked at as each of them.
*/

func plan(t *testing.T) seed.Plan {
	t.Helper()
	p := seed.NewPlan("example.com")
	if err := p.Validate(); err != nil {
		t.Fatalf("the plan is not internally consistent: %v", err)
	}
	return p
}

func TestEveryPersonaCanBeSignedInAs(t *testing.T) {
	seen := map[string]bool{}

	for _, persona := range plan(t).Personas {
		if persona.Email == "" && persona.SignsIn {
			t.Errorf("%s signs in but has no address to sign in with", persona.Key)
		}
		if seen[persona.Email] && persona.Email != "" {
			t.Errorf("two personas share the address %q", persona.Email)
		}
		seen[persona.Email] = true
	}
}

// The four ways of using GenXcare that look different on screen. Losing one of
// them silently is how a layout ships that only ever worked for a daughter.
func TestTheFourShapesOfUseAreAllPresent(t *testing.T) {
	p := plan(t)

	cases := []struct {
		persona string
		circles int
		why     string
	}{
		{"solo", 1, "a senior managing their own care sees one circle and no filter strip"},
		{"daughter", 1, "one parent is the common case, and the strip stays hidden"},
		{"son", 3, "two parents and himself, so the strip appears and has to sort"},
		{"nurse", 4, "a professional round, which is what makes worst-first ordering matter"},
	}

	for _, want := range cases {
		got := len(p.CirclesFor(want.persona))
		if got != want.circles {
			t.Errorf("%s is in %d circles, want %d — %s", want.persona, got, want.circles, want.why)
		}
	}
}

// Two zones reading the same on the clock is the case the time gutter exists
// for, and it cannot be tested without a circle in each.
func TestSomebodyKeepsCircleInMoreThanOneTimezone(t *testing.T) {
	zones := map[string]bool{}
	for _, circle := range plan(t).CirclesFor("son") {
		zones[circle.Timezone] = true
	}

	if len(zones) < 2 {
		t.Errorf("every circle is in the same zone (%v); nothing exercises the gutter", zones)
	}
}

// A professional is a full member who still must not edit the profile or
// manage the circle. If the seeder grants them everything, no screen ever
// shows what a restricted member sees.
func TestTheProfessionalIsGenuinelyRestricted(t *testing.T) {
	p := plan(t)

	member, ok := p.Membership("nurse", "amina")
	if !ok {
		t.Fatal("the professional is not in the shared circle")
	}

	permissions := care.DefaultPermissions(member.Role)
	if slices.Contains(permissions, care.PermissionSeniorEdit) {
		t.Error("the professional can edit the profile")
	}
	if !slices.Contains(permissions, care.PermissionMedicationsRecord) {
		t.Error("the professional cannot record a dose, which is their whole job")
	}
}

// Every circle state the members screen can render needs to exist somewhere,
// or those branches are only ever seen in a unit test.
func TestTheAwkwardStatesExistSomewhere(t *testing.T) {
	p := plan(t)

	var revoked, pending, archived bool
	for _, circle := range p.Circles {
		archived = archived || circle.Archived
		pending = pending || len(circle.Invitations) > 0
		for _, member := range circle.Members {
			revoked = revoked || member.Status == "revoked"
		}
	}

	if !revoked {
		t.Error("no circle has a revoked member")
	}
	if !pending {
		t.Error("no circle has an invitation waiting to be accepted")
	}
	if !archived {
		t.Error("no senior is archived")
	}
}

/*
The day has to have a shape. "Looks full" is not a quantity of rows — it is a
morning that already happened, something that went wrong, one thing to do now,
and an evening still ahead.
*/
func TestEveryDayHasAPastAPresentAndAFuture(t *testing.T) {
	now := time.Date(2026, 9, 8, 9, 40, 0, 0, time.UTC)

	data, err := seed.Build(plan(t), now)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	for _, circle := range plan(t).Circles {
		var settled, slipped, ahead int

		for _, dose := range data.Doses {
			if dose.CircleKey != circle.Key || !sameDay(dose.ScheduledFor, now) {
				continue
			}
			switch {
			case dose.Status != "pending":
				settled++
			case dose.ScheduledFor.Before(now.Add(-time.Hour)):
				slipped++
			default:
				ahead++
			}
		}

		if settled == 0 {
			t.Errorf("%s: nothing has happened yet today", circle.Key)
		}
		if slipped == 0 {
			t.Errorf("%s: nothing has been missed, so no attention state is ever shown", circle.Key)
		}
		if ahead == 0 {
			t.Errorf("%s: nothing is still to come, so there is no current row", circle.Key)
		}
	}
}

func sameDay(a, b time.Time) bool {
	return a.UTC().Format("2006-01-02") == b.UTC().Format("2006-01-02")
}

/*
The seeder writes fixtures, not history it has no right to invent.

TASK_MISSED and MEDICATION_MISSED are read off the clock rather than performed
by anybody, which is why nothing in the care domain ever writes one. The seeder
uses that vocabulary for a notification — an escalation the app really does
send — and must never let it reach the activity feed, where it would be a
record of something nobody did.

internal/careevents has a guard that keeps those names out of the care domains
by scanning the source. It exempts this package for the reason above, and this
is the test that keeps the exemption honest.
*/
func TestTheSeederNeverFabricatesAMissedCareEvent(t *testing.T) {
	data, err := seed.Build(seed.NewPlan("example.com"), time.Now())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	for _, event := range data.Events {
		for _, unemitted := range careevents.NotYetEmitted {
			if event.Type == string(unemitted) {
				t.Errorf("a seeded activity entry claims %q, which nobody performs", event.Type)
			}
		}
	}
}
