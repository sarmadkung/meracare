package invitations

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/genxcare/api/internal/auth"
	"github.com/genxcare/api/internal/care"
	"github.com/genxcare/api/internal/careevents"
	"github.com/genxcare/api/internal/relationships"
)

// Domain failures the transport layer maps onto responses.
var (
	// ErrRoleNotInvitable is returned for a role that cannot be handed out.
	ErrRoleNotInvitable = errors.New("that role cannot be invited")
	// ErrPermissionEscalation is returned when the inviter tried to grant
	// permissions they do not themselves hold.
	ErrPermissionEscalation = errors.New("cannot grant permissions you do not hold")
	// ErrAlreadyMember is returned when the invitee is already in the circle.
	ErrAlreadyMember = errors.New("that person is already in this care circle")
	// ErrNotAcceptable is returned for an invitation that is expired, revoked,
	// or already used.
	ErrNotAcceptable = errors.New("this invitation can no longer be used")
	// ErrWrongRecipient is returned when the signed-in user is not who the
	// invitation was sent to.
	ErrWrongRecipient = errors.New("this invitation was sent to a different address")
	// ErrCannotModifySenior is returned for an attempt to change or remove the
	// senior's own membership.
	ErrCannotModifySenior = errors.New("the senior's own membership cannot be changed")
)

// EscalationError carries which permissions were refused, so the client can say
// precisely what it could not grant.
type EscalationError struct {
	Refused care.PermissionSet
}

func (e *EscalationError) Error() string {
	return fmt.Sprintf("cannot grant permissions you do not hold: %v", e.Refused.Strings())
}

func (e *EscalationError) Unwrap() error { return ErrPermissionEscalation }

// MemberLookup resolves an existing membership. Satisfied by
// relationships.Repository.
type MemberLookup interface {
	FindByUserAndSenior(ctx context.Context, userID, seniorID uuid.UUID) (relationships.Relationship, error)
}

// UserLookup resolves users by ID and by email. Satisfied by users.Repository.
type UserLookup interface {
	GetByID(ctx context.Context, id uuid.UUID) (UserSummary, error)
	FindIDByEmail(ctx context.Context, email string) (uuid.UUID, error)
}

// UserSummary is the little the invitation flow needs to know about a person.
type UserSummary struct {
	ID          uuid.UUID
	DisplayName string
	Email       string
}

// SeniorLookup resolves a senior's display name for the invitation preview.
type SeniorLookup interface {
	DisplayName(ctx context.Context, seniorID uuid.UUID) (string, error)
}

// Service implements the invitation lifecycle.
type Service struct {
	invitations   *Repository
	relationships *relationships.Repository
	members       MemberLookup
	users         UserLookup
	seniors       SeniorLookup
	// events records what happened, in the same transaction as the change
	// itself (plans/phase7.md §26).
	events *careevents.Recorder
	// now is injectable so expiry can be tested without waiting.
	now func() time.Time
}

// NewService builds the service.
func NewService(
	invitationRepo *Repository,
	relationshipRepo *relationships.Repository,
	users UserLookup,
	seniors SeniorLookup,
	events *careevents.Recorder,
) *Service {
	return &Service{
		invitations:   invitationRepo,
		relationships: relationshipRepo,
		members:       relationshipRepo,
		users:         users,
		seniors:       seniors,
		events:        events,
		now:           time.Now,
	}
}

// WithClock replaces the service clock. For tests.
func (s *Service) WithClock(now func() time.Time) *Service {
	clone := *s
	clone.now = now
	return &clone
}

// CreateInput is a request to invite somebody.
type CreateInput struct {
	SeniorID uuid.UUID
	Email    string
	Role     care.Role
	// Permissions is optional. When empty, the role's defaults are used,
	// narrowed to what the inviter may delegate.
	Permissions []care.Permission
	Lifetime    time.Duration
}

// Created is a new invitation plus the one-time token to deliver.
type Created struct {
	Invitation Invitation
	// Token is returned exactly once. It is not stored and cannot be recovered.
	Token Token
}

