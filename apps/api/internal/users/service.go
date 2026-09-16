package users

import (
	"context"
	"fmt"

	"github.com/genxcare/api/internal/auth"
)

// Service holds the user-facing behaviour that sits above the repository.
type Service struct {
	repo *Repository
}

// NewService builds the service.
func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// ResolveFromClaims maps a verified Supabase identity onto an application user,
// creating the record the first time that identity signs in.
//
// This satisfies auth.UserResolver, which is how the authentication middleware
// obtains the Principal for a request.
func (s *Service) ResolveFromClaims(ctx context.Context, claims *auth.Claims) (auth.Principal, error) {
	if claims == nil {
		return auth.Principal{}, fmt.Errorf("users: nil claims")
	}

	user, err := s.repo.EnsureByAuthUserID(ctx, claims.AuthUserID, claims.Email, DefaultDisplayName(claims.Email))
	if err != nil {
		return auth.Principal{}, err
	}

	return auth.Principal{
		UserID:     user.ID,
		AuthUserID: user.AuthUserID,
		Email:      user.Email,
	}, nil
}

// Get returns one user by application ID.
func (s *Service) Get(ctx context.Context, principal auth.Principal) (User, error) {
	return s.repo.GetByID(ctx, principal.UserID)
}

// Update applies profile changes for the authenticated user.
func (s *Service) Update(ctx context.Context, principal auth.Principal, params UpdateParams) (User, error) {
	return s.repo.Update(ctx, principal.UserID, params)
}

// Compile-time proof that Service can drive the authentication middleware.
var _ auth.UserResolver = (*Service)(nil)
