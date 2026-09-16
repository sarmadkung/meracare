// Package relationships owns the care relationship: the link between a user and
// a senior that carries a role, a permission set, and a status.
//
// Nothing else grants access to a senior's data. Every authorization decision
// in the API resolves to a relationship looked up here.
package relationships

import (
	"time"

	"github.com/google/uuid"
	"github.com/genxcare/api/internal/care"
)

// Relationship is one user's membership of one senior's care circle.
type Relationship struct {
	ID          uuid.UUID
	SeniorID    uuid.UUID
	UserID      uuid.UUID
	Role        care.Role
	Permissions care.PermissionSet
	Status      care.RelationshipStatus
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// IsActive reports whether the membership currently grants access. A pending
// invitation and a revoked membership both grant nothing.
func (r Relationship) IsActive() bool { return r.Status == care.StatusActive }

// Can reports whether this relationship permits the action.
//
// A membership that is not active can never permit anything, so an expired or
// revoked caregiver loses access without any caller having to remember to check
// the status separately.
func (r Relationship) Can(permission care.Permission) bool {
	return r.IsActive() && r.Permissions.Has(permission)
}

// Response is the JSON representation of a care-circle member.
type Response struct {
	ID          string   `json:"id"`
	SeniorID    string   `json:"seniorId"`
	UserID      string   `json:"userId"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
	Status      string   `json:"status"`
	CreatedAt   string   `json:"createdAt"`
	UpdatedAt   string   `json:"updatedAt"`
}

// ToResponse converts a relationship into its API representation.
func ToResponse(relationship Relationship) Response {
	return Response{
		ID:          relationship.ID.String(),
		SeniorID:    relationship.SeniorID.String(),
		UserID:      relationship.UserID.String(),
		Role:        string(relationship.Role),
		Permissions: relationship.Permissions.Strings(),
		Status:      string(relationship.Status),
		CreatedAt:   relationship.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   relationship.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
