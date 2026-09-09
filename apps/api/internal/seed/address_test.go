package seed_test

import (
	"strings"
	"testing"

	"github.com/meracare/api/internal/seed"
)

/*
Where the personas' addresses come from.

Supabase refuses to create an account at a domain that accepts no mail, which
rules out example.com and every domain the project does not actually own. One
real mailbox has to stand in for four people, so the plan can be addressed at a
mailbox as well as at a domain.
*/

func emailOf(t *testing.T, plan seed.Plan, personaKey string) string {
	t.Helper()

	for _, persona := range plan.Personas {
		if persona.Key == personaKey {
			return persona.Email
		}
	}

	t.Fatalf("no persona %q in the plan", personaKey)
	return ""
}

func TestADomainGivesEveryPersonaTheirOwnAddress(t *testing.T) {
	plan := seed.NewPlan("example.com")

	if got := emailOf(t, plan, "solo"); got != "zahra@example.com" {
		t.Errorf("addressed at a domain, the solo senior is %q, want zahra@example.com", got)
	}
}

// The whole point: one inbox, four accounts. Supabase sees four addresses and
// the person reading the confirmation mail sees one mailbox.
func TestAMailboxPlusAddressesEveryPersona(t *testing.T) {
	plan := seed.NewPlan("someone@gmail.com")

	if got := emailOf(t, plan, "solo"); got != "someone+zahra@gmail.com" {
		t.Errorf("addressed at a mailbox, the solo senior is %q, want someone+zahra@gmail.com", got)
	}
	if got := emailOf(t, plan, "nurse"); got != "someone+fatima@gmail.com" {
		t.Errorf("the nurse is %q, want someone+fatima@gmail.com", got)
	}
}

// An invitation is an address somebody will be asked to sign up with, so it has
// to be as deliverable as the accounts are.
func TestInvitationsShareTheMailbox(t *testing.T) {
	plan := seed.NewPlan("someone@gmail.com")

	for _, circle := range plan.Circles {
		for _, invitation := range circle.Invitations {
			if !strings.HasPrefix(invitation.Email, "someone+") {
				t.Errorf("invitation to %q in %s is not at the mailbox", invitation.Email, circle.Key)
			}
		}
	}
}

// A mailbox with no local part is a domain written oddly, not an address, and
// "+zahra@gmail.com" would be neither.
func TestAnEmptyLocalPartIsTreatedAsADomain(t *testing.T) {
	plan := seed.NewPlan("@gmail.com")

	if got := emailOf(t, plan, "solo"); got != "zahra@gmail.com" {
		t.Errorf("the solo senior is %q, want zahra@gmail.com", got)
	}
}

/*
Signing in as yourself.

The personas are fiction, but the account behind one of them does not have to
be: a real address on a persona is how the person building the app opens it as
somebody with a week of care already behind them.
*/

func TestSignInAsPutsARealAccountBehindAPersona(t *testing.T) {
	plan, err := seed.NewPlan("example.com").SignInAs("son", "someone@gmail.com", "")
	if err != nil {
		t.Fatalf("SignInAs: %v", err)
	}

	if got := emailOf(t, plan, "son"); got != "someone@gmail.com" {
		t.Errorf("the son is %q, want someone@gmail.com", got)
	}
	if got := emailOf(t, plan, "nurse"); got != "fatima@example.com" {
		t.Errorf("the nurse is %q, want to be left alone", got)
	}
}

// The plan is handed out by value and read repeatedly; replacing an address
// must not reach back into the plan it was derived from.
func TestSignInAsLeavesTheOriginalAlone(t *testing.T) {
	original := seed.NewPlan("example.com")

	if _, err := original.SignInAs("son", "someone@gmail.com", "Someone Real"); err != nil {
		t.Fatalf("SignInAs: %v", err)
	}

	if got := emailOf(t, original, "son"); got != "bilal@example.com" {
		t.Errorf("the original plan's son became %q", got)
	}
	if got := circleNamed(t, original, "bilal").Name; got != "Bilal Ahmed" {
		t.Errorf("the original plan's circle became %q", got)
	}
}

