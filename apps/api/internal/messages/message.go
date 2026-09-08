// Package messages implements the one senior-scoped care-circle stream used by MVP chat.
package messages

import (
	"time"

	"github.com/google/uuid"
)

type Message struct {
	ID           uuid.UUID
	SeniorID     uuid.UUID
	SenderUserID uuid.UUID
	SenderName   string
	Content      string
	CreatedAt    time.Time
}

type Page struct {
	Items       []Message
	NextCursor  string
	UnreadCount int
}
