package appointments

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/genxcare/api/internal/database"
	"github.com/genxcare/api/internal/paging"
)

// ErrNotFound is returned when no appointment matches.
var ErrNotFound = errors.New("appointment not found")

// ErrBadCursor is returned when a page cursor cannot be read. It is a sentinel
// so the handler can answer 400 without inspecting the message.
var ErrBadCursor = paging.ErrBadCursor

// Repository reads and writes appointments.
type Repository struct {
	db database.Querier
}

// NewRepository builds a Repository over the shared pool.
func NewRepository(pool *database.Pool) *Repository {
	return &Repository{db: pool}
}

const appointmentColumns = `id, senior_id, created_by_user_id, title, kind,
	provider_name, location, notes, assigned_user_id, scheduled_at, ends_at,
	status, completed_at, completed_by, cancelled_at, cancelled_by,
	created_at, updated_at`

// CreateParams describes a new appointment. The caller has already been
// authorized, and the creator comes from the session rather than the request.
type CreateParams struct {
	SeniorID        uuid.UUID
	CreatedByUserID uuid.UUID

	Title        string
	Kind         Kind
	ProviderName string
	Location     string
	Notes        string

	AssignedUserID *uuid.UUID

	ScheduledAt time.Time
	EndsAt      *time.Time
}

// Create inserts an appointment.
func (r *Repository) Create(ctx context.Context, params CreateParams) (Appointment, error) {
	appointment, err := scanAppointment(r.db.QueryRow(ctx, `
		INSERT INTO appointments (
			senior_id, created_by_user_id, title, kind, provider_name, location,
			notes, assigned_user_id, scheduled_at, ends_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING `+appointmentColumns,
		params.SeniorID,
		params.CreatedByUserID,
		params.Title,
		optionalKind(params.Kind),
		params.ProviderName,
		params.Location,
		params.Notes,
		params.AssignedUserID,
		params.ScheduledAt,
		params.EndsAt,
	))
	if err != nil {
		return Appointment{}, fmt.Errorf("create appointment: %w", err)
	}
	return appointment, nil
}

