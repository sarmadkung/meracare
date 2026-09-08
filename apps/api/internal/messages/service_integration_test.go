package messages_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/genxcare/api/internal/messages"
	"github.com/genxcare/api/internal/testsupport"
)

func TestUnreadStateAdvancesOnlyThroughTheChosenMessage(t *testing.T) {
	pool := testsupport.RequireDatabase(t)
	ctx := context.Background()
	var seniorID, readerID, senderID uuid.UUID
	err := pool.QueryRow(ctx, `
		WITH reader AS (
			INSERT INTO users (auth_user_id, display_name) VALUES (gen_random_uuid(), 'Reader') RETURNING id
		), sender AS (
			INSERT INTO users (auth_user_id, display_name) VALUES (gen_random_uuid(), 'Sender') RETURNING id
		), senior AS (
			INSERT INTO senior_profiles (created_by_user_id, display_name) SELECT id, 'Amma' FROM reader RETURNING id
		)
		SELECT senior.id, reader.id, sender.id FROM senior, reader, sender`).Scan(&seniorID, &readerID, &senderID)
	if err != nil {
		t.Fatalf("seed conversation: %v", err)
	}
	service := messages.NewService(messages.NewRepository(pool))
	first, err := service.Create(ctx, seniorID, senderID, "I have arrived.")
	if err != nil {
		t.Fatalf("send first message: %v", err)
	}
	if _, err := service.Create(ctx, seniorID, senderID, "Lunch is ready."); err != nil {
		t.Fatalf("send second message: %v", err)
	}

	page, err := service.List(ctx, seniorID, readerID, "", 30)
	if err != nil || page.UnreadCount != 2 {
		t.Fatalf("unread page = %+v, error %v", page, err)
	}
	if err := service.MarkRead(ctx, seniorID, readerID, first.ID); err != nil {
		t.Fatalf("mark first read: %v", err)
	}
	page, err = service.List(ctx, seniorID, readerID, "", 30)
	if err != nil || page.UnreadCount != 1 {
		t.Fatalf("unread after first = %+v, error %v", page, err)
	}
	if err := service.MarkRead(ctx, seniorID, readerID, page.Items[0].ID); err != nil {
		t.Fatalf("mark latest read: %v", err)
	}
	page, err = service.List(ctx, seniorID, readerID, "", 30)
	if err != nil || page.UnreadCount != 0 {
		t.Fatalf("unread after latest = %+v, error %v", page, err)
	}
}

func TestMessagesPageWithoutRepeating(t *testing.T) {
	pool := testsupport.RequireDatabase(t)
	ctx := context.Background()
	var seniorID, senderID uuid.UUID
	if err := pool.QueryRow(ctx, `
		WITH sender AS (
			INSERT INTO users (auth_user_id, display_name) VALUES (gen_random_uuid(), 'Sender') RETURNING id
		), senior AS (
			INSERT INTO senior_profiles (created_by_user_id, display_name) SELECT id, 'Amma' FROM sender RETURNING id
		)
		SELECT senior.id, sender.id FROM senior, sender`).Scan(&seniorID, &senderID); err != nil {
		t.Fatalf("seed conversation: %v", err)
	}
	service := messages.NewService(messages.NewRepository(pool))
	for _, content := range []string{"one", "two", "three"} {
		if _, err := service.Create(ctx, seniorID, senderID, content); err != nil {
			t.Fatalf("send %q: %v", content, err)
		}
	}
	first, err := service.List(ctx, seniorID, senderID, "", 2)
	if err != nil || len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("first page = %+v, error %v", first, err)
	}
	second, err := service.List(ctx, seniorID, senderID, first.NextCursor, 2)
	if err != nil || len(second.Items) != 1 || second.NextCursor != "" {
		t.Fatalf("second page = %+v, error %v", second, err)
	}
	if first.Items[0].ID == second.Items[0].ID || first.Items[1].ID == second.Items[0].ID {
		t.Fatal("message repeated across page boundary")
	}
}
