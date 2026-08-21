package seniors

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/meracare/api/internal/care"
	"github.com/meracare/api/internal/database"
	"github.com/meracare/api/internal/relationships"
)

// ErrNotFound is returned when no senior profile matches.
var ErrNotFound = errors.New("senior profile not found")

// Repository reads and writes senior profiles.
type Repository struct {
	pool *database.Pool
}

// NewRepository builds a Repository over the shared pool.
func NewRepository(pool *database.Pool) *Repository {
	return &Repository{pool: pool}
}

const seniorColumns = `id, user_id, created_by_user_id, display_name, date_of_birth,
	coalesce(photo_url, ''), coalesce(phone, ''), coalesce(address, ''),
	coalesce(emergency_contact, ''), timezone, created_at, updated_at`

// seniorColumnsAliased is seniorColumns qualified for the join in ListForUser.
// Written out rather than derived, so the two lists stay obviously in step.
const seniorColumnsAliased = `s.id, s.user_id, s.created_by_user_id, s.display_name, s.date_of_birth,
	coalesce(s.photo_url, ''), coalesce(s.phone, ''), coalesce(s.address, ''),
	coalesce(s.emergency_contact, ''), s.timezone, s.created_at, s.updated_at`

// CreateParams describes a new senior profile.
type CreateParams struct {
	// UserID links the profile to an account, set only for Solo Mode.
	UserID uuid.NullUUID
	// CreatedByUserID is the authenticated caller.
	CreatedByUserID uuid.UUID

	DisplayName      string
	DateOfBirth      *time.Time
	Phone            string
	Address          string
	EmergencyContact string
	// Timezone is an IANA name; empty falls back to the column default.
	Timezone string
}

// CreateTx inserts a senior profile inside a transaction.
//
// A senior with no care relationship would be unreachable by anyone, so
// creation is only exposed transactionally: the caller commits the profile and
// the creator's membership together.
func (r *Repository) CreateTx(ctx context.Context, tx pgx.Tx, params CreateParams) (Senior, error) {
	senior, err := scanSenior(tx.QueryRow(ctx, `
		INSERT INTO senior_profiles (
			user_id, created_by_user_id, display_name, date_of_birth,
			phone, address, emergency_contact, timezone
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, coalesce($8, 'UTC'))
		RETURNING `+seniorColumns,
		params.UserID,
		params.CreatedByUserID,
		strings.TrimSpace(params.DisplayName),
		params.DateOfBirth,
		nullableString(params.Phone),
		nullableString(params.Address),
		nullableString(params.EmergencyContact),
		nullableString(params.Timezone),
	))
	if err != nil {
		return Senior{}, fmt.Errorf("create senior profile: %w", err)
	}
	return senior, nil
}

