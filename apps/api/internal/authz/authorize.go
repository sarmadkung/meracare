package authz

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/genxcare/api/internal/care"
	"github.com/genxcare/api/internal/relationships"
)

// ErrDenied is returned when the caller has no usable relationship to the
// senior, or lacks a required permission.
//
// It is a single error for every reason, matching the middleware's single 404:
// the caller learns that they cannot act, never why.
var ErrDenied = errors.New("access denied")

// Authorize resolves a caller's relationship to a senior and checks permissions.
//
// It exists for handlers whose senior is named by the resource rather than the
// URL — revoking an invitation by its own ID, for instance. Those cannot use
// RequirePermission as middleware, but they must not hand-roll the check, so
// they share this one code path.
func (g *Guard) Authorize(
	ctx context.Context,
	userID uuid.UUID,
	seniorID uuid.UUID,
	permissions ...care.Permission,
) (relationships.Relationship, error) {
	relationship, err := g.resolver.FindByUserAndSenior(ctx, userID, seniorID)
	if err != nil {
		if errors.Is(err, relationships.ErrNotFound) {
			return relationships.Relationship{}, ErrDenied
		}
		return relationships.Relationship{}, err
	}

	for _, permission := range permissions {
		if !relationship.Can(permission) {
			return relationships.Relationship{}, ErrDenied
		}
	}
	return relationship, nil
}