// Create issues an invitation.
//
// The inviter's own relationship is the authority for what may be granted:
// `members.invite` is checked by middleware before this runs, and every
// requested permission is checked against the inviter's set here. Without that
// second check, the ability to invite would be a path to every other
// permission.
func (s *Service) Create(
	ctx context.Context,
	inviter relationships.Relationship,
	input CreateInput,
) (Created, error) {
	if !care.CanInviteRole(input.Role) {
		return Created{}, ErrRoleNotInvitable
	}

	email := NormaliseEmail(input.Email)

	// Someone already in the circle is managed as a member, not re-invited.
	if err := s.assertNotAlreadyMember(ctx, input.SeniorID, email); err != nil {
		return Created{}, err
	}

	// Asking for specific permissions and asking for "the usual" are different
	// requests. An explicit list that exceeds the inviter is an escalation
	// attempt and is refused outright; unspecified defaults are simply narrowed
	// to what the inviter can actually confer.
	var granted care.PermissionSet
	if len(input.Permissions) == 0 {
		granted = care.DelegateDefaults(inviter.Permissions, input.Role)
	} else {
		delegation := care.Delegate(inviter.Permissions, input.Role, input.Permissions)
		if !delegation.OK() {
			return Created{}, &EscalationError{Refused: delegation.Refused}
		}
		granted = delegation.Granted
	}

	token, err := NewToken()
	if err != nil {
		return Created{}, err
	}

	lifetime := input.Lifetime
	if lifetime <= 0 {
		lifetime = DefaultLifetime
	}

	var invitation Invitation
	err = s.events.InTransaction(ctx, func(tx pgx.Tx, events *careevents.Repository) error {
		created, err := s.invitations.WithTx(tx).Create(ctx, CreateParams{
			SeniorID:      input.SeniorID,
			InviterUserID: inviter.UserID,
			InviteeEmail:  email,
			Role:          input.Role,
			Permissions:   granted,
			TokenHash:     token.Hash(),
			ExpiresAt:     s.now().Add(lifetime),
		})
		if err != nil {
			return err
		}
		invitation = created

		// The invitee's email, not a name: they may have no account yet, and
		// there is nothing else to call them. It is the same address the
		// inviter typed, so it reveals nothing to the circle that a member did
		// not already put there (plans/phase7.md §10).
		_, err = events.Record(ctx, careevents.RecordParams{
			SeniorID:    created.SeniorID,
			ActorUserID: &inviter.UserID,
			Type:        careevents.TypeMemberInvited,
			EntityType:  careevents.EntityInvitation,
			EntityID:    created.ID,
			Metadata: careevents.Metadata{
				careevents.MetaMemberName: created.InviteeEmail,
				careevents.MetaRole:       string(created.Role),
			},
		})
		return err
	})
	if err != nil {
		return Created{}, err
	}

	return Created{Invitation: invitation, Token: token}, nil
}

