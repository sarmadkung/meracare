package users

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/genxcare/api/internal/database"
)

// ErrNotFound is returned when no user matches the lookup.
var ErrNotFound = errors.New("user not found")

// Repository reads and writes application users.
type Repository struct {
	pool *database.Pool
}

// NewRepository builds a Repository over the shared pool.
func NewRepository(pool *database.Pool) *Repository {
	return &Repository{pool: pool}
}

const userColumns = `id, auth_user_id, coalesce(email, ''), display_name,
	coalesce(avatar_url, ''), coalesce(phone, ''), created_at, updated_at`

// EnsureByAuthUserID returns the user for a Supabase identity, creating it on
// first sign-in.
//
// The insert is idempotent: concurrent first requests for the same identity
// collapse onto one row via the unique constraint on auth_user_id.
func (r *Repository) EnsureByAuthUserID(ctx context.Context, authUserID uuid.UUID, email, displayName string) (User, error) {
	normalisedEmail := normaliseEmail(email)

	query := `
		INSERT INTO users (auth_user_id, email, display_name)
		VALUES ($1, $2, $3)
		ON CONFLICT (auth_user_id) DO UPDATE
			SET email = coalesce(EXCLUDED.email, users.email)
		RETURNING ` + userColumns

	user, err := scanUser(r.pool.QueryRow(ctx, query, authUserID, normalisedEmail, displayName))
	if err == nil {
		return user, nil
	}

	// Another account already claims this email address. Fall back to creating
	// the user without one rather than refusing the sign-in; the address stays
	// authoritative in Supabase Auth.
	if database.IsUniqueViolation(err, "users_email_lower_key") {
		return scanUser(r.pool.QueryRow(ctx, query, authUserID, nil, displayName))
	}
	return User{}, fmt.Errorf("ensure user: %w", err)
}

// GetByID loads one user by application ID.
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (User, error) {
	user, err := scanUser(r.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("get user: %w", err)
	}
	return user, nil
}

// UpdateParams carries the fields `PATCH /v1/me` may change. A nil pointer
// means "leave unchanged"; a pointer to an empty string clears the value.
type UpdateParams struct {
	DisplayName *string
	AvatarURL   *string
	Phone       *string
}

// Update applies the supplied changes and returns the stored user.
func (r *Repository) Update(ctx context.Context, id uuid.UUID, params UpdateParams) (User, error) {
	query := `
		UPDATE users SET
			display_name = coalesce($2, display_name),
			avatar_url   = CASE WHEN $3::boolean THEN $4 ELSE avatar_url END,
			phone        = CASE WHEN $5::boolean THEN $6 ELSE phone END
		WHERE id = $1
		RETURNING ` + userColumns

	user, err := scanUser(r.pool.QueryRow(ctx, query,
		id,
		params.DisplayName,
		params.AvatarURL != nil, nullableString(params.AvatarURL),
		params.Phone != nil, nullableString(params.Phone),
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("update user: %w", err)
	}
	return user, nil
}

func scanUser(row pgx.Row) (User, error) {
	var user User
	err := row.Scan(
		&user.ID,
		&user.AuthUserID,
		&user.Email,
		&user.DisplayName,
		&user.AvatarURL,
		&user.Phone,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	return user, err
}

// normaliseEmail lowercases the address and returns nil for an empty one, so
// the partial unique index treats missing addresses as absent rather than blank.
func normaliseEmail(email string) *string {
	trimmed := strings.ToLower(strings.TrimSpace(email))
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// nullableString maps an empty string to SQL NULL so clearing a field stores
// NULL rather than "".
func nullableString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
