// Package authz enforces relationship-based authorization.
//
// docs/02-permissions-and-authorization.md requires every protected request to
// establish three things, in order:
//
//  1. the authenticated user — done by internal/auth;
//  2. their relationship to the senior in the URL;
//  3. their permission for the requested action.
//
// Steps 2 and 3 happen here, as middleware, so a handler cannot forget them.
// A handler that reads a senior ID from the URL without passing through a Guard
// has no way to obtain the relationship it would need.
package authz

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/genxcare/api/internal/auth"
	"github.com/genxcare/api/internal/care"
	"github.com/genxcare/api/internal/relationships"
	"github.com/genxcare/api/pkg/httpx"
	"github.com/genxcare/api/pkg/logging"
)

// SeniorIDParam is the chi route parameter carrying the senior's ID.
const SeniorIDParam = "seniorID"

// RelationshipResolver loads the membership connecting a user to a senior.
// internal/relationships.Repository satisfies it.
type RelationshipResolver interface {
	FindByUserAndSenior(ctx context.Context, userID, seniorID uuid.UUID) (relationships.Relationship, error)
}

// Guard builds middleware that authorizes access to one senior.
type Guard struct {
	resolver RelationshipResolver
}

// NewGuard builds a Guard.
func NewGuard(resolver RelationshipResolver) *Guard {
	return &Guard{resolver: resolver}
}

type relationshipContextKey struct{}

// RequirePermission authorizes the caller against the senior named in the URL.
//
// Every failure — malformed ID, no relationship, an inactive one, or a missing
// permission — returns the same 404. Distinguishing "you may not see this
// senior" from "this senior does not exist" would let anyone enumerate the
// seniors on the platform by probing IDs (docs/09-security-privacy.md).
func (g *Guard) RequirePermission(permissions ...care.Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			logger := logging.FromContext(ctx)
			principal := auth.MustPrincipal(ctx)

			seniorID, err := uuid.Parse(chi.URLParam(r, SeniorIDParam))
			if err != nil {
				notFound(w, r)
				return
			}

			relationship, err := g.resolver.FindByUserAndSenior(ctx, principal.UserID, seniorID)
			if err != nil {
				if errors.Is(err, relationships.ErrNotFound) {
					logger.Info("denied access to senior",
						slog.String("senior_id", seniorID.String()),
						slog.String("reason", "no care relationship"),
					)
					notFound(w, r)
					return
				}
				httpx.WriteError(w, r, httpx.ErrInternal(err))
				return
			}

			for _, permission := range permissions {
				if relationship.Can(permission) {
					continue
				}
				logger.Info("denied access to senior",
					slog.String("senior_id", seniorID.String()),
					slog.String("required_permission", string(permission)),
					slog.String("relationship_status", string(relationship.Status)),
				)
				notFound(w, r)
				return
			}

			next.ServeHTTP(w, r.WithContext(WithRelationship(ctx, relationship)))
		})
	}
}

func notFound(w http.ResponseWriter, r *http.Request) {
	httpx.WriteError(w, r, httpx.ErrNotFound("That senior does not exist, or you do not have access."))
}

// WithRelationship returns a context carrying the authorized relationship.
func WithRelationship(ctx context.Context, relationship relationships.Relationship) context.Context {
	return context.WithValue(ctx, relationshipContextKey{}, relationship)
}

// RelationshipFrom returns the authorized relationship, if the request passed
// through a Guard.
func RelationshipFrom(ctx context.Context) (relationships.Relationship, bool) {
	relationship, ok := ctx.Value(relationshipContextKey{}).(relationships.Relationship)
	return relationship, ok
}

// MustRelationship returns the authorized relationship.
//
// It panics when called outside a guarded route. That is a programming error,
// and the recovery middleware turns it into a 500 — which is far safer than a
// handler silently proceeding with a zero-value relationship that grants
// nothing but reads as "no error".
func MustRelationship(ctx context.Context) relationships.Relationship {
	relationship, ok := RelationshipFrom(ctx)
	if !ok {
		panic("authz: no relationship in context; handler is not behind a Guard")
	}
	return relationship
}
