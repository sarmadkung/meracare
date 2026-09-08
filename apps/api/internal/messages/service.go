package messages

import (
	"context"

	"github.com/google/uuid"
)

const (
	defaultPageSize = 30
	maxPageSize     = 100
)

type Service struct{ messages *Repository }

func NewService(messages *Repository) *Service { return &Service{messages: messages} }

func (s *Service) List(ctx context.Context, seniorID, readerID uuid.UUID, cursor string, limit int) (Page, error) {
	if limit <= 0 || limit > maxPageSize {
		limit = defaultPageSize
	}
	return s.messages.List(ctx, seniorID, readerID, cursor, limit)
}

func (s *Service) Create(ctx context.Context, seniorID, senderID uuid.UUID, content string) (Message, error) {
	return s.messages.Create(ctx, seniorID, senderID, content)
}

func (s *Service) MarkRead(ctx context.Context, seniorID, userID, messageID uuid.UUID) error {
	return s.messages.MarkRead(ctx, seniorID, userID, messageID)
}
