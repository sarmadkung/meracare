package users_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/genxcare/api/internal/auth"
	"github.com/genxcare/api/internal/testsupport"
	"github.com/genxcare/api/internal/users"
)

// These tests exercise real SQL. They are skipped unless TEST_DATABASE_URL is
// set — see internal/testsupport.

func TestEnsureByAuthUserIDCreatesThenReturnsSameUser(t *testing.T) {
	pool := testsupport.RequireDatabase(t)
	repo := users.NewRepository(pool)
	ctx := context.Background()

	authUserID := uuid.New()

	created, err := repo.EnsureByAuthUserID(ctx, authUserID, "Sara@Example.com", "sara")
	if err != nil {
		t.Fatalf("first EnsureByAuthUserID: %v", err)
	}
	if created.ID == uuid.Nil {
		t.Fatal("created user has no ID")
	}
	if created.Email != "sara@example.com" {
		t.Errorf("Email = %q, want the address lowercased", created.Email)
	}
	if created.DisplayName != "sara" {
		t.Errorf("DisplayName = %q, want sara", created.DisplayName)
	}

	// A second sign-in must reuse the same application user.
	again, err := repo.EnsureByAuthUserID(ctx, authUserID, "sara@example.com", "ignored")
	if err != nil {
		t.Fatalf("second EnsureByAuthUserID: %v", err)
	}
	if again.ID != created.ID {
		t.Errorf("second sign-in created a new user (%s != %s)", again.ID, created.ID)
	}
	if again.DisplayName != "sara" {
		t.Errorf("DisplayName = %q, want the stored name to survive sign-in", again.DisplayName)
	}
}

// Two Supabase identities sharing an email must both be able to sign in; the
// second simply stores no address.
func TestEnsureByAuthUserIDToleratesDuplicateEmail(t *testing.T) {
	pool := testsupport.RequireDatabase(t)
	repo := users.NewRepository(pool)
	ctx := context.Background()

	if _, err := repo.EnsureByAuthUserID(ctx, uuid.New(), "shared@example.com", "first"); err != nil {
		t.Fatalf("first user: %v", err)
	}

	second, err := repo.EnsureByAuthUserID(ctx, uuid.New(), "shared@example.com", "second")
	if err != nil {
		t.Fatalf("second user: %v", err)
	}
	if second.Email != "" {
		t.Errorf("Email = %q, want empty for the colliding address", second.Email)
	}
}

func TestGetByIDReturnsErrNotFoundForUnknownUser(t *testing.T) {
	pool := testsupport.RequireDatabase(t)
	repo := users.NewRepository(pool)

	if _, err := repo.GetByID(context.Background(), uuid.New()); !errors.Is(err, users.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestUpdateAppliesOnlySuppliedFields(t *testing.T) {
	pool := testsupport.RequireDatabase(t)
	repo := users.NewRepository(pool)
	ctx := context.Background()

	user, err := repo.EnsureByAuthUserID(ctx, uuid.New(), "ahmed@example.com", "ahmed")
	if err != nil {
		t.Fatalf("EnsureByAuthUserID: %v", err)
	}

	displayName := "Ahmed Khan"
	phone := "+92 300 1234567"
	updated, err := repo.Update(ctx, user.ID, users.UpdateParams{DisplayName: &displayName, Phone: &phone})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.DisplayName != displayName {
		t.Errorf("DisplayName = %q, want %q", updated.DisplayName, displayName)
	}
	if updated.Phone != phone {
		t.Errorf("Phone = %q, want %q", updated.Phone, phone)
	}
	if updated.Email != "ahmed@example.com" {
		t.Errorf("Email = %q, want it untouched", updated.Email)
	}
	if !updated.UpdatedAt.After(user.UpdatedAt) {
		t.Error("updated_at trigger did not advance the timestamp")
	}

	// An explicit empty string clears the field; omitted fields stay as they are.
	blank := ""
	cleared, err := repo.Update(ctx, user.ID, users.UpdateParams{Phone: &blank})
	if err != nil {
		t.Fatalf("Update (clear): %v", err)
	}
	if cleared.Phone != "" {
		t.Errorf("Phone = %q, want cleared", cleared.Phone)
	}
	if cleared.DisplayName != displayName {
		t.Errorf("DisplayName = %q, want it unchanged", cleared.DisplayName)
	}
}

func TestUpdateReturnsErrNotFoundForUnknownUser(t *testing.T) {
	pool := testsupport.RequireDatabase(t)
	repo := users.NewRepository(pool)

	name := "Ghost"
	if _, err := repo.Update(context.Background(), uuid.New(), users.UpdateParams{DisplayName: &name}); !errors.Is(err, users.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

// The service is what the authentication middleware calls on every request.
func TestServiceResolveFromClaimsCreatesUserOnFirstSignIn(t *testing.T) {
	pool := testsupport.RequireDatabase(t)
	service := users.NewService(users.NewRepository(pool))
	ctx := context.Background()

	authUserID := uuid.New()
	claims := &auth.Claims{AuthUserID: authUserID, Email: "maria@example.com"}

	principal, err := service.ResolveFromClaims(ctx, claims)
	if err != nil {
		t.Fatalf("ResolveFromClaims: %v", err)
	}
	if principal.UserID == uuid.Nil {
		t.Fatal("principal has no application user ID")
	}
	if principal.AuthUserID != authUserID {
		t.Errorf("AuthUserID = %s, want %s", principal.AuthUserID, authUserID)
	}

	stored, err := service.Get(ctx, principal)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.DisplayName != "maria" {
		t.Errorf("DisplayName = %q, want maria derived from the email", stored.DisplayName)
	}
}
