// Package notes implements senior-scoped care notes (docs/03, CareNote).
package notes

import (
	"time"

	"github.com/google/uuid"
)

type Note struct {
	ID           uuid.UUID
	SeniorID     uuid.UUID
	AuthorUserID uuid.UUID
	AuthorName   string
	Content      string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
