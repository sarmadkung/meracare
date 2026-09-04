package care_test

import (
	"slices"
	"testing"

	"github.com/genxcare/api/internal/care"
)

func TestCanInviteRole(t *testing.T) {
	for _, role := range []care.Role{care.RoleFamilyMember, care.RoleProfessionalCaregiver} {
		if !care.CanInviteRole(role) {
			t.Errorf("%q should be invitable", role)
		}
	}
	// The senior's seat is established with the profile, never handed out.
	if care.CanInviteRole(care.RoleSenior) {
		t.Error("the senior role must not be invitable")
	}
	if care.CanInviteRole("owner") {
		t.Error("an unrecognised role must not be invitable")
	}
}

func TestDelegateGrantsWhatTheGranterHolds(t *testing.T) {
	granter := care.Normalise(care.DefaultPermissions(care.RoleFamilyMember))

	result := care.Delegate(granter, care.RoleFamilyMember, []care.Permission{
		care.PermissionSeniorView,
		care.PermissionTasksView,
	})

	if !result.OK() {
		t.Fatalf("refused %v, want none", result.Refused)
	}
	want := care.PermissionSet{care.PermissionSeniorView, care.PermissionTasksView}
	if !slices.Equal(result.Granted, want) {
		t.Errorf("granted = %v, want %v", result.Granted, want)
	}
}

// The escalation the spec calls out: a member granting rights they lack.
func TestDelegateRefusesPermissionsTheGranterLacks(t *testing.T) {
	// A caregiver holds neither members.manage nor senior.edit.
	granter := care.Normalise(care.DefaultPermissions(care.RoleProfessionalCaregiver))

	result := care.Delegate(granter, care.RoleFamilyMember, []care.Permission{
		care.PermissionTasksView,
		care.PermissionMembersManage,
		care.PermissionSeniorEdit,
	})

	if result.OK() {
		t.Fatal("delegation should have been refused")
	}
	want := care.PermissionSet{care.PermissionMembersManage, care.PermissionSeniorEdit}
	if !slices.Equal(result.Refused, want) {
		t.Errorf("refused = %v, want %v", result.Refused, want)
	}
	// The refusal is reported rather than silently narrowed, so the caller
	// fails the request instead of creating a weaker membership than asked for.
	if !slices.Contains(result.Granted, care.PermissionTasksView) {
		t.Error("permissions the granter does hold should still be reported as grantable")
	}
}

// A member must not confer more than they hold merely by omitting the set.
func TestDelegateNarrowsRoleDefaultsToTheGranter(t *testing.T) {
	// This caregiver has been given the ability to invite, but nothing else
	// beyond the caregiver defaults.
	granter := care.Normalise(append(
		care.DefaultPermissions(care.RoleProfessionalCaregiver),
		care.PermissionMembersInvite,
	))

	granted := care.DelegateDefaults(granter, care.RoleFamilyMember)

	// family_member defaults include senior.edit and tasks.manage, which this
	// granter does not hold.
	for _, forbidden := range []care.Permission{
		care.PermissionSeniorEdit,
		care.PermissionTasksManage,
		care.PermissionMedicationsManage,
		care.PermissionAppointmentsManage,
	} {
		if slices.Contains(granted, forbidden) {
			t.Errorf("defaults leaked %q, which the granter does not hold", forbidden)
		}
	}
	// What they do hold still passes through.
	if !slices.Contains(granted, care.PermissionTasksView) {
		t.Error("expected tasks.view to be delegated")
	}
}

func TestDelegateUsesRoleDefaultsWhenNothingIsRequested(t *testing.T) {
	// The senior holds everything, so the defaults survive intact.
	granter := care.Normalise(care.DefaultPermissions(care.RoleSenior))

	granted := care.DelegateDefaults(granter, care.RoleProfessionalCaregiver)

	want := care.Normalise(care.DefaultPermissions(care.RoleProfessionalCaregiver))
	if !slices.Equal(granted, want) {
		t.Errorf("granted = %v, want the caregiver defaults %v", granted, want)
	}
}

// An invented permission name must not be treated as something the granter
// could satisfy, nor stored.
func TestDelegateDropsUnrecognisedPermissions(t *testing.T) {
	granter := care.Normalise(care.DefaultPermissions(care.RoleSenior))

	result := care.Delegate(granter, care.RoleFamilyMember, []care.Permission{
		"owner",
		"*",
		care.PermissionTasksView,
	})

	if !result.OK() {
		t.Fatalf("refused %v; unrecognised names should be dropped, not refused", result.Refused)
	}
	if !slices.Equal(result.Granted, care.PermissionSet{care.PermissionTasksView}) {
		t.Errorf("granted = %v, want only tasks.view", result.Granted)
	}
}

// A granter with nothing can confer nothing, even asking for defaults.
func TestDelegateFromAnEmptyGranterGrantsNothing(t *testing.T) {
	granted := care.DelegateDefaults(care.PermissionSet{}, care.RoleFamilyMember)

	if len(granted) != 0 {
		t.Errorf("granted = %v, want none", granted)
	}
}

// Delegation can never widen a set: whatever comes out is a subset of the
// granter's own permissions, for every role and every request.
func TestDelegateNeverExceedsTheGranter(t *testing.T) {
	for _, granterRole := range care.Roles {
		granter := care.Normalise(care.DefaultPermissions(granterRole))

		for _, targetRole := range care.Roles {
			// Ask for everything the domain defines.
			result := care.Delegate(granter, targetRole, care.Permissions)

			for _, granted := range result.Granted {
				if !granter.Has(granted) {
					t.Errorf("%s granting to %s: leaked %q", granterRole, targetRole, granted)
				}
			}
		}
	}
}
