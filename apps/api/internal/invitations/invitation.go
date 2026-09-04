package invitations

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/genxcare/api/internal/care"
)

// Status is the stored lifecycle state of an invitation.
type Status string

const (
	StatusPending  Status = "pending"
	StatusAccepted Status = "accepted"
	StatusRevoked  Status = "revoked"
	StatusExpired  Status = "expired"
)

// DefaultLifetime is how long a new invitation stays valid.
//
// Long enough that a recipient can act at their convenience, short enough that
// a forgotten invitation in an old inbox does not remain a way in.
const DefaultLifetime = 7 * 24 * time.Hour

// Invitation is a proposal to join a senior's care circle.
type Invitation struct {
	ID            uuid.UUID
	SeniorID      uuid.UUID
	InviterUserID uuid.UUID
	InviteeEmail  string
	Role          care.Role
	Permissions   care.PermissionSet
	Status        Status
	ExpiresAt     time.Time

	AcceptedAt       *time.Time
	AcceptedByUserID uuid.NullUUID
	RevokedAt        *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

// EffectiveStatus is the invitation's real state at time now.
//
// Expiry is computed rather than read, so an invitation is dead the moment it
// lapses. Relying on a stored flag would leave a window in which a background
// sweep had not yet run and the token still worked — the spec requires the API
// itself to enforce expiry.
func (i Invitation) EffectiveStatus(now time.Time) Status {
	if i.Status == StatusPending && !now.Before(i.ExpiresAt) {
		return StatusExpired
	}
	return i.Status
}

// IsAcceptable reports whether the invitation can still be redeemed.
func (i Invitation) IsAcceptable(now time.Time) bool {
	return i.EffectiveStatus(now) == StatusPending
}

// MatchesRecipient reports whether email is the address this invitation was
// sent to.
//
// Comparison is case-insensitive: addresses are not case-sensitive in practice,
// and a recipient should not be locked out by how their provider capitalises
// them.
func (i Invitation) MatchesRecipient(email string) bool {
	return NormaliseEmail(email) != "" && NormaliseEmail(email) == NormaliseEmail(i.InviteeEmail)
}

// NormaliseEmail lowercases and trims an address for comparison and storage.
func NormaliseEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// Response is the JSON representation of an invitation for members of the
// circle managing it.
//
// The token is absent: it is returned exactly once, by the endpoint that
// creates the invitation, and never retrievable afterwards.
type Response struct {
	ID           string   `json:"id"`
	SeniorID     string   `json:"seniorId"`
	InviteeEmail string   `json:"inviteeEmail"`
	Role         string   `json:"role"`
	Permissions  []string `json:"permissions"`
	Status       string   `json:"status"`
	ExpiresAt    string   `json:"expiresAt"`
	CreatedAt    string   `json:"createdAt"`
}

// ToResponse renders an invitation, reporting its effective status.
func ToResponse(invitation Invitation, now time.Time) Response {
	return Response{
		ID:           invitation.ID.String(),
		SeniorID:     invitation.SeniorID.String(),
		InviteeEmail: invitation.InviteeEmail,
		Role:         string(invitation.Role),
		Permissions:  invitation.Permissions.Strings(),
		Status:       string(invitation.EffectiveStatus(now)),
		ExpiresAt:    invitation.ExpiresAt.UTC().Format(time.RFC3339),
		CreatedAt:    invitation.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// PreviewResponse is what the holder of a token sees before accepting.
//
// It is deliberately thin. Anyone holding the token can read it, including
// someone it was not sent to, so it carries only what a recipient needs to
// decide: who is asking, on whose behalf, and what they would gain. No senior
// contact details, no care data, no member list.
type PreviewResponse struct {
	SeniorName   string   `json:"seniorName"`
	InviterName  string   `json:"inviterName"`
	InviteeEmail string   `json:"inviteeEmail"`
	Role         string   `json:"role"`
	Permissions  []string `json:"permissions"`
	Status       string   `json:"status"`
	ExpiresAt    string   `json:"expiresAt"`
}
