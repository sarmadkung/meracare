package invitations_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/meracare/api/internal/care"
	"github.com/meracare/api/internal/invitations"
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

		// 12 symbols of Crockford base32 — 60 bits, and short enough to read
		// aloud or type from a message.
		if len(token) != 12 {
			t.Fatalf("token is %d characters, want 12: %q", len(token), token)
		}
		for _, symbol := range token {
			if !strings.ContainsRune(invitations.TokenAlphabet, symbol) {
				t.Fatalf("token contains %q, which is outside the alphabet: %q", symbol, token)
			}
		}
	}
}

// Crockford's alphabet omits I, L, O and U so a code can be read aloud or
// copied by hand without the 0/O and 1/I/L confusions.
func TestNewTokenExcludesAmbiguousLetters(t *testing.T) {
	for range 500 {
		token, err := invitations.NewToken()
		if err != nil {
			t.Fatalf("NewToken: %v", err)
		}
		if strings.ContainsAny(string(token), "ILOU") {
			t.Fatalf("token contains an ambiguous letter: %q", token)
		}
	}
}

func TestNormaliseTokenAcceptsWhatPeopleActuallyType(t *testing.T) {
	// Every form below is the same code, as it might arrive from a share
	// message, a phone call, or a careless paste.
	for name, raw := range map[string]string{
		"canonical":        "K7M29XQP4WTZ",
		"grouped":          "K7M2-9XQP-4WTZ",
		"lowercase":        "k7m29xqp4wtz",
		"lowercase spaced": "k7m2 9xqp 4wtz",
		"surrounding tabs": "\tK7M29XQP4WTZ\n",
	} {
		t.Run(name, func(t *testing.T) {
			got, ok := invitations.NormaliseToken(raw)
			if !ok {
				t.Fatalf("NormaliseToken(%q) rejected a valid code", raw)
			}
			if got != invitations.Token("K7M29XQP4WTZ") {
				t.Errorf("NormaliseToken(%q) = %q, want %q", raw, got, "K7M29XQP4WTZ")
			}
		})
	}
}

// Crockford decoding folds the letters it excluded onto the digits they are
// mistaken for, so someone who reads a 0 as an O still gets in.
func TestNormaliseTokenFoldsMistakenLetters(t *testing.T) {
	got, ok := invitations.NormaliseToken("OIL29XQP4WTZ")
	if !ok {
		t.Fatal("NormaliseToken rejected a code using mistakable letters")
	}
	if want := invitations.Token("01129XQP4WTZ"); got != want {
		t.Errorf("NormaliseToken = %q, want %q", got, want)
	}
}

func TestNormaliseTokenRejectsMalformed(t *testing.T) {
	for name, raw := range map[string]string{
		"empty":           "",
		"blank":           "   ",
		"too short":       "K7M29XQP4WT",
		"too long":        "K7M29XQP4WTZ9",
		"invalid symbol":  "K7M29XQP4WT!",
		"excluded letter": "K7M29XQP4WTU",
		"base64 token":    "9Xq_pQ7mZa-bcdefghijklmnopqrstuvwxyzABCDEFG",
	} {
		t.Run(name, func(t *testing.T) {
			if got, ok := invitations.NormaliseToken(raw); ok {
				t.Errorf("NormaliseToken(%q) = %q, want rejection", raw, got)
			}
		})
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

	// Two random 60-bit values sharing even 8 leading symbols is a 2^-40 event,
	// and a sequential or time-derived scheme would fail this immediately.
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
		"empty":            "",
		"not the alphabet": "not a token!",
		"too short":        valid[:11],
		"too long":         valid + "9",
		"uuid":             invitations.Token(uuid.NewString()),
		// Valid() judges the canonical form only; anything else must go
		// through NormaliseToken first.
		"lowercase": invitations.Token(strings.ToLower(string(valid))),
		"grouped":   valid[:4] + "-" + valid[4:8] + "-" + valid[8:],
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
