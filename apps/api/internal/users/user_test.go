package users_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/genxcare/api/internal/users"
)

func TestToResponseOmitsEmailAndNullsBlankFields(t *testing.T) {
	created := time.Date(2026, 8, 15, 9, 30, 0, 0, time.UTC)
	user := users.User{
		ID:          uuid.New(),
		AuthUserID:  uuid.New(),
		Email:       "sara@example.com",
		DisplayName: "Sara",
		AvatarURL:   "",
		Phone:       "  ",
		CreatedAt:   created,
		UpdatedAt:   created,
	}

	response := users.ToResponse(user)

	if response.ID != user.ID.String() {
		t.Errorf("ID = %q, want %q", response.ID, user.ID)
	}
	if response.AvatarURL != nil {
		t.Errorf("AvatarURL = %v, want nil", *response.AvatarURL)
	}
	if response.Phone != nil {
		t.Errorf("Phone = %v, want nil", *response.Phone)
	}
	if response.CreatedAt != "2026-08-15T09:30:00Z" {
		t.Errorf("CreatedAt = %q", response.CreatedAt)
	}

	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	// The email lives in Supabase Auth; the API must not restate it.
	if containsField(encoded, "email") || containsField(encoded, "authUserId") {
		t.Errorf("response exposes an unexpected field: %s", encoded)
	}
}

func TestToResponseKeepsPopulatedFields(t *testing.T) {
	response := users.ToResponse(users.User{
		ID:          uuid.New(),
		DisplayName: "Ahmed",
		AvatarURL:   "https://cdn.example/a.png",
		Phone:       "+92 300 1234567",
	})

	if response.AvatarURL == nil || *response.AvatarURL != "https://cdn.example/a.png" {
		t.Errorf("AvatarURL = %v", response.AvatarURL)
	}
	if response.Phone == nil || *response.Phone != "+92 300 1234567" {
		t.Errorf("Phone = %v", response.Phone)
	}
}

func TestDefaultDisplayName(t *testing.T) {
	cases := map[string]string{
		"sara@example.com":    "sara",
		" Ahmed@Example.com ": "Ahmed",
		"":                    "GenXcare member",
		"not-an-email":        "GenXcare member",
		"@example.com":        "GenXcare member",
	}

	for email, want := range cases {
		if got := users.DefaultDisplayName(email); got != want {
			t.Errorf("DefaultDisplayName(%q) = %q, want %q", email, got, want)
		}
	}
}

func containsField(encoded []byte, field string) bool {
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return false
	}
	_, present := decoded[field]
	return present
}
