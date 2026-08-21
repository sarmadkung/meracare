package careevents

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/meracare/api/internal/database"
	"github.com/meracare/api/internal/paging"
)

// ErrBadCursor is returned when a page cursor cannot be read. It is a sentinel
// so the handler can answer 400 without inspecting the message.
var ErrBadCursor = paging.ErrBadCursor

// Repository reads and writes care events.
type Repository struct {
	db database.Querier
}

// NewRepository builds a Repository over the shared pool.
func NewRepository(pool *database.Pool) *Repository {
	return &Repository{db: pool}
}

// WithTx returns a repository bound to tx, so an event is written through the
// same connection as the domain change it describes (plans/phase7.md §26).
func (r *Repository) WithTx(tx pgx.Tx) *Repository {
	return &Repository{db: tx}
}

const eventColumns = `id, senior_id, actor_user_id, event_type, entity_type,
	entity_id, metadata, occurred_at, created_at`

// RecordParams describes one event to write.
type RecordParams struct {
	SeniorID    uuid.UUID
	ActorUserID *uuid.UUID
	Type        Type
	EntityType  EntityType
	EntityID    uuid.UUID
	Metadata    Metadata
	// OccurredAt defaults to the database clock when zero, which is what almost
	// every caller wants: the event happened as the transaction ran.
	OccurredAt time.Time
}

// Record writes one event.
//
// There is no Update and no Delete, deliberately. A care event is a historical
// record: if the task it describes is renamed next month, the event still says
// what was true when it was written (plans/phase7.md §5). The absence of the
// methods is the enforcement.
func (r *Repository) Record(ctx context.Context, params RecordParams) (Event, error) {
	metadata, err := json.Marshal(cleaned(params.Metadata))
	if err != nil {
		return Event{}, fmt.Errorf("encode event metadata: %w", err)
	}

	var occurredAt *time.Time
	if !params.OccurredAt.IsZero() {
		occurredAt = &params.OccurredAt
	}

	event, err := scanEvent(r.db.QueryRow(ctx, `
		INSERT INTO care_events (
			senior_id, actor_user_id, event_type, entity_type, entity_id,
			metadata, occurred_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, COALESCE($7, now()))
		RETURNING `+eventColumns,
		params.SeniorID,
		params.ActorUserID,
		string(params.Type),
		string(params.EntityType),
		params.EntityID,
		metadata,
		occurredAt,
	))
	if err != nil {
		return Event{}, fmt.Errorf("record care event: %w", err)
	}
	return event, nil
}

// Page is one page of a senior's activity, newest first.
type Page struct {
	Items []Event
	// NextCursor is empty when the timeline is exhausted.
	NextCursor string
}

// List returns one page of a senior's activity, newest first.
//
// Keyset rather than offset pagination: a timeline grows without limit and
// OFFSET makes the server count past every row to reach the page nobody has
// read yet (docs/05, "Avoid offset pagination"; plans/phase7.md §12).
//
// The cursor is (occurred_at, id) rather than occurred_at alone, and that
// matters more here than anywhere else in the API: several events routinely
// share an instant — a task completed and its event written in the same
// transaction, or a queue of offline mutations draining at once — and a
// timestamp-only cursor would drop or repeat one at every page boundary.
func (r *Repository) List(
	ctx context.Context,
	seniorID uuid.UUID,
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
		`SELECT `+eventColumns+`
		 FROM care_events
		 WHERE senior_id = $1
		   AND ($2::timestamptz IS NULL OR (occurred_at, id) < ($2, $3))
		 ORDER BY occurred_at DESC, id DESC
		 LIMIT $4`,
		seniorID, at, atID, limit+1)
	if err != nil {
		return Page{}, fmt.Errorf("list care events: %w", err)
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
		NextCursor: paging.EncodeCursor(last.OccurredAt, last.ID),
	}, nil
}

// RecentEvent is one care event, with the actor's name resolved, as the
// notification scheduler needs it.
//
// A separate shape from Event because it is a different question: Event answers
// "what is this senior's history?", this answers "what has just happened
// anywhere, and who did it?" — across every senior, with the name joined in so
// a sweep does not make one query per event.
type RecentEvent struct {
	ID          uuid.UUID
	SeniorID    uuid.UUID
	Type        Type
	ActorUserID *uuid.UUID
	ActorName   string
	OccurredAt  time.Time
}

// ListRecent returns events of the given types that occurred in [from, to),
// across every senior.
//
// Read after commit, which is what makes it safe for the scheduler to notify
// from: an event that rolled back was never visible, so there is no notification
// to undo (plans/phase11.md §52).
func (r *Repository) ListRecent(
	ctx context.Context,
	types []Type,
	from, to time.Time,
) ([]RecentEvent, error) {
	if len(types) == 0 {
		return nil, nil
	}

	names := make([]string, 0, len(types))
	for _, eventType := range types {
		names = append(names, string(eventType))
	}

	rows, err := r.db.Query(ctx, `
		SELECT e.id, e.senior_id, e.event_type, e.actor_user_id,
		       COALESCE(u.display_name, ''), e.occurred_at
		  FROM care_events e
		  LEFT JOIN users u ON u.id = e.actor_user_id
		 WHERE e.event_type = ANY($1)
		   AND e.occurred_at >= $2 AND e.occurred_at < $3
		 ORDER BY e.occurred_at`,
		names, from, to)
	if err != nil {
		return nil, fmt.Errorf("list recent care events: %w", err)
	}
	defer rows.Close()

	found := make([]RecentEvent, 0)
	for rows.Next() {
		var (
			event     RecentEvent
			eventType string
		)
		if err := rows.Scan(
			&event.ID, &event.SeniorID, &eventType, &event.ActorUserID,
			&event.ActorName, &event.OccurredAt,
		); err != nil {
			return nil, fmt.Errorf("read recent care events: %w", err)
		}
		event.Type = Type(eventType)
		found = append(found, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read recent care events: %w", err)
	}
	return found, nil
}

// cleaned drops empty values, so an absent title is an absent key rather than
// an empty string the renderer has to special-case.
func cleaned(metadata Metadata) Metadata {
	if metadata == nil {
		return Metadata{}
	}

	kept := make(Metadata, len(metadata))
	for key, value := range metadata {
		if value != "" {
			kept[key] = value
		}
	}
	return kept
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanEvent(row rowScanner) (Event, error) {
	var (
		event      Event
		eventType  string
		entityType string
		metadata   []byte
	)

	err := row.Scan(
		&event.ID,
		&event.SeniorID,
		&event.ActorUserID,
		&eventType,
		&entityType,
		&event.EntityID,
		&metadata,
		&event.OccurredAt,
		&event.CreatedAt,
	)
	if err != nil {
		return Event{}, err
	}

	event.Type = Type(eventType)
	event.EntityType = EntityType(entityType)

	if err := json.Unmarshal(metadata, &event.Metadata); err != nil {
		return Event{}, fmt.Errorf("care event %s: decode metadata: %w", event.ID, err)
	}
	return event, nil
}

func collect(rows pgx.Rows) ([]Event, error) {
	found := make([]Event, 0)
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("scan care event: %w", err)
		}
		found = append(found, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read care events: %w", err)
	}
	return found, nil
}