// Get loads one appointment.
func (r *Repository) Get(ctx context.Context, id uuid.UUID) (Appointment, error) {
	appointment, err := scanAppointment(r.db.QueryRow(ctx,
		`SELECT `+appointmentColumns+` FROM appointments WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Appointment{}, ErrNotFound
	}
	if err != nil {
		return Appointment{}, fmt.Errorf("get appointment: %w", err)
	}
	return appointment, nil
}

// ListWindow returns a senior's appointments starting within [from, to),
// soonest first.
//
// Used for the day view and, with a far horizon, for the upcoming list. The
// limit is what keeps a circle with years of bookings from returning all of
// them at once (plans/phase6.md §32).
func (r *Repository) ListWindow(
	ctx context.Context,
	seniorID uuid.UUID,
	from, to time.Time,
	limit int,
) ([]Appointment, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+appointmentColumns+`
		 FROM appointments
		 WHERE senior_id = $1 AND scheduled_at >= $2 AND scheduled_at < $3
		 ORDER BY scheduled_at, id
		 LIMIT $4`,
		seniorID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("list appointments: %w", err)
	}
	defer rows.Close()

	return collect(rows)
}

// Page is one page of appointment history, newest first.
type Page struct {
	Items []Appointment
	// NextCursor is empty when the history is exhausted.
	NextCursor string
}

// ListBefore returns one page of a senior's appointments from before an
// instant, newest first.
//
// Keyset rather than offset pagination: a care circle accumulates appointments
// for years, and OFFSET makes the server count past every one of them to reach
// the page nobody has read yet (docs/05, "Avoid offset pagination").
//
// Every status is returned. A cancelled visit is exactly the sort of thing
// somebody scrolling back is looking for, and hiding it would make the history
// a record of what went to plan rather than of what happened.
func (r *Repository) ListBefore(
	ctx context.Context,
	seniorID uuid.UUID,
	before time.Time,
	cursor string,
	limit int,
) (Page, error) {
	at, atID, err := paging.DecodeCursor(cursor)
	if err != nil {
		return Page{}, err
	}

	// One more than asked for, so the presence of a next page is known without
	// a second count query.
	rows, err := r.db.Query(ctx,
		`SELECT `+appointmentColumns+`
		 FROM appointments
		 WHERE senior_id = $1
		   AND scheduled_at < $2
		   AND ($3::timestamptz IS NULL OR (scheduled_at, id) < ($3, $4))
		 ORDER BY scheduled_at DESC, id DESC
		 LIMIT $5`,
		seniorID, before, at, atID, limit+1)
	if err != nil {
		return Page{}, fmt.Errorf("list appointment history: %w", err)
	}
	defer rows.Close()

	found, err := collect(rows)
	if err != nil {
		return Page{}, err
	}

	if len(found) <= limit {
		return Page{Items: found}, nil
	}

	last := found[limit-1]
	return Page{
		Items:      found[:limit],
		NextCursor: paging.EncodeCursor(last.ScheduledAt, last.ID),
	}, nil
}

// UpdateParams carries the fields an edit may change. A nil field is left as it
// is.
type UpdateParams struct {
	Title        *string
	ProviderName *string
	Location     *string
	Notes        *string
	ScheduledAt  *time.Time

	// Kind changes the sort of visit; ClearKind removes one.
	Kind      *Kind
	ClearKind bool

	// EndsAt sets an end time; ClearEndsAt removes one.
	EndsAt      *time.Time
	ClearEndsAt bool

	// AssignedUserID names who is taking them; ClearAssignee removes the name.
	AssignedUserID *uuid.UUID
	ClearAssignee  bool
}

// Update applies an edit to an appointment that is still scheduled.
//
// COALESCE keeps every untouched column at its current value, so a partial edit
// cannot blank a field the client did not mention.
//
// The `status = 'scheduled'` predicate is the real guard against rewriting
// history: an edit that arrives after somebody has completed or cancelled the
// appointment matches no row, and the service reports that rather than silently
// changing a settled record (plans/phase6.md §8). It also resolves two devices
// racing, in the database rather than in the application.
func (r *Repository) Update(
	ctx context.Context,
	id uuid.UUID,
	params UpdateParams,
) (Appointment, error) {
	var kind *string
	if params.Kind != nil {
		value := string(*params.Kind)
		kind = &value
	}

	appointment, err := scanAppointment(r.db.QueryRow(ctx, `
		UPDATE appointments
		SET title            = COALESCE($2, title),
		    provider_name    = COALESCE($3, provider_name),
		    location         = COALESCE($4, location),
		    notes            = COALESCE($5, notes),
		    scheduled_at     = COALESCE($6, scheduled_at),
		    kind             = CASE WHEN $7 THEN NULL ELSE COALESCE($8, kind) END,
		    ends_at          = CASE WHEN $9 THEN NULL ELSE COALESCE($10, ends_at) END,
		    assigned_user_id = CASE WHEN $11 THEN NULL ELSE COALESCE($12, assigned_user_id) END
		WHERE id = $1 AND status = 'scheduled'
		RETURNING `+appointmentColumns,
		id,
		params.Title,
		params.ProviderName,
		params.Location,
		params.Notes,
		params.ScheduledAt,
		params.ClearKind,
		kind,
		params.ClearEndsAt,
		params.EndsAt,
		params.ClearAssignee,
		params.AssignedUserID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Appointment{}, ErrNotFound
	}
	if err != nil {
		return Appointment{}, fmt.Errorf("update appointment: %w", err)
	}
	return appointment, nil
}

// ActParams records who settled an appointment.
type ActParams struct {
	AppointmentID uuid.UUID
	Action        Action
	ActorID       uuid.UUID
}

// Act records an appointment as completed or cancelled.
//
// The update requires it to still be scheduled, so two members acting at once
// resolve in the database: exactly one updates a row, and the other is told
// what actually happened rather than overwriting it (plans/phase6.md §25).
//
// ErrNotFound here means "no longer scheduled", which the service distinguishes
// from "no such appointment" by re-reading the row.
func (r *Repository) Act(ctx context.Context, params ActParams) (Appointment, error) {
	status := resultOf[params.Action]

	appointment, err := scanAppointment(r.db.QueryRow(ctx, `
		UPDATE appointments
		SET status       = $2,
		    completed_at = CASE WHEN $2 = 'completed' THEN now() ELSE completed_at END,
		    completed_by = CASE WHEN $2 = 'completed' THEN $3 ELSE completed_by END,
		    cancelled_at = CASE WHEN $2 = 'cancelled' THEN now() ELSE cancelled_at END,
		    cancelled_by = CASE WHEN $2 = 'cancelled' THEN $3 ELSE cancelled_by END
		WHERE id = $1 AND status = 'scheduled'
		RETURNING `+appointmentColumns,
		params.AppointmentID,
		string(status),
		params.ActorID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Appointment{}, ErrNotFound
	}
	if err != nil {
		return Appointment{}, fmt.Errorf("settle appointment: %w", err)
	}
	return appointment, nil
}

// --- Scanning --------------------------------------------------------------

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAppointment(row rowScanner) (Appointment, error) {
	var (
		appointment Appointment
		kind        *string
		status      string
	)

	err := row.Scan(
		&appointment.ID,
		&appointment.SeniorID,
		&appointment.CreatedByUserID,
		&appointment.Title,
		&kind,
		&appointment.ProviderName,
		&appointment.Location,
		&appointment.Notes,
		&appointment.AssignedUserID,
		&appointment.ScheduledAt,
		&appointment.EndsAt,
		&status,
		&appointment.CompletedAt,
		&appointment.CompletedBy,
		&appointment.CancelledAt,
		&appointment.CancelledBy,
		&appointment.CreatedAt,
		&appointment.UpdatedAt,
	)
	if err != nil {
		return Appointment{}, err
	}

	if kind != nil {
		appointment.Kind = Kind(*kind)
	}
	appointment.Status = Status(status)
	return appointment, nil
}

func collect(rows pgx.Rows) ([]Appointment, error) {
	found := make([]Appointment, 0)
	for rows.Next() {
		appointment, err := scanAppointment(rows)
		if err != nil {
			return nil, fmt.Errorf("scan appointment: %w", err)
		}
		found = append(found, appointment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read appointments: %w", err)
	}
	return found, nil
}

// optionalKind renders an unset kind as SQL NULL rather than the empty string,
// which the CHECK constraint would refuse.
func optionalKind(kind Kind) *string {
	if kind == "" {
		return nil
	}
	value := string(kind)
	return &value
}

// WithTx returns a repository bound to tx, so a appointment change and the care event
// describing it are written through the same connection and commit together
// (plans/phase7.md §26).
func (r *Repository) WithTx(tx pgx.Tx) *Repository {
	return &Repository{db: tx}
}
