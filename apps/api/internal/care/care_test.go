package care_test

import (
	"slices"
	"testing"

	"github.com/genxcare/api/internal/care"
)

func TestRoleValid(t *testing.T) {
	for _, role := range care.Roles {
		if !role.Valid() {
			t.Errorf("%q should be a valid role", role)
		}
	}
	for _, role := range []care.Role{"", "admin", "Senior", "care_coordinator"} {
		if role.Valid() {
			t.Errorf("%q should not be a valid role", role)
		}
	}
}

func TestRelationshipStatusValid(t *testing.T) {
	for _, status := range care.RelationshipStatuses {
		if !status.Valid() {
			t.Errorf("%q should be a valid status", status)
		}
	}
	for _, status := range []care.RelationshipStatus{"", "deleted", "Active"} {
		if status.Valid() {
			t.Errorf("%q should not be a valid status", status)
		}
	}
}

func TestDefaultPermissionsPerRole(t *testing.T) {
	// A senior manages their own care with no one else involved: Solo Mode
	// depends on the senior holding every permission for their own profile.
	senior := care.PermissionSet(care.DefaultPermissions(care.RoleSenior))
	if len(senior) != len(care.Permissions) {
		t.Errorf("senior holds %d permissions, want all %d", len(senior), len(care.Permissions))
	}

	family := care.PermissionSet(care.DefaultPermissions(care.RoleFamilyMember))
	if !family.HasAll(
		care.PermissionSeniorView,
		care.PermissionSeniorEdit,
		care.PermissionTasksManage,
		care.PermissionMembersInvite,
	) {
		t.Error("family members should be able to coordinate care and invite others")
	}
	if family.Has(care.PermissionMembersManage) {
		t.Error("family members should not administer circle membership by default")
	}

	// docs/02: professional caregivers must not automatically receive family or
	// private information, and carry out care rather than restructure it.
	professional := care.PermissionSet(care.DefaultPermissions(care.RoleProfessionalCaregiver))
	if !professional.HasAll(
		care.PermissionTasksComplete,
		care.PermissionMedicationsRecord,
		care.PermissionNotesCreate,
	) {
		t.Error("professional caregivers must be able to carry out care")
	}
	for _, forbidden := range []care.Permission{
		care.PermissionSeniorEdit,
		care.PermissionTasksManage,
		care.PermissionMedicationsManage,
		care.PermissionAppointmentsManage,
		care.PermissionMembersInvite,
		care.PermissionMembersManage,
	} {
		if professional.Has(forbidden) {
			t.Errorf("professional caregivers should not hold %q by default", forbidden)
		}
	}
}

// An unrecognised role must grant nothing, so a mistake fails closed.
func TestDefaultPermissionsUnknownRoleGrantsNothing(t *testing.T) {
	if got := care.DefaultPermissions("nonsense"); len(got) != 0 {
		t.Errorf("DefaultPermissions(unknown) = %v, want none", got)
	}
}

// Callers must not be able to mutate the shared defaults.
func TestDefaultPermissionsReturnsACopy(t *testing.T) {
	first := care.DefaultPermissions(care.RoleProfessionalCaregiver)
	original := len(first)
	first = append(first, care.PermissionSeniorEdit)
	_ = first

	if second := care.DefaultPermissions(care.RoleProfessionalCaregiver); len(second) != original {
		t.Errorf("defaults were mutated: %d permissions, want %d", len(second), original)
	}
	if slices.Contains(care.DefaultPermissions(care.RoleProfessionalCaregiver), care.PermissionSeniorEdit) {
		t.Error("appending to a returned slice leaked into the shared defaults")
	}
}

func TestPermissionSetHas(t *testing.T) {
	set := care.PermissionSet{care.PermissionTasksView, care.PermissionTasksComplete}

	if !set.Has(care.PermissionTasksView) {
		t.Error("Has should find a granted permission")
	}
	if set.Has(care.PermissionTasksManage) {
		t.Error("Has should not find a permission that was never granted")
	}
	if !set.HasAll(care.PermissionTasksView, care.PermissionTasksComplete) {
		t.Error("HasAll should accept a fully granted list")
	}
	if set.HasAll(care.PermissionTasksView, care.PermissionSeniorEdit) {
		t.Error("HasAll should reject a partially granted list")
	}
	if !set.HasAll() {
		t.Error("HasAll with no arguments should be satisfied")
	}
}

// A client cannot widen its access by inventing permission names.
func TestNormaliseDropsUnknownAndDuplicateValues(t *testing.T) {
	got := care.Normalise([]care.Permission{
		care.PermissionTasksView,
		"superuser",
		care.PermissionTasksView,
		care.PermissionSeniorView,
		"",
	})

	want := care.PermissionSet{care.PermissionSeniorView, care.PermissionTasksView}
	if !slices.Equal(got, want) {
		t.Errorf("Normalise = %v, want %v", got, want)
	}
}

func TestNormaliseSortsForComparability(t *testing.T) {
	first := care.Normalise([]care.Permission{care.PermissionTasksView, care.PermissionSeniorView})
	second := care.Normalise([]care.Permission{care.PermissionSeniorView, care.PermissionTasksView})

	if !slices.Equal(first, second) {
		t.Errorf("Normalise is order-dependent: %v != %v", first, second)
	}
}

func TestPermissionsRoundTripThroughStrings(t *testing.T) {
	original := care.Normalise(care.DefaultPermissions(care.RoleFamilyMember))

	restored := care.PermissionsFromStrings(original.Strings())

	if !slices.Equal(original, restored) {
		t.Errorf("round trip changed the set:\n got %v\nwant %v", restored, original)
	}
}

// Every default must be a recognised permission, or the database CHECK
// constraint would reject a relationship the API itself created.
func TestDefaultPermissionsAreAllRecognised(t *testing.T) {
	for _, role := range care.Roles {
		for _, permission := range care.DefaultPermissions(role) {
			if !permission.Valid() {
				t.Errorf("role %q grants unrecognised permission %q", role, permission)
			}
		}
	}
}