func TestSignInAsRefusesAPersonaNobodyCanBe(t *testing.T) {
	if _, err := seed.NewPlan("example.com").SignInAs("nobody", "someone@gmail.com", ""); err == nil {
		t.Error("a persona that is not in the plan was accepted")
	}

	// Naveed has no account by design: he is an author in the history, not a
	// person who signs in.
	if _, err := seed.NewPlan("example.com").SignInAs("former", "someone@gmail.com", ""); err == nil {
		t.Error("a persona who never signs in was accepted")
	}
}

// Two personas at one address would be two memberships on one account, and the
// circles they are each in would silently merge.
func TestSignInAsRefusesAnAddressAlreadyInUse(t *testing.T) {
	if _, err := seed.NewPlan("example.com").SignInAs("son", "fatima@example.com", ""); err == nil {
		t.Error("an address another persona already holds was accepted")
	}
}

/*
Being yourself, by name.

Taking a persona's place and keeping their name is a screen that greets you as
somebody else. The name belongs to the person, so it moves with the account:
the persona, the circle that *is* them, and the next-of-kin line that names them
in somebody else's record.
*/

func nameOf(t *testing.T, plan seed.Plan, personaKey string) string {
	t.Helper()

	for _, persona := range plan.Personas {
		if persona.Key == personaKey {
			return persona.Name
		}
	}

	t.Fatalf("no persona %q in the plan", personaKey)
	return ""
}

func circleNamed(t *testing.T, plan seed.Plan, circleKey string) seed.Circle {
	t.Helper()

	for _, circle := range plan.Circles {
		if circle.Key == circleKey {
			return circle
		}
	}

	t.Fatalf("no circle %q in the plan", circleKey)
	return seed.Circle{}
}

func TestSignInAsTakesYourName(t *testing.T) {
	plan, err := seed.NewPlan("example.com").SignInAs("son", "someone@gmail.com", "Someone Real")
	if err != nil {
		t.Fatalf("SignInAs: %v", err)
	}

	if got := nameOf(t, plan, "son"); got != "Someone Real" {
		t.Errorf("the son is called %q, want Someone Real", got)
	}
}

// The son holds his own care as well as his parents'. That circle is him, so
// leaving it named Bilal Ahmed would put a stranger in his own person strip.
func TestSignInAsRenamesTheCircleThatIsYou(t *testing.T) {
	plan, err := seed.NewPlan("example.com").SignInAs("son", "someone@gmail.com", "Someone Real")
	if err != nil {
		t.Fatalf("SignInAs: %v", err)
	}

	if got := circleNamed(t, plan, "bilal").Name; got != "Someone Real" {
		t.Errorf("his own circle is called %q, want Someone Real", got)
	}
	if got := circleNamed(t, plan, "yusuf").Name; got != "Yusuf Khan" {
		t.Errorf("somebody else's circle became %q", got)
	}
}

// He is Yusuf's next of kin. An emergency contact naming somebody the app no
// longer contains reads as a bug, because it is one.
func TestSignInAsFollowsYouIntoEmergencyContacts(t *testing.T) {
	plan, err := seed.NewPlan("example.com").SignInAs("son", "someone@gmail.com", "Someone Real")
	if err != nil {
		t.Fatalf("SignInAs: %v", err)
	}

	if got := circleNamed(t, plan, "yusuf").Emergency; !strings.HasPrefix(got, "Someone Real ") {
		t.Errorf("Yusuf's emergency contact is %q, want it to name Someone Real", got)
	}
	if got := circleNamed(t, plan, "amina").Emergency; !strings.HasPrefix(got, "Sana Ahmed ") {
		t.Errorf("Amina's emergency contact became %q", got)
	}
}

// An empty name is how you take an account without taking a new identity.
func TestSignInAsKeepsThePersonaNameWhenGivenNone(t *testing.T) {
	plan, err := seed.NewPlan("example.com").SignInAs("son", "someone@gmail.com", "")
	if err != nil {
		t.Fatalf("SignInAs: %v", err)
	}

	if got := nameOf(t, plan, "son"); got != "Bilal Ahmed" {
		t.Errorf("the son is called %q, want Bilal Ahmed", got)
	}
}
