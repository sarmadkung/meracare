package invitations_test

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/genxcare/api/internal/care"
	"github.com/genxcare/api/internal/invitations"
)

func TestNewTokenIsRandomAndWellFormed(t *testing.T) {
	const samples = 500

	seen := make(map[invitations.Token]struct{}, samples)
	for range samples {
		token, err := invitations.NewToken()
		if err != nil {
			t.Fatalf("NewToken: %v", err)
		}
		if _, duplicate := seen[token]; duplicate {
			t.Fatalf("NewToken produced a duplicate: %q", token)
		}
		seen[token] = struct{}{}

		if !token.Valid() {
			t.Fatalf("NewToken produced an invalid token: %q", token)
		}

		// 32 bytes of entropy, URL-safe and unpadded.
		decoded, err := base64.RawURLEncoding.DecodeString(string(token))
		if err != nil {
			t.Fatalf("token is not base64url: %v", err)
		}
		if len(decoded) != 32 {
			t.Fatalf("token carries %d bytes of entropy, want 32", len(decoded))
		}
	}
}

// The token must not be derived from anything guessable.
func TestTokensDoNotShareAPrefix(t *testing.T) {
	first, err := invitations.NewToken()
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}
	second, err := invitations.NewToken()
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}

	// Two random 256-bit values sharing even 8 leading characters would be an
	// extraordinary coincidence, and a sequential or time-derived scheme would
	// fail this immediately.
	if first[:8] == second[:8] {
		t.Errorf("tokens share a prefix: %q and %q", first, second)
	}
}

func TestTokenHashIsStableAndDistinct(t *testing.T) {
	token := invitations.Token("a-token")

	if got, want := token.Hash(), token.Hash(); !invitations.EqualHash(got, want) {
		t.Error("Hash is not stable for the same token")
	}
	if len(token.Hash()) != 32 {
		t.Errorf("hash is %d bytes, want 32 (SHA-256)", len(token.Hash()))
	}
	if invitations.EqualHash(token.Hash(), invitations.Token("another-token").Hash()) {
		t.Error("different tokens produced the same hash")
	}
}

// The stored hash must not be reversible to the token.
func TestTokenHashIsNotThePlainToken(t *testing.T) {
	token, err := invitations.NewToken()
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}

	if string(token.Hash()) == string(token) {
		t.Fatal("the stored hash is the raw token")
	}
}

func TestTokenValid(t *testing.T) {
	valid, err := invitations.NewToken()
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}
	if !valid.Valid() {
		t.Error("a freshly minted token should be valid")
	}

	for name, token := range map[string]invitations.Token{
		"empty":       "",
		"not base64":  "not a token!!",
		"too short":   invitations.Token(base64.RawURLEncoding.EncodeToString([]byte("short"))),
		"too long":    invitations.Token(base64.RawURLEncoding.EncodeToString(make([]byte, 64))),
		"uuid":        invitations.Token(uuid.NewString()),
		"padded form": valid + "==",
	} {
		if token.Valid() {
			t.Errorf("%s: %q should be invalid", name, token)
		}
	}
}

func TestEqualHash(t *testing.T) {
	a := invitations.Token("one").Hash()
	b := invitations.Token("one").Hash()
	c := invitations.Token("two").Hash()

	if !invitations.EqualHash(a, b) {
		t.Error("identical hashes should compare equal")
	}
	if invitations.EqualHash(a, c) {
		t.Error("different hashes should not compare equal")
	}
	if invitations.EqualHash(a, nil) {
		t.Error("a hash should not equal nil")
	}
}

func TestEffectiveStatusComputesExpiry(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)

	cases := map[string]struct {
		invitation invitations.Invitation
		want       invitations.Status
	}{
		"pending and fresh": {
			invitation: invitations.Invitation{
				Status:    invitations.StatusPending,
				ExpiresAt: now.Add(time.Hour),
			},
			want: invitations.StatusPending,
		},
		// Expiry is computed, so a lapsed invitation is dead immediately —
		// no background sweep has to have run.
		"pending but lapsed": {
			invitation: invitations.Invitation{
				Status:    invitations.StatusPending,
				ExpiresAt: now.Add(-time.Second),
			},
			want: invitations.StatusExpired,
		},
		"lapses exactly now": {
			invitation: invitations.Invitation{
				Status:    invitations.StatusPending,
				ExpiresAt: now,
			},
			want: invitations.StatusExpired,
		},
		// A used or withdrawn invitation keeps its status regardless of time.
		"accepted": {
			invitation: invitations.Invitation{
				Status:    invitations.StatusAccepted,
				ExpiresAt: now.Add(-time.Hour),
			},
			want: invitations.StatusAccepted,
		},
		"revoked": {
			invitation: invitations.Invitation{
				Status:    invitations.StatusRevoked,
				ExpiresAt: now.Add(time.Hour),
			},
			want: invitations.StatusRevoked,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := tc.invitation.EffectiveStatus(now); got != tc.want {
				t.Errorf("EffectiveStatus = %q, want %q", got, tc.want)
			}
			wantAcceptable := tc.want == invitations.StatusPending
			if got := tc.invitation.IsAcceptable(now); got != wantAcceptable {
				t.Errorf("IsAcceptable = %v, want %v", got, wantAcceptable)
			}
		})
	}
}

func TestMatchesRecipient(t *testing.T) {
	invitation := invitations.Invitation{InviteeEmail: "sara@example.com"}

	for _, match := range []string{"sara@example.com", "SARA@Example.com", "  sara@example.com  "} {
		if !invitation.MatchesRecipient(match) {
			t.Errorf("%q should match the recipient", match)
		}
	}
	for _, mismatch := range []string{"", "   ", "other@example.com", "sara@example.co"} {
		if invitation.MatchesRecipient(mismatch) {
			t.Errorf("%q should not match the recipient", mismatch)
		}
	}
}

// A user with no email on record must not match an invitation.
func TestMatchesRecipientRejectsBlankOnBothSides(t *testing.T) {
	invitation := invitations.Invitation{InviteeEmail: ""}

	if invitation.MatchesRecipient("") {
		t.Error("a blank address must not match a blank invitee")
	}
}

func TestToResponseOmitsTheToken(t *testing.T) {
	now := time.Now()
	response := invitations.ToResponse(invitations.Invitation{
		ID:           uuid.New(),
		SeniorID:     uuid.New(),
		InviteeEmail: "sara@example.com",
		Role:         care.RoleFamilyMember,
		Permissions:  care.PermissionSet{care.PermissionSeniorView},
		Status:       invitations.StatusPending,
		ExpiresAt:    now.Add(time.Hour),
		CreatedAt:    now,
	}, now)

	if response.Status != string(invitations.StatusPending) {
		t.Errorf("Status = %q", response.Status)
	}
	// The response type has no token field at all, which is the point: there is
	// no code path that can return one after creation.
	if response.InviteeEmail != "sara@example.com" {
		t.Errorf("InviteeEmail = %q", response.InviteeEmail)
	}
}
