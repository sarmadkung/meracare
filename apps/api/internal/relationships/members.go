package relationships

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/genxcare/api/internal/care"
)

// Member is a care-circle membership together with who holds it.
type Member struct {
	Relationship Relationship
	DisplayName  string
	Email        string
	// IsSenior marks the person the circle exists for.
	IsSenior bool
}

// MemberResponse is the JSON representation of a care-circle member.
type MemberResponse struct {
	// ID is the relationship ID, which is what member endpoints address.
	ID          string   `json:"id"`
	UserID      string   `json:"userId"`
	DisplayName string   `json:"displayName"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
	Status      string   `json:"status"`
	IsSenior    bool     `json:"isSenior"`
	// IsSelf marks the reader's own membership, so the client can avoid
	// offering someone the option to remove themselves by surprise.
	IsSelf    bool   `json:"isSelf"`
	JoinedAt  string `json:"joinedAt"`
	UpdatedAt string `json:"updatedAt"`
}

// ToMemberResponse renders a member for one reader.
//
// The member's email is deliberately absent: a care circle can contain people
// who have no relationship with each other beyond the senior, and docs/09
// requires collecting and exposing the minimum.
func ToMemberResponse(member Member, readerUserID uuid.UUID) MemberResponse {
	return MemberResponse{
		ID:          member.Relationship.ID.String(),
		UserID:      member.Relationship.UserID.String(),
		DisplayName: member.DisplayName,
		Role:        string(member.Relationship.Role),
		Permissions: member.Relationship.Permissions.Strings(),
		Status:      string(member.Relationship.Status),
		IsSenior:    member.Relationship.Role == care.RoleSenior,
		IsSelf:      member.Relationship.UserID == readerUserID,
		JoinedAt:    member.Relationship.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   member.Relationship.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// ListMembers returns the senior's active care circle with each member's name.
//
// One query with a join rather than a membership list followed by a user lookup
// per member (docs/11 forbids N+1 access patterns).
func (r *Repository) ListMembers(ctx context.Context, seniorID uuid.UUID) ([]Member, error) {
	rows, err := r.db.Query(ctx, `
		SELECT cr.id, cr.senior_id, cr.user_id, cr.role, cr.permissions, cr.status,
		       cr.created_at, cr.updated_at,
		       u.display_name, coalesce(u.email, '')
		FROM care_relationships cr
		JOIN users u ON u.id = cr.user_id
		WHERE cr.senior_id = $1 AND cr.status <> 'revoked'
		ORDER BY
		    -- The senior first, then the order people joined.
		    CASE WHEN cr.role = 'senior' THEN 0 ELSE 1 END,
		    cr.created_at`,
		seniorID)
	if err != nil {
		return nil, fmt.Errorf("list care circle members: %w", err)
	}
	defer rows.Close()

	members := make([]Member, 0)
	for rows.Next() {
		var (
			member      Member
			role        string
			status      string
			permissions []string
		)
		err := rows.Scan(
			&member.Relationship.ID,
			&member.Relationship.SeniorID,
			&member.Relationship.UserID,
			&role,
			&permissions,
			&status,
			&member.Relationship.CreatedAt,
			&member.Relationship.UpdatedAt,
			&member.DisplayName,
			&member.Email,
		)
		if err != nil {
			return nil, fmt.Errorf("scan care circle member: %w", err)
		}

		member.Relationship.Role = care.Role(role)
		member.Relationship.Status = care.RelationshipStatus(status)
		member.Relationship.Permissions = care.PermissionsFromStrings(permissions)
		member.IsSenior = member.Relationship.Role == care.RoleSenior
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read care circle members: %w", err)
	}
	return members, nil
}

// GetByID loads one relationship.
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (Relationship, error) {
	relationship, err := scanRelationship(r.db.QueryRow(ctx,
		`SELECT `+relationshipColumns+` FROM care_relationships WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Relationship{}, ErrNotFound
	}
	if err != nil {
		return Relationship{}, fmt.Errorf("get care relationship: %w", err)
	}
	return relationship, nil
}

// UpdatePermissions replaces a membership's permission set.
//
// The caller has already checked that the editor may delegate every permission
// in the set; this only writes it.
func (r *Repository) UpdatePermissions(
	ctx context.Context,
	id uuid.UUID,
	permissions care.PermissionSet,
) (Relationship, error) {
	relationship, err := scanRelationship(r.db.QueryRow(ctx, `
		UPDATE care_relationships
		SET permissions = $2
		WHERE id = $1 AND status <> 'revoked'
		RETURNING `+relationshipColumns,
		id, care.Normalise(permissions).Strings()))
	if errors.Is(err, pgx.ErrNoRows) {
		return Relationship{}, ErrNotFound
	}
	if err != nil {
		return Relationship{}, fmt.Errorf("update relationship permissions: %w", err)
	}
	return relationship, nil
}

// RevokeMembership withdraws a membership.
//
// The row is updated, never deleted: care events, notes and completions
// reference their author, and docs/07 requires that history be preserved. The
// membership stops granting access the moment status changes, because every
// authorization check requires an active relationship.
func (r *Repository) RevokeMembership(ctx context.Context, id uuid.UUID) (Relationship, error) {
	relationship, err := scanRelationship(r.db.QueryRow(ctx, `
		UPDATE care_relationships
		SET status = 'revoked'
		WHERE id = $1 AND status <> 'revoked'
		RETURNING `+relationshipColumns, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Relationship{}, ErrNotFound
	}
	if err != nil {
		return Relationship{}, fmt.Errorf("revoke membership: %w", err)
	}
	return relationship, nil
}

// LockSeniorAndHasOtherManager serializes leave decisions for a care circle,
// then reports whether somebody else can still manage membership.
func (r *Repository) LockSeniorAndHasOtherManager(
	ctx context.Context,
	seniorID, leavingUserID uuid.UUID,
) (bool, error) {
	var locked uuid.UUID
	if err := r.db.QueryRow(ctx,
		`SELECT id FROM senior_profiles WHERE id = $1 FOR UPDATE`, seniorID,
	).Scan(&locked); err != nil {
		return false, fmt.Errorf("lock senior care circle: %w", err)
	}

	var found bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM care_relationships
			WHERE senior_id = $1
			  AND user_id <> $2
			  AND status = 'active'
			  AND permissions @> ARRAY['members.manage']::text[]
		)`, seniorID, leavingUserID).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("check another care-circle manager: %w", err)
	}
	return found, nil
}

// UpsertActiveTx creates a membership, or reactivates an existing one.
//
// Accepting an invitation must work for someone who was previously in the
// circle and left: the unique constraint on (senior_id, user_id) means a
// straight insert would fail, and deleting the old row would orphan their care
// history. The existing row is revived with the newly granted role and
// permissions instead.
func (r *Repository) UpsertActiveTx(ctx context.Context, tx pgx.Tx, params CreateParams) (Relationship, error) {
	relationship, err := scanRelationship(tx.QueryRow(ctx, `
		INSERT INTO care_relationships (senior_id, user_id, role, permissions, status)
		VALUES ($1, $2, $3, $4, 'active')
		ON CONFLICT (senior_id, user_id) DO UPDATE
		SET role        = EXCLUDED.role,
		    permissions = EXCLUDED.permissions,
		    status      = 'active'
		RETURNING `+relationshipColumns,
		params.SeniorID,
		params.UserID,
		string(params.Role),
		care.Normalise(params.Permissions).Strings(),
	))
	if err != nil {
		return Relationship{}, fmt.Errorf("upsert care relationship: %w", err)
	}
	return relationship, nil
}
