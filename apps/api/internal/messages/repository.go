package messages

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/genxcare/api/internal/database"
	"github.com/genxcare/api/internal/paging"
)

var ErrNotFound = errors.New("message not found")
var ErrBadCursor = paging.ErrBadCursor

type Repository struct{ db database.Querier }

func NewRepository(pool *database.Pool) *Repository { return &Repository{db: pool} }

const messageColumns = `m.id, m.senior_id, m.sender_user_id, u.display_name,
	m.content, m.created_at`

func (r *Repository) Create(ctx context.Context, seniorID, senderID uuid.UUID, content string) (Message, error) {
	message, err := scanMessage(r.db.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO messages (senior_id, sender_user_id, content)
			VALUES ($1, $2, $3) RETURNING *
		)
		SELECT i.id, i.senior_id, i.sender_user_id, u.display_name, i.content, i.created_at
		FROM inserted i JOIN users u ON u.id = i.sender_user_id`, seniorID, senderID, content))
	if err != nil {
		return Message{}, fmt.Errorf("create message: %w", err)
	}
	return message, nil
}

func (r *Repository) List(ctx context.Context, seniorID, readerID uuid.UUID, cursor string, limit int) (Page, error) {
	at, atID, err := paging.DecodeCursor(cursor)
	if err != nil {
		return Page{}, err
	}
	rows, err := r.db.Query(ctx, `SELECT `+messageColumns+`
		FROM messages m JOIN users u ON u.id = m.sender_user_id
		WHERE m.senior_id = $1
		  AND ($2::timestamptz IS NULL OR (m.created_at, m.id) < ($2, $3))
		ORDER BY m.created_at DESC, m.id DESC LIMIT $4`, seniorID, at, atID, limit+1)
	if err != nil {
		return Page{}, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	items := make([]Message, 0, limit+1)
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return Page{}, fmt.Errorf("scan message: %w", err)
		}
		items = append(items, message)
	}
	if err := rows.Err(); err != nil {
		return Page{}, fmt.Errorf("list messages: %w", err)
	}

	var unread int
	err = r.db.QueryRow(ctx, `
		SELECT count(*)
		FROM messages m
		LEFT JOIN message_read_states s ON s.senior_id = m.senior_id AND s.user_id = $2
		WHERE m.senior_id = $1 AND m.sender_user_id <> $2
		  AND (s.last_read_at IS NULL OR (m.created_at, m.id) > (s.last_read_at, s.last_read_message_id))`,
		seniorID, readerID).Scan(&unread)
	if err != nil {
		return Page{}, fmt.Errorf("count unread messages: %w", err)
	}

	page := Page{Items: items, UnreadCount: unread}
	if len(items) > limit {
		last := items[limit-1]
		page.Items = items[:limit]
		page.NextCursor = paging.EncodeCursor(last.CreatedAt, last.ID)
	}
	return page, nil
}

func (r *Repository) MarkRead(ctx context.Context, seniorID, userID, messageID uuid.UUID) error {
	result, err := r.db.Exec(ctx, `
		INSERT INTO message_read_states (senior_id, user_id, last_read_message_id, last_read_at)
		SELECT senior_id, $2, id, created_at FROM messages WHERE senior_id = $1 AND id = $3
		ON CONFLICT (senior_id, user_id) DO UPDATE SET
			last_read_message_id = EXCLUDED.last_read_message_id,
			last_read_at = EXCLUDED.last_read_at
		WHERE message_read_states.last_read_at IS NULL
		   OR (message_read_states.last_read_at, message_read_states.last_read_message_id)
		      < (EXCLUDED.last_read_at, EXCLUDED.last_read_message_id)`, seniorID, userID, messageID)
	if err != nil {
		return fmt.Errorf("mark messages read: %w", err)
	}
	if result.RowsAffected() == 0 {
		// A no-op can be an already-newer position or an unknown message. Check
		// existence without letting a caller distinguish another circle's id.
		var exists bool
		if err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM messages WHERE senior_id = $1 AND id = $2)`, seniorID, messageID).Scan(&exists); err != nil {
			return fmt.Errorf("check message read position: %w", err)
		}
		if !exists {
			return ErrNotFound
		}
	}
	return nil
}

type scanner interface{ Scan(...any) error }

func scanMessage(row scanner) (Message, error) {
	var message Message
	err := row.Scan(&message.ID, &message.SeniorID, &message.SenderUserID,
		&message.SenderName, &message.Content, &message.CreatedAt)
	return message, err
}
