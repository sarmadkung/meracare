package notes_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/genxcare/api/internal/auth"
	"github.com/genxcare/api/internal/careevents"
	"github.com/genxcare/api/internal/notes"
	"github.com/genxcare/api/internal/relationships"
	"github.com/genxcare/api/internal/seniors"
	"github.com/genxcare/api/internal/testsupport"
	"github.com/genxcare/api/internal/users"
)

func TestANoteKeepsItsAuthorAndWritesActivity(t *testing.T) {
	pool := testsupport.RequireDatabase(t)
	ctx := context.Background()
	usersRepo := users.NewRepository(pool)
	user, err := usersRepo.EnsureByAuthUserID(ctx, uuid.New(), "daughter@example.com", "Daughter")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	seniorService := seniors.NewService(seniors.NewRepository(pool), relationships.NewRepository(pool))
	circle, err := seniorService.Create(ctx, auth.Principal{UserID: user.ID, AuthUserID: user.AuthUserID, Email: user.Email}, seniors.CreateInput{
		Mode: seniors.CreateModeFamily, DisplayName: "Amma", Timezone: "Asia/Karachi",
	})
	if err != nil {
		t.Fatalf("create senior: %v", err)
	}
	eventRepo := careevents.NewRepository(pool)
	service := notes.NewService(notes.NewRepository(pool), careevents.NewRecorder(pool, eventRepo))

	created, err := service.Create(ctx, circle.Senior.ID, user.ID, "Ate lunch well.")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	if created.AuthorName != "Daughter" || created.Content != "Ate lunch well." {
		t.Errorf("created note = %+v", created)
	}
	page, err := careevents.NewService(eventRepo).Activity(ctx, circle.Senior.ID, "", 10)
	if err != nil {
		t.Fatalf("read activity: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Type != careevents.TypeNoteAdded || page.Items[0].EntityID != created.ID {
		t.Fatalf("note activity = %+v", page.Items)
	}
}

func TestOnlyTheAuthorCanUpdateANote(t *testing.T) {
	pool := testsupport.RequireDatabase(t)
	ctx := context.Background()
	var seniorID, authorID uuid.UUID
	err := pool.QueryRow(ctx, `
		WITH author AS (
			INSERT INTO users (auth_user_id, display_name) VALUES (gen_random_uuid(), 'Author') RETURNING id
		), senior AS (
			INSERT INTO senior_profiles (created_by_user_id, display_name) SELECT id, 'Amma' FROM author RETURNING id
		)
		SELECT senior.id, author.id FROM senior, author`).Scan(&seniorID, &authorID)
	if err != nil {
		t.Fatalf("seed note owner: %v", err)
	}
	repo := notes.NewRepository(pool)
	created, err := repo.Create(ctx, seniorID, authorID, "Original")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	if _, err := repo.UpdateByAuthor(ctx, created.ID, uuid.New(), "Rewritten"); !errors.Is(err, notes.ErrNotFound) {
		t.Fatalf("other author update error = %v, want not found", err)
	}
	kept, err := repo.Get(ctx, created.ID)
	if err != nil || kept.Content != "Original" {
		t.Fatalf("stored note = %+v, error %v", kept, err)
	}
}
