// Package members manages an existing care circle: who is in it, what they may
// do, and removing them.
//
// Joining is the invitation flow's job (internal/invitations); this package
// owns everything after that point.
package members

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/genxcare/api/internal/care"
	"github.com/genxcare/api/internal/careevents"
	"github.com/genxcare/api/internal/relationships"
)

// Domain failures the transport layer maps onto responses.
var (
	// ErrNotFound is returned when the membership does not exist, or belongs to
	// a different senior than the one in the URL.
	ErrNotFound = errors.New("member not found")
	// ErrCannotModifySenior is returned for an attempt to change or remove the
	// membership of the person the circle exists for.
	ErrCannotModifySenior = errors.New("the senior's own membership cannot be changed")
	// ErrPermissionEscalation is returned when the editor tried to grant
	// permissions they do not hold.
	ErrPermissionEscalation = errors.New("cannot grant permissions you do not hold")
)

// EscalationError carries the refused permissions.
type EscalationError struct {
	Refused care.PermissionSet
}

func (e *EscalationError) Error() string {
	return fmt.Sprintf("cannot grant permissions you do not hold: %v", e.Refused.Strings())
}

func (e *EscalationError) Unwrap() error { return ErrPermissionEscalation }

// Service manages care-circle membership.
type Service struct {
	relationships *relationships.Repository
	// events records what happened, in the same transaction as the change
	// itself (plans/phase7.md §26).
	events *careevents.Recorder
}

// NewService builds the service.
func NewService(relationshipRepo *relationships.Repository, events *careevents.Recorder) *Service {
	return &Service{relationships: relationshipRepo, events: events}
}

// List returns the senior's active care circle.
func (s *Service) List(ctx context.Context, seniorID uuid.UUID) ([]relationships.Member, error) {
	return s.relationships.ListMembers(ctx, seniorID)
}

// UpdatePermissions changes what a member may do.
//
// Two rules apply, in this order:
//
//  1. The senior's own membership is untouchable. Their circle exists for them,
//     and letting a member strip the senior's access would be the sharpest
//     escalation available.
//  2. The editor may only grant permissions they themselves hold, exactly as
//     when inviting. Otherwise `members.manage` would be a route to everything.
func (s *Service) UpdatePermissions(
	ctx context.Context,
	editor relationships.Relationship,
	seniorID uuid.UUID,
	relationshipID uuid.UUID,
	requested []care.Permission,
) (relationships.Relationship, error) {
	target, err := s.load(ctx, seniorID, relationshipID)
	if err != nil {
		return relationships.Relationship{}, err
	}

	if target.Role == care.RoleSenior {
		return relationships.Relationship{}, ErrCannotModifySenior
	}

	delegation := care.Delegate(editor.Permissions, target.Role, requested)
	if !delegation.OK() {
		return relationships.Relationship{}, &EscalationError{Refused: delegation.Refused}
	}

	return s.relationships.UpdatePermissions(ctx, target.ID, delegation.Granted)
}

// Revoke removes a member from the circle.
//
// The relationship row survives with status `revoked`, so anything they
// authored keeps its author, while every authorization check — which requires
// an active relationship — starts failing immediately.
func (s *Service) Revoke(
	ctx context.Context,
	seniorID uuid.UUID,
	relationshipID uuid.UUID,
	actorID uuid.UUID,
) (relationships.Relationship, error) {
	target, err := s.load(ctx, seniorID, relationshipID)
	if err != nil {
		return relationships.Relationship{}, err
	}

	if target.Role == care.RoleSenior {
		return relationships.Relationship{}, ErrCannotModifySenior
	}

	// The name is read before the revocation so the event can say who was
	// removed. Reading it afterwards would work too, but this way the event
	// carries the name they had while they were a member, which is what the
	// timeline entry is about.
	name := s.memberName(ctx, seniorID, target.UserID)

	var revoked relationships.Relationship
	err = s.events.InTransaction(ctx, func(tx pgx.Tx, events *careevents.Repository) error {
		updated, err := s.relationships.WithTx(tx).RevokeMembership(ctx, target.ID)
		if err != nil {
			return err
		}
		revoked = updated

		_, err = events.Record(ctx, careevents.RecordParams{
			SeniorID:    seniorID,
			ActorUserID: &actorID,
			Type:        careevents.TypeMemberRevoked,
			EntityType:  careevents.EntityRelationship,
			EntityID:    updated.ID,
			Metadata: careevents.Metadata{
				careevents.MetaMemberName: name,
				careevents.MetaRole:       string(updated.Role),
			},
		})
		return err
	})
	if err != nil {
		return relationships.Relationship{}, err
	}
	return revoked, nil
}

// memberName finds a member's display name for an event's metadata.
//
// A failure here is not worth failing the revocation over: the event is more
// useful with a name and still useful without one, and refusing to remove
// somebody from a care circle because their name could not be read would be a
// worse outcome than an entry that reads "Somebody".
func (s *Service) memberName(ctx context.Context, seniorID, userID uuid.UUID) string {
	members, err := s.relationships.ListMembers(ctx, seniorID)
	if err != nil {
		return ""
	}
	for _, member := range members {
		if member.Relationship.UserID == userID {
			return member.DisplayName
		}
	}
	return ""
}

// load fetches a membership and confirms it belongs to the senior in the URL.
//
// Without that second check, a member of one circle could address a membership
// in another by ID and act on it, since the guard only authorized them for
// their own senior.
func (s *Service) load(
	ctx context.Context,
	seniorID uuid.UUID,
	relationshipID uuid.UUID,
) (relationships.Relationship, error) {
	target, err := s.relationships.GetByID(ctx, relationshipID)
	if errors.Is(err, relationships.ErrNotFound) {
		return relationships.Relationship{}, ErrNotFound
	}
	if err != nil {
		return relationships.Relationship{}, err
	}
	if target.SeniorID != seniorID {
		return relationships.Relationship{}, ErrNotFound
	}
	return target, nil
}
