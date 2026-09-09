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
