package invitations

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/genxcare/api/internal/care"
	"github.com/genxcare/api/internal/database"
)

// ErrNotFound is returned when no invitation matches the lookup.
var ErrNotFound = errors.New("invitation not found")

// Repository reads and writes invitations.
type Repository struct {
	db database.Querier
}

// NewRepository builds a Repository over the shared pool.
func NewRepository(pool *database.Pool) *Repository {
	return &Repository{db: pool}
}

const invitationColumns = `id, senior_id, inviter_user_id, invitee_email, role, permissions,
	status, expires_at, accepted_at, accepted_by_user_id, revoked_at, created_at, updated_at`

// CreateParams describes a new invitation. The caller has already validated the
// role and narrowed the permissions to what the inviter may delegate.
type CreateParams struct {
	SeniorID      uuid.UUID
	InviterUserID uuid.UUID
	InviteeEmail  string
	Role          care.Role
	Permissions   care.PermissionSet
	TokenHash     []byte
	ExpiresAt     time.Time
}

// ErrAlreadyInvited is returned when the person already has an outstanding
// invitation to this circle.
var ErrAlreadyInvited = errors.New("an invitation is already pending for this person")

// Create inserts an invitation.
func (r *Repository) Create(ctx context.Context, params CreateParams) (Invitation, error) {
	invitation, err := scanInvitation(r.db.QueryRow(ctx, `
		INSERT INTO invitations (
			senior_id, inviter_user_id, invitee_email, role, permissions, token_hash, expires_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+invitationColumns,
		params.SeniorID,
		params.InviterUserID,
		NormaliseEmail(params.InviteeEmail),
		string(params.Role),
		params.Permissions.Strings(),
		params.TokenHash,
		params.ExpiresAt,
	))
	if err != nil {
		if database.IsUniqueViolation(err, "invitations_one_pending_per_invitee") {
			return Invitation{}, ErrAlreadyInvited
		}
		return Invitation{}, fmt.Errorf("create invitation: %w", err)
	}
	return invitation, nil
}

// FindByTokenHash looks an invitation up by the hash of its token.
//
// A single indexed probe: the raw token is never compared in the database, and
// never stored there to be compared against.
func (r *Repository) FindByTokenHash(ctx context.Context, tokenHash []byte) (Invitation, error) {
	invitation, err := scanInvitation(r.db.QueryRow(ctx,
		`SELECT `+invitationColumns+` FROM invitations WHERE token_hash = $1`, tokenHash))
	if errors.Is(err, pgx.ErrNoRows) {
		return Invitation{}, ErrNotFound
	}
	if err != nil {
		return Invitation{}, fmt.Errorf("find invitation by token: %w", err)
	}
	return invitation, nil
}

// GetByID loads one invitation.
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (Invitation, error) {
	invitation, err := scanInvitation(r.db.QueryRow(ctx,
		`SELECT `+invitationColumns+` FROM invitations WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Invitation{}, ErrNotFound
	}
	if err != nil {
		return Invitation{}, fmt.Errorf("get invitation: %w", err)
	}
	return invitation, nil
}

// ListForSenior returns a circle's invitations, newest first.
func (r *Repository) ListForSenior(ctx context.Context, seniorID uuid.UUID) ([]Invitation, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+invitationColumns+` FROM invitations WHERE senior_id = $1 ORDER BY created_at DESC`,
		seniorID)
	if err != nil {
		return nil, fmt.Errorf("list invitations: %w", err)
	}
	defer rows.Close()

	found := make([]Invitation, 0)
	for rows.Next() {
		invitation, err := scanInvitation(rows)
		if err != nil {
			return nil, fmt.Errorf("scan invitation: %w", err)
		}
		found = append(found, invitation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read invitations: %w", err)
	}
	return found, nil
}

// Revoke marks a pending invitation revoked.
//
// The update is conditional on the invitation still being pending, so a race
// between accepting and revoking resolves in the database rather than in the
// application: exactly one of the two wins.
func (r *Repository) Revoke(ctx context.Context, id uuid.UUID) (Invitation, error) {
	invitation, err := scanInvitation(r.db.QueryRow(ctx, `
		UPDATE invitations
		SET status = 'revoked', revoked_at = now()
		WHERE id = $1 AND status = 'pending'
		RETURNING `+invitationColumns, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Invitation{}, ErrNotFound
	}
	if err != nil {
		return Invitation{}, fmt.Errorf("revoke invitation: %w", err)
	}
	return invitation, nil
}

// MarkAcceptedTx consumes an invitation inside a transaction.
//
// Like Revoke, the update requires the row to still be pending and unexpired.
// That single conditional UPDATE is what makes a token single-use: two
// concurrent accepts both attempt it, and only one updates a row.
func (r *Repository) MarkAcceptedTx(
	ctx context.Context,
	tx pgx.Tx,
	id uuid.UUID,
	acceptedByUserID uuid.UUID,
) (Invitation, error) {
	invitation, err := scanInvitation(tx.QueryRow(ctx, `
		UPDATE invitations
		SET status = 'accepted', accepted_at = now(), accepted_by_user_id = $2
		WHERE id = $1 AND status = 'pending' AND expires_at > now()
		RETURNING `+invitationColumns, id, acceptedByUserID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Invitation{}, ErrNotFound
	}
	if err != nil {
		return Invitation{}, fmt.Errorf("accept invitation: %w", err)
	}
	return invitation, nil
}

// ExpirePending marks lapsed invitations expired.
//
// Housekeeping only: EffectiveStatus already treats a lapsed invitation as
// expired, so correctness never depends on this having run.
func (r *Repository) ExpirePending(ctx context.Context) (int64, error) {
	tag, err := r.db.Exec(ctx,
		`UPDATE invitations SET status = 'expired' WHERE status = 'pending' AND expires_at <= now()`)
	if err != nil {
		return 0, fmt.Errorf("expire invitations: %w", err)
	}
	return tag.RowsAffected(), nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanInvitation(row rowScanner) (Invitation, error) {
	var (
		invitation  Invitation
		role        string
		status      string
		permissions []string
	)

	err := row.Scan(
		&invitation.ID,
		&invitation.SeniorID,
		&invitation.InviterUserID,
		&invitation.InviteeEmail,
		&role,
		&permissions,
		&status,
		&invitation.ExpiresAt,
		&invitation.AcceptedAt,
		&invitation.AcceptedByUserID,
		&invitation.RevokedAt,
		&invitation.CreatedAt,
		&invitation.UpdatedAt,
	)
	if err != nil {
		return Invitation{}, err
	}

	invitation.Role = care.Role(role)
	invitation.Status = Status(status)
	invitation.Permissions = care.PermissionsFromStrings(permissions)
	return invitation, nil
}

// WithTx returns a repository bound to tx, so a invitation change and the care event
// describing it are written through the same connection and commit together
// (plans/phase7.md §26).
func (r *Repository) WithTx(tx pgx.Tx) *Repository {
	return &Repository{db: tx}
}
