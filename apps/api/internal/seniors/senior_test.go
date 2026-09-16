package seniors_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/genxcare/api/internal/care"
	"github.com/genxcare/api/internal/relationships"
	"github.com/genxcare/api/internal/seniors"
)

func relationshipFor(role care.Role, userID uuid.UUID) relationships.Relationship {
	return relationships.Relationship{
		ID:          uuid.New(),
		UserID:      userID,
		Role:        role,
		Permissions: care.Normalise(care.DefaultPermissions(role)),
		Status:      care.StatusActive,
	}
}

func TestToResponseMarksTheCallersOwnProfile(t *testing.T) {
	userID := uuid.New()
	senior := seniors.Senior{
		ID:          uuid.New(),
		UserID:      uuid.NullUUID{UUID: userID, Valid: true},
		DisplayName: "Ahmed",
	}

	// Solo Mode: the caller is the senior.
	if got := seniors.ToResponse(senior, relationshipFor(care.RoleSenior, userID)); !got.IsSelf {
		t.Error("IsSelf = false for the caller's own profile")
	}

	// A daughter viewing her father's profile is not the senior.
	if got := seniors.ToResponse(senior, relationshipFor(care.RoleFamilyMember, uuid.New())); got.IsSelf {
		t.Error("IsSelf = true for someone else's profile")
	}

	// A managed profile with no account is nobody's own.
	managed := seniors.Senior{ID: uuid.New(), DisplayName: "Mrs Khan"}
	if got := seniors.ToResponse(managed, relationshipFor(care.RoleFamilyMember, userID)); got.IsSelf {
		t.Error("IsSelf = true for a profile with no linked account")
	}
}

// The emergency contact reaches every member of the circle. A professional
// caregiver is the person most likely to be present when it is needed.
func TestToResponseShowsEmergencyContactToEveryMember(t *testing.T) {
	senior := seniors.Senior{
		ID:               uuid.New(),
		DisplayName:      "Mrs Khan",
		EmergencyContact: "Sara — 0300 1234567",
	}

	for _, role := range care.Roles {
		response := seniors.ToResponse(senior, relationshipFor(role, uuid.New()))
		if response.EmergencyContact == nil || *response.EmergencyContact != "Sara — 0300 1234567" {
			t.Errorf("role %q should see the emergency contact, got %v", role, response.EmergencyContact)
		}
	}
}

func TestToResponseCarriesTheCallersOwnCapabilities(t *testing.T) {
	senior := seniors.Senior{ID: uuid.New(), DisplayName: "Mrs Khan"}

	response := seniors.ToResponse(senior, relationshipFor(care.RoleProfessionalCaregiver, uuid.New()))

	if response.Role != string(care.RoleProfessionalCaregiver) {
		t.Errorf("Role = %q, want professional_caregiver", response.Role)
	}
	// The client renders actions from this list, so it must reflect the
	// caller's real permissions rather than the role name alone.
	permissions := care.PermissionsFromStrings(response.Permissions)
	if !permissions.Has(care.PermissionTasksComplete) {
		t.Error("expected tasks.complete in the caller's permissions")
	}
	if permissions.Has(care.PermissionSeniorEdit) {
		t.Error("caregiver response should not advertise senior.edit")
	}
}

func TestToResponseFormatsOptionalFields(t *testing.T) {
	dateOfBirth := time.Date(1948, 3, 7, 0, 0, 0, 0, time.UTC)
	senior := seniors.Senior{
		ID:          uuid.New(),
		DisplayName: "Mrs Khan",
		DateOfBirth: &dateOfBirth,
		Phone:       "  ",
		Address:     "12 Garden Road",
		CreatedAt:   time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC),
	}

	response := seniors.ToResponse(senior, relationshipFor(care.RoleFamilyMember, uuid.New()))

	if response.DateOfBirth == nil || *response.DateOfBirth != "1948-03-07" {
		t.Errorf("DateOfBirth = %v, want 1948-03-07", response.DateOfBirth)
	}
	if response.Phone != nil {
		t.Errorf("blank phone should serialise as null, got %q", *response.Phone)
	}
	if response.Address == nil || *response.Address != "12 Garden Road" {
		t.Errorf("Address = %v", response.Address)
	}
	if response.CreatedAt != "2026-08-15T09:00:00Z" {
		t.Errorf("CreatedAt = %q", response.CreatedAt)
	}
}

func TestCreateModeValid(t *testing.T) {
	for _, mode := range []seniors.CreateMode{
		seniors.CreateModeSelf,
		seniors.CreateModeFamily,
		seniors.CreateModeProfessional,
	} {
		if !mode.Valid() {
			t.Errorf("%q should be a valid mode", mode)
		}
	}
	for _, mode := range []seniors.CreateMode{"", "senior", "admin", "Self"} {
		if mode.Valid() {
			t.Errorf("%q should not be a valid mode", mode)
		}
	}
}

func TestParseDateOfBirth(t *testing.T) {
	parsed, err := seniors.ParseDateOfBirth(" 1948-03-07 ")
	if err != nil {
		t.Fatalf("ParseDateOfBirth: %v", err)
	}
	if parsed.Year() != 1948 || parsed.Month() != time.March || parsed.Day() != 7 {
		t.Errorf("parsed = %v", parsed)
	}

	for _, invalid := range []string{"07/03/1948", "1948-3-7", "not a date", ""} {
		if _, err := seniors.ParseDateOfBirth(invalid); err == nil {
			t.Errorf("ParseDateOfBirth(%q) should fail", invalid)
		}
	}
}
