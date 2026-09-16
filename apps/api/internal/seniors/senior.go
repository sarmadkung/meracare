// Package seniors owns the senior profile: the person care is coordinated for.
//
// A senior profile may or may not have an account of its own. Solo Mode is a
// user creating a profile that represents themselves; family care is a user
// creating a profile for someone else (docs/01-roles-and-care-model.md).
package seniors

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/genxcare/api/internal/relationships"
)

// Senior is a senior profile.
type Senior struct {
	ID uuid.UUID
	// UserID is the senior's own account, when they have one.
	UserID uuid.NullUUID
	// CreatedByUserID is who set the profile up. Kept distinct from the senior
	// and from circle membership so coordination can change hands later.
	CreatedByUserID uuid.UUID

	DisplayName      string
	DateOfBirth      *time.Time
	PhotoURL         string
	Phone            string
	Address          string
	EmergencyContact string

	// Timezone is an IANA name such as "Asia/Karachi". It decides when this
	// senior's day begins and ends, and so when their care is due.
	Timezone string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// DefaultTimezone is used when a profile has never been given one.
const DefaultTimezone = "UTC"

// Location resolves the senior's timezone.
//
// An unresolvable name falls back to UTC rather than failing the request:
// timezones are validated before they are stored, so reaching the fallback
// means the tzdata on this machine is older than the name, and showing care at
// the wrong hour is better than showing none at all.
func (s Senior) Location() *time.Location {
	if strings.TrimSpace(s.Timezone) == "" {
		return time.UTC
	}
	location, err := time.LoadLocation(s.Timezone)
	if err != nil {
		return time.UTC
	}
	return location
}

// ValidTimezone reports whether the name resolves on this machine.
func ValidTimezone(name string) bool {
	_, err := time.LoadLocation(strings.TrimSpace(name))
	return err == nil
}

// Response is the JSON representation of a senior profile.
type Response struct {
	ID               string  `json:"id"`
	DisplayName      string  `json:"displayName"`
	DateOfBirth      *string `json:"dateOfBirth"`
	PhotoURL         *string `json:"photoUrl"`
	Phone            *string `json:"phone"`
	Address          *string `json:"address"`
	EmergencyContact *string `json:"emergencyContact"`
	// Timezone tells the client which zone to render due times in, so a family
	// member abroad sees their relative's schedule in the relative's day.
	Timezone string `json:"timezone"`
	// IsSelf marks the profile as the caller's own, which is what the mobile
	// app uses to present Solo Mode rather than a caregiver view.
	IsSelf bool `json:"isSelf"`
	// Role and Permissions describe the caller's relationship to this senior,
	// so the client can render only the actions it is allowed to attempt.
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
	CreatedAt   string   `json:"createdAt"`
	UpdatedAt   string   `json:"updatedAt"`
}

// dateLayout is the wire format for a date with no time component.
const dateLayout = "2006-01-02"

// ToResponse renders a senior for one caller, given that caller's relationship.
//
// The caller's role and permissions travel with the profile so the client can
// render only the actions it is allowed to attempt. Nothing here is withheld by
// role: every field is care information that any member of the circle needs.
// In particular the emergency contact is shown to professional caregivers —
// they are most likely to be present when it matters, and docs/02's restriction
// concerns information unrelated to the senior's care, not who to call.
func ToResponse(senior Senior, relationship relationships.Relationship) Response {
	response := Response{
		ID:               senior.ID.String(),
		DisplayName:      senior.DisplayName,
		PhotoURL:         optionalString(senior.PhotoURL),
		Phone:            optionalString(senior.Phone),
		Address:          optionalString(senior.Address),
		EmergencyContact: optionalString(senior.EmergencyContact),
		Timezone:         senior.Timezone,
		IsSelf:           senior.UserID.Valid && senior.UserID.UUID == relationship.UserID,
		Role:             string(relationship.Role),
		Permissions:      relationship.Permissions.Strings(),
		CreatedAt:        senior.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:        senior.UpdatedAt.UTC().Format(time.RFC3339),
	}

	if senior.DateOfBirth != nil {
		formatted := senior.DateOfBirth.UTC().Format(dateLayout)
		response.DateOfBirth = &formatted
	}

	return response
}

func optionalString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

// ParseDateOfBirth reads a `YYYY-MM-DD` date.
func ParseDateOfBirth(value string) (time.Time, error) {
	return time.Parse(dateLayout, strings.TrimSpace(value))
}
