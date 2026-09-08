package relationships

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/genxcare/api/internal/care"
	"github.com/genxcare/api/internal/database"
)

// ErrNotFound is returned when no relationship connects the user to the senior.
var ErrNotFound = errors.New("care relationship not found")

// Repository reads and writes care relationships.
type Repository struct {
	db database.Querier
}

// NewRepository builds a Repository over the shared pool.
func NewRepository(pool *database.Pool) *Repository {
	return &Repository{db: pool}
}

const relationshipColumns = `id, senior_id, user_id, role, permissions, status, created_at, updated_at`
const relationshipColumnsAliased = `cr.id, cr.senior_id, cr.user_id, cr.role, cr.permissions,
	cr.status, cr.created_at, cr.updated_at`

// CreateParams describes a new membership.
type CreateParams struct {
	SeniorID    uuid.UUID
	UserID      uuid.UUID
	Role        care.Role
	Permissions care.PermissionSet
	Status      care.RelationshipStatus
}

// Create inserts a membership.
func (r *Repository) Create(ctx context.Context, params CreateParams) (Relationship, error) {
	return r.create(ctx, r.db, params)
}

// CreateTx inserts a membership inside an existing transaction, so a senior and
// the creator's membership are committed together or not at all.
func (r *Repository) CreateTx(ctx context.Context, tx pgx.Tx, params CreateParams) (Relationship, error) {
	return r.create(ctx, tx, params)
}

// querier is the subset of pgx shared by the pool and a transaction.
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func (r *Repository) create(ctx context.Context, db querier, params CreateParams) (Relationship, error) {
	status := params.Status
	if status == "" {
		status = care.StatusActive
	}

	relationship, err := scanRelationship(db.QueryRow(ctx, `
		INSERT INTO care_relationships (senior_id, user_id, role, permissions, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+relationshipColumns,
		params.SeniorID,
		params.UserID,
		string(params.Role),
		care.Normalise(params.Permissions).Strings(),
		string(status),
	))
	if err != nil {
		return Relationship{}, fmt.Errorf("create care relationship: %w", err)
	}
	return relationship, nil
}

// FindByUserAndSenior loads the membership connecting a user to a senior.
//
// This is the query every authorization decision runs. It returns the
// relationship whatever its status; the caller decides what a pending or
// revoked membership means.
func (r *Repository) FindByUserAndSenior(ctx context.Context, userID, seniorID uuid.UUID) (Relationship, error) {
	relationship, err := scanRelationship(r.db.QueryRow(ctx,
		`SELECT `+relationshipColumnsAliased+`
		 FROM care_relationships cr
		 JOIN senior_profiles s ON s.id = cr.senior_id AND s.archived_at IS NULL
		 WHERE cr.user_id = $1 AND cr.senior_id = $2`,
		userID, seniorID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Relationship{}, ErrNotFound
	}
	if err != nil {
		return Relationship{}, fmt.Errorf("find care relationship: %w", err)
	}
	return relationship, nil
}

// ListForSenior returns the senior's care circle, most recently joined last.
func (r *Repository) ListForSenior(ctx context.Context, seniorID uuid.UUID) ([]Relationship, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+relationshipColumns+`
		 FROM care_relationships
		 WHERE senior_id = $1 AND status <> 'revoked'
		 ORDER BY created_at`,
		seniorID,
	)
	if err != nil {
		return nil, fmt.Errorf("list care circle: %w", err)
	}
	defer rows.Close()

	return collectRelationships(rows)
}

func scanRelationship(row pgx.Row) (Relationship, error) {
	var (
		relationship Relationship
		role         string
		status       string
		permissions  []string
	)

	err := row.Scan(
		&relationship.ID,
		&relationship.SeniorID,
		&relationship.UserID,
		&role,
		&permissions,
		&status,
		&relationship.CreatedAt,
		&relationship.UpdatedAt,
	)
	if err != nil {
		return Relationship{}, err
	}

	relationship.Role = care.Role(role)
	relationship.Status = care.RelationshipStatus(status)
	relationship.Permissions = care.PermissionsFromStrings(permissions)
	return relationship, nil
}

func collectRelationships(rows pgx.Rows) ([]Relationship, error) {
	found := make([]Relationship, 0)
	for rows.Next() {
		relationship, err := scanRelationship(rows)
		if err != nil {
			return nil, fmt.Errorf("scan care relationship: %w", err)
		}
		found = append(found, relationship)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read care relationships: %w", err)
	}
	return found, nil
}

// WithTx returns a repository bound to tx, so a membership change and the care event
// describing it are written through the same connection and commit together
// (plans/phase7.md §26).
func (r *Repository) WithTx(tx pgx.Tx) *Repository {
	return &Repository{db: tx}
}
