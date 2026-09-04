// Package users owns the application user record that a Supabase identity maps
// onto, plus the `/v1/me` endpoints.
package users

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// User is the application-side account. Supabase's auth.users row stays in the
// auth system; only `AuthUserID` links the two (docs/07-database-and-sync.md).
type User struct {
	ID          uuid.UUID
	AuthUserID  uuid.UUID
	Email       string
	DisplayName string
	AvatarURL   string
	Phone       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Response is the JSON representation returned by `/v1/me`.
type Response struct {
	ID          string  `json:"id"`
	DisplayName string  `json:"displayName"`
	AvatarURL   *string `json:"avatarUrl"`
	Phone       *string `json:"phone"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   string  `json:"updatedAt"`
}

// ToResponse converts a User into its API representation.
//
// The email is intentionally omitted: it is held by Supabase Auth and the
// client already knows its own address, so the API does not restate it.
func ToResponse(user User) Response {
	return Response{
		ID:          user.ID.String(),
		DisplayName: user.DisplayName,
		AvatarURL:   optionalString(user.AvatarURL),
		Phone:       optionalString(user.Phone),
		CreatedAt:   user.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   user.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func optionalString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

// DefaultDisplayName derives a first display name for a newly created account.
//
// Supabase social sign-in supplies a name; email sign-up often does not, so the
// local part of the address is used until the user sets their own.
func DefaultDisplayName(email string) string {
	local, _, found := strings.Cut(strings.TrimSpace(email), "@")
	if found && strings.TrimSpace(local) != "" {
		return strings.TrimSpace(local)
	}
	return "GenXcare member"
}