// GetByID loads one senior profile.
//
// Authorization happens in middleware before this is reached; the repository
// does not re-check it.
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (Senior, error) {
	senior, err := scanSenior(r.pool.QueryRow(ctx,
		`SELECT `+seniorColumns+` FROM senior_profiles WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Senior{}, ErrNotFound
	}
	if err != nil {
		return Senior{}, fmt.Errorf("get senior profile: %w", err)
	}
	return senior, nil
}

// Membership pairs a senior with the caller's relationship to them.
type Membership struct {
	Senior       Senior
	Relationship relationships.Relationship
}

// ListForUser returns every senior the user can currently reach, together with
// the relationship that grants the access.
//
// One query rather than a list of IDs followed by a lookup per senior: a
// professional caregiver may have many seniors, and docs/11 forbids N+1 access
// patterns on the caregiver's home screen.
func (r *Repository) ListForUser(ctx context.Context, userID uuid.UUID) ([]Membership, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+seniorColumnsAliased+`,
		       cr.id, cr.senior_id, cr.user_id, cr.role, cr.permissions, cr.status,
		       cr.created_at, cr.updated_at
		FROM care_relationships cr
		JOIN senior_profiles s ON s.id = cr.senior_id
		WHERE cr.user_id = $1 AND cr.status = 'active'
		ORDER BY s.display_name, s.id`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list seniors: %w", err)
	}
	defer rows.Close()

	memberships := make([]Membership, 0)
	for rows.Next() {
		membership, err := scanMembership(rows)
		if err != nil {
			return nil, err
		}
		memberships = append(memberships, membership)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read seniors: %w", err)
	}
	return memberships, nil
}

// UpdateParams carries the fields PATCH may change. A nil pointer leaves the
// field unchanged; a pointer to an empty value clears it.
type UpdateParams struct {
	DisplayName *string
	// ClearDateOfBirth distinguishes "leave it alone" from "remove it", which a
	// nil *time.Time alone cannot express.
	DateOfBirth      *time.Time
	ClearDateOfBirth bool
	Phone            *string
	Address          *string
	EmergencyContact *string
	Timezone         *string
}

// Update applies the supplied changes and returns the stored profile.
// ListAllActive returns every active membership in the system.
//
// The notification scheduler's roster. It works outwards from memberships
// rather than from users because a user with no active relationship has nothing
// to be notified about, and a revoked caregiver leaves this result the moment
// they are revoked (plans/phase11.md §§12, 31).
//
// Unpaged, and that is a deliberate MVP decision rather than an oversight: one
// pass needs the whole roster at once to decide anything, and at MeraCare's
// scale the whole roster is a few thousand rows. The first deployment where
// that stops being true will need the sweep sharded by senior, which is a
// change to the scheduler rather than to this query.
func (r *Repository) ListAllActive(ctx context.Context) ([]Membership, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+seniorColumnsAliased+`,
		       cr.id, cr.senior_id, cr.user_id, cr.role, cr.permissions, cr.status,
		       cr.created_at, cr.updated_at
		FROM care_relationships cr
		JOIN senior_profiles s ON s.id = cr.senior_id
		WHERE cr.status = 'active'
		ORDER BY cr.senior_id, cr.user_id`)
	if err != nil {
		return nil, fmt.Errorf("list active memberships: %w", err)
	}
	defer rows.Close()

	memberships := make([]Membership, 0)
	for rows.Next() {
		membership, err := scanMembership(rows)
		if err != nil {
			return nil, err
		}
		memberships = append(memberships, membership)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read active memberships: %w", err)
	}
	return memberships, nil
}

func (r *Repository) Update(ctx context.Context, id uuid.UUID, params UpdateParams) (Senior, error) {
	senior, err := scanSenior(r.pool.QueryRow(ctx, `
		UPDATE senior_profiles SET
			display_name      = coalesce($2, display_name),
			date_of_birth     = CASE WHEN $3::boolean THEN $4::date ELSE date_of_birth END,
			phone             = CASE WHEN $5::boolean THEN $6 ELSE phone END,
			address           = CASE WHEN $7::boolean THEN $8 ELSE address END,
			emergency_contact = CASE WHEN $9::boolean THEN $10 ELSE emergency_contact END,
			timezone          = coalesce($11, timezone)
		WHERE id = $1
		RETURNING `+seniorColumns,
		id,
		trimmedOrNil(params.DisplayName),
		params.DateOfBirth != nil || params.ClearDateOfBirth, params.DateOfBirth,
		params.Phone != nil, nullablePointer(params.Phone),
		params.Address != nil, nullablePointer(params.Address),
		params.EmergencyContact != nil, nullablePointer(params.EmergencyContact),
		nullablePointer(params.Timezone),
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Senior{}, ErrNotFound
	}
	if err != nil {
		return Senior{}, fmt.Errorf("update senior profile: %w", err)
	}
	return senior, nil
}

// BeginTx starts a transaction for operations that span tables.
func (r *Repository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.pool.Begin(ctx)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSenior(row rowScanner) (Senior, error) {
	var (
		senior      Senior
		dateOfBirth *time.Time
	)

	err := row.Scan(
		&senior.ID,
		&senior.UserID,
		&senior.CreatedByUserID,
		&senior.DisplayName,
		&dateOfBirth,
		&senior.PhotoURL,
		&senior.Phone,
		&senior.Address,
		&senior.EmergencyContact,
		&senior.Timezone,
		&senior.CreatedAt,
		&senior.UpdatedAt,
	)
	if err != nil {
		return Senior{}, err
	}

	senior.DateOfBirth = dateOfBirth
	return senior, nil
}

func scanMembership(row rowScanner) (Membership, error) {
	var (
		membership  Membership
		dateOfBirth *time.Time
		role        string
		status      string
		permissions []string
	)

	err := row.Scan(
		&membership.Senior.ID,
		&membership.Senior.UserID,
		&membership.Senior.CreatedByUserID,
		&membership.Senior.DisplayName,
		&dateOfBirth,
		&membership.Senior.PhotoURL,
		&membership.Senior.Phone,
		&membership.Senior.Address,
		&membership.Senior.EmergencyContact,
		&membership.Senior.Timezone,
		&membership.Senior.CreatedAt,
		&membership.Senior.UpdatedAt,

		&membership.Relationship.ID,
		&membership.Relationship.SeniorID,
		&membership.Relationship.UserID,
		&role,
		&permissions,
		&status,
		&membership.Relationship.CreatedAt,
		&membership.Relationship.UpdatedAt,
	)
	if err != nil {
		return Membership{}, fmt.Errorf("scan senior: %w", err)
	}

	membership.Senior.DateOfBirth = dateOfBirth
	membership.Relationship.Role = care.Role(role)
	membership.Relationship.Status = care.RelationshipStatus(status)
	membership.Relationship.Permissions = care.PermissionsFromStrings(permissions)
	return membership, nil
}

func nullableString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func nullablePointer(value *string) *string {
	if value == nil {
		return nil
	}
	return nullableString(*value)
}

func trimmedOrNil(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}
