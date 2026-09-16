package notes

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/genxcare/api/internal/database"
)

var ErrNotFound = errors.New("care note not found")

type Repository struct{ db database.Querier }

func NewRepository(pool *database.Pool) *Repository { return &Repository{db: pool} }
func (r *Repository) WithTx(tx pgx.Tx) *Repository  { return &Repository{db: tx} }

const noteColumns = `n.id, n.senior_id, n.author_user_id, u.display_name,
	n.content, n.created_at, n.updated_at`

func (r *Repository) Create(ctx context.Context, seniorID, authorID uuid.UUID, content string) (Note, error) {
	note, err := scanNote(r.db.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO care_notes (senior_id, author_user_id, content)
			VALUES ($1, $2, $3)
			RETURNING *
		)
		SELECT i.id, i.senior_id, i.author_user_id, u.display_name,
			i.content, i.created_at, i.updated_at
		FROM inserted i JOIN users u ON u.id = i.author_user_id`, seniorID, authorID, content))
	if err != nil {
		return Note{}, fmt.Errorf("create care note: %w", err)
	}
	return note, nil
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID) (Note, error) {
	note, err := scanNote(r.db.QueryRow(ctx, `SELECT `+noteColumns+`
		FROM care_notes n JOIN users u ON u.id = n.author_user_id WHERE n.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Note{}, ErrNotFound
	}
	if err != nil {
		return Note{}, fmt.Errorf("get care note: %w", err)
	}
	return note, nil
}

func (r *Repository) List(ctx context.Context, seniorID uuid.UUID) ([]Note, error) {
	rows, err := r.db.Query(ctx, `SELECT `+noteColumns+`
		FROM care_notes n JOIN users u ON u.id = n.author_user_id
		WHERE n.senior_id = $1 ORDER BY n.created_at DESC, n.id DESC LIMIT 100`, seniorID)
	if err != nil {
		return nil, fmt.Errorf("list care notes: %w", err)
	}
	defer rows.Close()

	items := make([]Note, 0)
	for rows.Next() {
		note, err := scanNote(rows)
		if err != nil {
			return nil, fmt.Errorf("scan care note: %w", err)
		}
		items = append(items, note)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list care notes: %w", err)
	}
	return items, nil
}

func (r *Repository) UpdateByAuthor(ctx context.Context, id, authorID uuid.UUID, content string) (Note, error) {
	note, err := scanNote(r.db.QueryRow(ctx, `
		WITH updated AS (
			UPDATE care_notes SET content = $3 WHERE id = $1 AND author_user_id = $2 RETURNING *
		)
		SELECT i.id, i.senior_id, i.author_user_id, u.display_name,
			i.content, i.created_at, i.updated_at
		FROM updated i JOIN users u ON u.id = i.author_user_id`, id, authorID, content))
	if errors.Is(err, pgx.ErrNoRows) {
		return Note{}, ErrNotFound
	}
	if err != nil {
		return Note{}, fmt.Errorf("update care note: %w", err)
	}
	return note, nil
}

type scanner interface{ Scan(...any) error }

func scanNote(row scanner) (Note, error) {
	var note Note
	err := row.Scan(&note.ID, &note.SeniorID, &note.AuthorUserID, &note.AuthorName,
		&note.Content, &note.CreatedAt, &note.UpdatedAt)
	return note, err
}
