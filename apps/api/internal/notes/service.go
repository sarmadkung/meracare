package notes

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/genxcare/api/internal/careevents"
)

type Service struct {
	notes  *Repository
	events *careevents.Recorder
}

func NewService(notes *Repository, events *careevents.Recorder) *Service {
	return &Service{notes: notes, events: events}
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Note, error) {
	return s.notes.Get(ctx, id)
}

func (s *Service) List(ctx context.Context, seniorID uuid.UUID) ([]Note, error) {
	return s.notes.List(ctx, seniorID)
}

func (s *Service) Create(ctx context.Context, seniorID, authorID uuid.UUID, content string) (Note, error) {
	var note Note
	err := s.events.InTransaction(ctx, func(tx pgx.Tx, events *careevents.Repository) error {
		created, err := s.notes.WithTx(tx).Create(ctx, seniorID, authorID, content)
		if err != nil {
			return err
		}
		note = created
		_, err = events.Record(ctx, careevents.RecordParams{
			SeniorID: seniorID, ActorUserID: &authorID, Type: careevents.TypeNoteAdded,
			EntityType: careevents.EntityNote, EntityID: created.ID,
		})
		return err
	})
	return note, err
}

func (s *Service) Update(ctx context.Context, id, authorID uuid.UUID, content string) (Note, error) {
	return s.notes.UpdateByAuthor(ctx, id, authorID, content)
}
