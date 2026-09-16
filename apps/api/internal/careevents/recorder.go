package careevents

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/genxcare/api/internal/database"
)

// Recorder is how a domain writes an event alongside the change it describes.
//
// It exists so the transaction boundary is stated in one place rather than
// reassembled correctly in each of four domains. A task completion and its
// TASK_COMPLETED event must commit together: a completion with no event is a
// gap in the record nobody will notice, and an event with no completion is a
// timeline that lies (plans/phase7.md §§6, 26).
//
// Deliberately a transaction and nothing more. docs/12 and plans/phase7.md §23
// both rule out a message broker, and for a modular monolith writing to one
// PostgreSQL database a broker would buy nothing a transaction does not already
// give — while giving up the one guarantee that actually matters here, which is
// atomicity with the domain write.
type Recorder struct {
	pool   *database.Pool
	events *Repository
}

// NewRecorder builds a Recorder over the shared pool.
func NewRecorder(pool *database.Pool, events *Repository) *Recorder {
	return &Recorder{pool: pool, events: events}
}

// InTransaction runs fn inside one transaction.
//
// fn receives the transaction, so it can bind its own repositories to it with
// their WithTx methods, and an events repository already bound to the same
// transaction. If fn returns an error, or recording the event fails, everything
// rolls back — so a domain mutation cannot succeed while its event is lost.
func (r *Recorder) InTransaction(
	ctx context.Context,
	fn func(tx pgx.Tx, events *Repository) error,
) error {
	return database.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		return fn(tx, r.events.WithTx(tx))
	})
}