// assertNotAlreadyMember refuses to invite somebody who is already active in
// the circle. A previously revoked member may be invited back.
func (s *Service) assertNotAlreadyMember(ctx context.Context, seniorID uuid.UUID, email string) error {
	userID, err := s.users.FindIDByEmail(ctx, email)
	if err != nil {
		// No account with that address yet, which is the ordinary case for a
		// new invitee.
		return nil //nolint:nilerr // absence of a user is not a failure here
	}

	existing, err := s.members.FindByUserAndSenior(ctx, userID, seniorID)
	if errors.Is(err, relationships.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if existing.IsActive() {
		return ErrAlreadyMember
	}
	return nil
}

// List returns a circle's invitations.
func (s *Service) List(ctx context.Context, seniorID uuid.UUID) ([]Invitation, error) {
	return s.invitations.ListForSenior(ctx, seniorID)
}

// Preview describes an invitation to the holder of its token.
//
// Anyone holding the token can read this, so it carries only what a recipient
// needs in order to decide.
func (s *Service) Preview(ctx context.Context, token Token) (PreviewResponse, error) {
	invitation, err := s.findByToken(ctx, token)
	if err != nil {
		return PreviewResponse{}, err
	}

	seniorName, err := s.seniors.DisplayName(ctx, invitation.SeniorID)
	if err != nil {
		return PreviewResponse{}, err
	}
	inviter, err := s.users.GetByID(ctx, invitation.InviterUserID)
	if err != nil {
		return PreviewResponse{}, err
	}

	return PreviewResponse{
		SeniorName:   seniorName,
		InviterName:  inviter.DisplayName,
		InviteeEmail: invitation.InviteeEmail,
		Role:         string(invitation.Role),
		Permissions:  invitation.Permissions.Strings(),
		Status:       string(invitation.EffectiveStatus(s.now())),
		ExpiresAt:    invitation.ExpiresAt.UTC().Format(time.RFC3339),
	}, nil
}

// Accept redeems an invitation and creates the care relationship.
//
// The two happen in one transaction, and the invitation is consumed by a
// conditional UPDATE that requires it to still be pending. Two concurrent
// accepts therefore produce exactly one membership: the second finds no row to
// update and fails.
func (s *Service) Accept(
	ctx context.Context,
	principal auth.Principal,
	token Token,
) (relationships.Relationship, error) {
	invitation, err := s.findByToken(ctx, token)
	if err != nil {
		return relationships.Relationship{}, err
	}

	if !invitation.IsAcceptable(s.now()) {
		return relationships.Relationship{}, ErrNotAcceptable
	}

	// An invitation is addressed to a person, not merely to whoever holds the
	// link. Someone who obtains a token for another address cannot redeem it.
	if !invitation.MatchesRecipient(principal.Email) {
		return relationships.Relationship{}, ErrWrongRecipient
	}

	// One transaction covering all three writes: consuming the invitation,
	// creating the membership, and recording that somebody joined. The first
	// two were already atomic in Phase 3; the event joins them rather than
	// opening a second transaction that could commit on its own
	// (plans/phase7.md §26).
	var relationship relationships.Relationship
	err = s.events.InTransaction(ctx, func(tx pgx.Tx, events *careevents.Repository) error {
		// Consuming the invitation first means a lost race fails before any
		// membership is written.
		if _, err := s.invitations.MarkAcceptedTx(ctx, tx, invitation.ID, principal.UserID); err != nil {
			if errors.Is(err, ErrNotFound) {
				return ErrNotAcceptable
			}
			return err
		}

		joined, err := s.relationships.UpsertActiveTx(ctx, tx, relationships.CreateParams{
			SeniorID:    invitation.SeniorID,
			UserID:      principal.UserID,
			Role:        invitation.Role,
			Permissions: invitation.Permissions,
			Status:      care.StatusActive,
		})
		if err != nil {
			return err
		}
		relationship = joined

		// The actor is the person joining, taken from their session — the one
		// thing about this flow a client could never be trusted to assert
		// (plans/phase7.md §4).
		_, err = events.Record(ctx, careevents.RecordParams{
			SeniorID:    joined.SeniorID,
			ActorUserID: &principal.UserID,
			Type:        careevents.TypeMemberJoined,
			EntityType:  careevents.EntityRelationship,
			EntityID:    joined.ID,
			Metadata: careevents.Metadata{
				careevents.MetaMemberName: principal.Email,
				careevents.MetaRole:       string(joined.Role),
			},
		})
		return err
	})
	if err != nil {
		return relationships.Relationship{}, err
	}
	return relationship, nil
}

// Revoke withdraws a pending invitation. The caller's authority over the
// invitation's senior is checked by the handler before this runs.
func (s *Service) Revoke(ctx context.Context, invitationID uuid.UUID) (Invitation, error) {
	invitation, err := s.invitations.Revoke(ctx, invitationID)
	if errors.Is(err, ErrNotFound) {
		// Either it does not exist or it is no longer pending. Both mean there
		// is nothing to revoke.
		return Invitation{}, ErrNotAcceptable
	}
	return invitation, err
}

// GetByID loads an invitation, for handlers that must authorize against its
// senior before acting.
func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (Invitation, error) {
	return s.invitations.GetByID(ctx, id)
}

// findByToken validates the token's shape before looking it up, so malformed
// input never reaches the database.
// findByToken resolves a code as it was supplied.
//
// Normalising here rather than in the handlers means every route that redeems a
// code — preview and accept alike — accepts the same forms, and a malformed one
// is turned away before it becomes a query.
func (s *Service) findByToken(ctx context.Context, supplied Token) (Invitation, error) {
	token, ok := NormaliseToken(string(supplied))
	if !ok {
		return Invitation{}, ErrNotFound
	}
	return s.invitations.FindByTokenHash(ctx, token.Hash())
}

// Now exposes the service clock to handlers rendering effective status.
func (s *Service) Now() time.Time { return s.now() }
