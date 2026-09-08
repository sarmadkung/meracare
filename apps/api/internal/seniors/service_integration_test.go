package seniors_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/genxcare/api/internal/auth"
	"github.com/genxcare/api/internal/care"
	"github.com/genxcare/api/internal/database"
	"github.com/genxcare/api/internal/relationships"
	"github.com/genxcare/api/internal/seniors"
	"github.com/genxcare/api/internal/testsupport"
	"github.com/genxcare/api/internal/users"
)

// These tests exercise real SQL, including the constraints that back
// authorization. Skipped unless TEST_DATABASE_URL is set.

type fixture struct {
	pool          *database.Pool
	service       *seniors.Service
	seniors       *seniors.Repository
	relationships *relationships.Repository
	users         *users.Repository
}

func newFixture(t *testing.T) fixture {
	t.Helper()

	pool := testsupport.RequireDatabase(t)
	seniorRepo := seniors.NewRepository(pool)
	relationshipRepo := relationships.NewRepository(pool)

	return fixture{
		pool:          pool,
		service:       seniors.NewService(seniorRepo, relationshipRepo),
		seniors:       seniorRepo,
		relationships: relationshipRepo,
		users:         users.NewRepository(pool),
	}
}

// newUser creates an application user and returns the principal for it.
func (f fixture) newUser(t *testing.T, email string) auth.Principal {
	t.Helper()

	user, err := f.users.EnsureByAuthUserID(context.Background(), uuid.New(), email, users.DefaultDisplayName(email))
	if err != nil {
		t.Fatalf("create user %s: %v", email, err)
	}
	return auth.Principal{UserID: user.ID, AuthUserID: user.AuthUserID, Email: user.Email}
}

// Solo Mode: a user creates a profile for themselves and needs nobody else.
func TestCreateSoloProfile(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	principal := f.newUser(t, "solo@example.com")

	membership, err := f.service.Create(ctx, principal, seniors.CreateInput{
		Mode:        seniors.CreateModeSelf,
		DisplayName: "Ahmed Khan",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if !membership.Senior.UserID.Valid || membership.Senior.UserID.UUID != principal.UserID {
		t.Error("a self profile should link to the creator's account")
	}
	if membership.Senior.CreatedByUserID != principal.UserID {
		t.Error("created_by should record the creator")
	}
	if membership.Relationship.Role != care.RoleSenior {
		t.Errorf("role = %q, want senior", membership.Relationship.Role)
	}
	if !membership.Relationship.IsActive() {
		t.Error("a self-created relationship should be active immediately, with no invitation")
	}
	// The senior runs their own care circle, which is what makes Solo Mode work.
	if !membership.Relationship.Can(care.PermissionTasksManage) ||
		!membership.Relationship.Can(care.PermissionSeniorEdit) {
		t.Error("a solo senior should be able to manage their own care")
	}
}

func TestCreateFamilyProfileIsNotLinkedToAnAccount(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	principal := f.newUser(t, "daughter@example.com")

	dateOfBirth := time.Date(1948, 3, 7, 0, 0, 0, 0, time.UTC)
	membership, err := f.service.Create(ctx, principal, seniors.CreateInput{
		Mode:             seniors.CreateModeFamily,
		DisplayName:      "Mrs Khan",
		DateOfBirth:      &dateOfBirth,
		Phone:            "0300 1234567",
		EmergencyContact: "Sara — 0300 7654321",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// The senior has no account yet; she may be invited later.
	if membership.Senior.UserID.Valid {
		t.Error("a profile created for someone else must not link to the creator's account")
	}
	if membership.Relationship.Role != care.RoleFamilyMember {
		t.Errorf("role = %q, want family_member", membership.Relationship.Role)
	}
	if membership.Senior.DateOfBirth == nil || !membership.Senior.DateOfBirth.Equal(dateOfBirth) {
		t.Errorf("DateOfBirth = %v, want %v", membership.Senior.DateOfBirth, dateOfBirth)
	}
}

// A professional caregiver manages many seniors from one account.
func TestProfessionalCaregiverManagesMultipleSeniors(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	caregiver := f.newUser(t, "maria@example.com")

	for _, name := range []string{"Mrs Khan", "Mr Ahmed", "Mrs Ali"} {
		if _, err := f.service.Create(ctx, caregiver, seniors.CreateInput{
			Mode:        seniors.CreateModeProfessional,
			DisplayName: name,
		}); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}

	memberships, err := f.service.List(ctx, caregiver)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(memberships) != 3 {
		t.Fatalf("caregiver sees %d seniors, want 3", len(memberships))
	}
	for _, membership := range memberships {
		if membership.Relationship.Role != care.RoleProfessionalCaregiver {
			t.Errorf("role = %q, want professional_caregiver", membership.Relationship.Role)
		}
		// This professional created each client and is therefore its initial
		// coordinator. Invited professionals retain the narrower role defaults.
		if !membership.Relationship.Can(care.PermissionSeniorEdit) {
			t.Error("a professional client creator should be able to complete setup")
		}
	}
}

// The central authorization property: a user sees only their own seniors.
func TestListReturnsOnlySeniorsTheUserIsConnectedTo(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sara := f.newUser(t, "sara@example.com")
	stranger := f.newUser(t, "stranger@example.com")

	if _, err := f.service.Create(ctx, sara, seniors.CreateInput{
		Mode:        seniors.CreateModeFamily,
		DisplayName: "Mrs Khan",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	strangerSees, err := f.service.List(ctx, stranger)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(strangerSees) != 0 {
		t.Fatalf("an unconnected user sees %d seniors, want 0", len(strangerSees))
	}

	saraSees, err := f.service.List(ctx, sara)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(saraSees) != 1 {
		t.Fatalf("Sara sees %d seniors, want 1", len(saraSees))
	}
}

// Family and professional caregivers coordinating around the same senior is
// the mixed-care mode from docs/00.
func TestMixedCareCircle(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sara := f.newUser(t, "sara-mixed@example.com")
	maria := f.newUser(t, "maria-mixed@example.com")

	membership, err := f.service.Create(ctx, sara, seniors.CreateInput{
		Mode:        seniors.CreateModeFamily,
		DisplayName: "Mrs Khan",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Phase 3 adds invitations; here the membership is created directly.
	if _, err := f.relationships.Create(ctx, relationships.CreateParams{
		SeniorID:    membership.Senior.ID,
		UserID:      maria.UserID,
		Role:        care.RoleProfessionalCaregiver,
		Permissions: care.Normalise(care.DefaultPermissions(care.RoleProfessionalCaregiver)),
		Status:      care.StatusActive,
	}); err != nil {
		t.Fatalf("add caregiver: %v", err)
	}

	circle, err := f.relationships.ListForSenior(ctx, membership.Senior.ID)
	if err != nil {
		t.Fatalf("ListForSenior: %v", err)
	}
	if len(circle) != 2 {
		t.Fatalf("care circle has %d members, want 2", len(circle))
	}

	// Both reach the same senior, with different capabilities.
	mariaSees, err := f.service.List(ctx, maria)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(mariaSees) != 1 || mariaSees[0].Senior.ID != membership.Senior.ID {
		t.Fatal("the caregiver should reach the shared senior")
	}
	if mariaSees[0].Relationship.Can(care.PermissionSeniorEdit) {
		t.Error("the caregiver should not be able to edit the profile")
	}
}

// A revoked member loses access without their care history being deleted.
func TestRevokedMemberLosesAccess(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sara := f.newUser(t, "sara-revoke@example.com")
	maria := f.newUser(t, "maria-revoke@example.com")

	membership, err := f.service.Create(ctx, sara, seniors.CreateInput{
		Mode:        seniors.CreateModeFamily,
		DisplayName: "Mrs Khan",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	relationship, err := f.relationships.Create(ctx, relationships.CreateParams{
		SeniorID:    membership.Senior.ID,
		UserID:      maria.UserID,
		Role:        care.RoleProfessionalCaregiver,
		Permissions: care.Normalise(care.DefaultPermissions(care.RoleProfessionalCaregiver)),
		Status:      care.StatusRevoked,
	})
	if err != nil {
		t.Fatalf("create revoked relationship: %v", err)
	}

	// The row still exists, so past care events keep their author...
	found, err := f.relationships.FindByUserAndSenior(ctx, maria.UserID, membership.Senior.ID)
	if err != nil {
		t.Fatalf("FindByUserAndSenior: %v", err)
	}
	if found.ID != relationship.ID {
		t.Error("the revoked relationship should still be retrievable")
	}
	// ...but it grants nothing.
	if found.Can(care.PermissionSeniorView) {
		t.Error("a revoked membership must not grant access")
	}

	sees, err := f.service.List(ctx, maria)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sees) != 0 {
		t.Errorf("a revoked member sees %d seniors, want 0", len(sees))
	}
}

func TestFindByUserAndSeniorReturnsErrNotFoundForStrangers(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sara := f.newUser(t, "sara-stranger@example.com")
	stranger := f.newUser(t, "stranger2@example.com")

	membership, err := f.service.Create(ctx, sara, seniors.CreateInput{
		Mode:        seniors.CreateModeFamily,
		DisplayName: "Mrs Khan",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = f.relationships.FindByUserAndSenior(ctx, stranger.UserID, membership.Senior.ID)
	if !errors.Is(err, relationships.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

// One account cannot hold two Solo Mode profiles.
func TestCreateSecondSelfProfileIsRejected(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	principal := f.newUser(t, "solo-twice@example.com")

	if _, err := f.service.Create(ctx, principal, seniors.CreateInput{
		Mode:        seniors.CreateModeSelf,
		DisplayName: "Ahmed",
	}); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	_, err := f.service.Create(ctx, principal, seniors.CreateInput{
		Mode:        seniors.CreateModeSelf,
		DisplayName: "Ahmed again",
	})
	if err == nil {
		t.Fatal("a second self profile should be rejected")
	}
}

// The profile and the creator's membership are committed together, so a failed
// creation leaves no orphan profile that nobody can reach.
func TestFailedCreateLeavesNoOrphanProfile(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	principal := f.newUser(t, "orphan@example.com")

	if _, err := f.service.Create(ctx, principal, seniors.CreateInput{
		Mode:        seniors.CreateModeSelf,
		DisplayName: "Ahmed",
	}); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	before, err := f.service.List(ctx, principal)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	// This fails on the unique self-profile constraint, after the profile row
	// has been inserted inside the transaction.
	if _, err := f.service.Create(ctx, principal, seniors.CreateInput{
		Mode:        seniors.CreateModeSelf,
		DisplayName: "Duplicate",
	}); err == nil {
		t.Fatal("expected the duplicate self profile to fail")
	}

	after, err := f.service.List(ctx, principal)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(after) != len(before) {
		t.Errorf("senior count changed from %d to %d after a failed create", len(before), len(after))
	}
}

func TestUpdateAppliesOnlySuppliedFields(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	principal := f.newUser(t, "update@example.com")

	dateOfBirth := time.Date(1948, 3, 7, 0, 0, 0, 0, time.UTC)
	membership, err := f.service.Create(ctx, principal, seniors.CreateInput{
		Mode:             seniors.CreateModeFamily,
		DisplayName:      "Mrs Khan",
		DateOfBirth:      &dateOfBirth,
		Phone:            "0300 1234567",
		EmergencyContact: "Sara",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	address := "12 Garden Road"
	updated, err := f.service.Update(ctx, membership.Senior.ID, seniors.UpdateParams{Address: &address})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if updated.Address != address {
		t.Errorf("Address = %q, want %q", updated.Address, address)
	}
	if updated.DisplayName != "Mrs Khan" {
		t.Errorf("DisplayName = %q, want it unchanged", updated.DisplayName)
	}
	if updated.Phone != "0300 1234567" {
		t.Errorf("Phone = %q, want it unchanged", updated.Phone)
	}
	if updated.DateOfBirth == nil {
		t.Error("DateOfBirth should be unchanged, not cleared")
	}

	// An explicit clear removes the value.
	blank := ""
	cleared, err := f.service.Update(ctx, membership.Senior.ID, seniors.UpdateParams{
		Phone:            &blank,
		ClearDateOfBirth: true,
	})
	if err != nil {
		t.Fatalf("Update (clear): %v", err)
	}
	if cleared.Phone != "" {
		t.Errorf("Phone = %q, want cleared", cleared.Phone)
	}
	if cleared.DateOfBirth != nil {
		t.Errorf("DateOfBirth = %v, want cleared", cleared.DateOfBirth)
	}
	if cleared.Address != address {
		t.Errorf("Address = %q, want it unchanged", cleared.Address)
	}
}

func TestUpdateReturnsErrNotFoundForUnknownSenior(t *testing.T) {
	f := newFixture(t)

	name := "Ghost"
	_, err := f.service.Update(context.Background(), uuid.New(), seniors.UpdateParams{DisplayName: &name})
	if !errors.Is(err, seniors.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

// The database refuses a permission the domain does not define, whatever the
// application layer does.
func TestDatabaseRejectsUnrecognisedPermission(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	principal := f.newUser(t, "constraint@example.com")

	membership, err := f.service.Create(ctx, principal, seniors.CreateInput{
		Mode:        seniors.CreateModeFamily,
		DisplayName: "Mrs Khan",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	other := f.newUser(t, "constraint-other@example.com")
	_, err = f.relationships.Create(ctx, relationships.CreateParams{
		SeniorID: membership.Senior.ID,
		UserID:   other.UserID,
		Role:     care.RoleFamilyMember,
		// Normalise would drop this, so it is written past the domain layer
		// deliberately to prove the CHECK constraint is real.
		Permissions: care.PermissionSet{"superuser"},
		Status:      care.StatusActive,
	})
	if err != nil {
		// Normalise dropped it, which is also correct — the stored set must
		// simply never contain the invented permission.
		t.Logf("create rejected: %v", err)
		return
	}

	stored, err := f.relationships.FindByUserAndSenior(ctx, other.UserID, membership.Senior.ID)
	if err != nil {
		t.Fatalf("FindByUserAndSenior: %v", err)
	}
	if len(stored.Permissions) != 0 {
		t.Errorf("stored permissions = %v, want none", stored.Permissions)
	}
}

// A care circle has exactly one senior.
func TestOnlyOneSeniorPerCircle(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	owner := f.newUser(t, "circle-owner@example.com")
	other := f.newUser(t, "circle-other@example.com")

	membership, err := f.service.Create(ctx, owner, seniors.CreateInput{
		Mode:        seniors.CreateModeSelf,
		DisplayName: "Ahmed",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = f.relationships.Create(ctx, relationships.CreateParams{
		SeniorID:    membership.Senior.ID,
		UserID:      other.UserID,
		Role:        care.RoleSenior,
		Permissions: care.Normalise(care.DefaultPermissions(care.RoleSenior)),
		Status:      care.StatusActive,
	})
	if err == nil {
		t.Fatal("a second senior in the same circle should be rejected")
	}
}

// A user holds at most one membership per senior.
func TestDuplicateMembershipIsRejected(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sara := f.newUser(t, "dupe-sara@example.com")
	maria := f.newUser(t, "dupe-maria@example.com")

	membership, err := f.service.Create(ctx, sara, seniors.CreateInput{
		Mode:        seniors.CreateModeFamily,
		DisplayName: "Mrs Khan",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	params := relationships.CreateParams{
		SeniorID:    membership.Senior.ID,
		UserID:      maria.UserID,
		Role:        care.RoleProfessionalCaregiver,
		Permissions: care.Normalise(care.DefaultPermissions(care.RoleProfessionalCaregiver)),
		Status:      care.StatusActive,
	}
	if _, err := f.relationships.Create(ctx, params); err != nil {
		t.Fatalf("first membership: %v", err)
	}
	if _, err := f.relationships.Create(ctx, params); err == nil {
		t.Fatal("a duplicate membership should be rejected")
	}
}

// The same person can hold different roles in different circles — the property
// that makes a global is_caregiver flag wrong (docs/02).
func TestUserHoldsDifferentRolesInDifferentCircles(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	person := f.newUser(t, "multi-role@example.com")

	own, err := f.service.Create(ctx, person, seniors.CreateInput{
		Mode:        seniors.CreateModeSelf,
		DisplayName: "Their own profile",
	})
	if err != nil {
		t.Fatalf("create self: %v", err)
	}
	parent, err := f.service.Create(ctx, person, seniors.CreateInput{
		Mode:        seniors.CreateModeFamily,
		DisplayName: "Their mother",
	})
	if err != nil {
		t.Fatalf("create family: %v", err)
	}
	client, err := f.service.Create(ctx, person, seniors.CreateInput{
		Mode:        seniors.CreateModeProfessional,
		DisplayName: "Their client",
	})
	if err != nil {
		t.Fatalf("create professional: %v", err)
	}

	wantRoles := map[uuid.UUID]care.Role{
		own.Senior.ID:    care.RoleSenior,
		parent.Senior.ID: care.RoleFamilyMember,
		client.Senior.ID: care.RoleProfessionalCaregiver,
	}

	memberships, err := f.service.List(ctx, person)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(memberships) != 3 {
		t.Fatalf("saw %d seniors, want 3", len(memberships))
	}
	for _, membership := range memberships {
		want := wantRoles[membership.Senior.ID]
		if membership.Relationship.Role != want {
			t.Errorf("senior %s: role = %q, want %q",
				membership.Senior.DisplayName, membership.Relationship.Role, want)
		}
	}

	// The same account can fully set up the client it created and its own care.
	for _, membership := range memberships {
		canEdit := membership.Relationship.Can(care.PermissionSeniorEdit)
		if membership.Senior.ID == client.Senior.ID && !canEdit {
			t.Error("the professional who created a client should be able to edit it")
		}
		if membership.Senior.ID == own.Senior.ID && !canEdit {
			t.Error("should be able to edit their own profile")
		}
	}
}

func TestTheCreatorOfACircleCanAdministerIt(t *testing.T) {
	// A family circle used to have nobody who could ever revoke anybody. The
	// daughter who sets it up is a family member, and members.manage is
	// deliberately not a family-member default; the mother has no account to
	// hold it; and an invitation can only delegate what the inviter already
	// has. So a caregiver's access to somebody's medical information could be
	// granted and never withdrawn (plans/phase9.md §§5, 40).
	//
	// Whoever creates a circle therefore holds members.manage, in every mode.
	f := newFixture(t)
	ctx := context.Background()

	for _, testCase := range []struct {
		mode seniors.CreateMode
		role care.Role
	}{
		{seniors.CreateModeSelf, care.RoleSenior},
		{seniors.CreateModeFamily, care.RoleFamilyMember},
		{seniors.CreateModeProfessional, care.RoleProfessionalCaregiver},
	} {
		t.Run(string(testCase.mode), func(t *testing.T) {
			principal := f.newUser(t, string(testCase.mode)+"-creator@example.com")

			membership, err := f.service.Create(ctx, principal, seniors.CreateInput{
				Mode:        testCase.mode,
				DisplayName: "Amma",
			})
			if err != nil {
				t.Fatalf("Create: %v", err)
			}

			if membership.Relationship.Role != testCase.role {
				t.Errorf("role = %q, want %q", membership.Relationship.Role, testCase.role)
			}
			if !membership.Relationship.Can(care.PermissionMembersManage) {
				t.Errorf("the creator cannot administer the circle they created: %v",
					membership.Relationship.Permissions.Strings())
			}
			if testCase.mode == seniors.CreateModeProfessional {
				for _, permission := range []care.Permission{
					care.PermissionSeniorEdit, care.PermissionTasksManage,
					care.PermissionMedicationsManage, care.PermissionAppointmentsManage,
					care.PermissionMembersInvite,
				} {
					if !membership.Relationship.Can(permission) {
						t.Errorf("professional creator missing %q", permission)
					}
				}
			}
		})
	}
}

func TestRemovingAManagedProfileDeletesEmptyAndArchivesHistory(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	owner := f.newUser(t, "profile-removal@example.com")

	empty, err := f.service.Create(ctx, owner, seniors.CreateInput{
		Mode: seniors.CreateModeFamily, DisplayName: "Wrong person",
	})
	if err != nil {
		t.Fatalf("create empty: %v", err)
	}
	disposition, err := f.service.Remove(ctx, empty.Senior.ID, owner.UserID)
	if err != nil || disposition != seniors.RemovalDeleted {
		t.Fatalf("remove empty = %q, %v", disposition, err)
	}
	if _, err := f.seniors.GetByID(ctx, empty.Senior.ID); !errors.Is(err, seniors.ErrNotFound) {
		t.Fatalf("deleted profile GetByID error = %v", err)
	}

	active, err := f.service.Create(ctx, owner, seniors.CreateInput{
		Mode: seniors.CreateModeFamily, DisplayName: "Mrs Khan",
	})
	if err != nil {
		t.Fatalf("create active: %v", err)
	}
	_, err = f.pool.Exec(ctx, `INSERT INTO care_events
		(senior_id, actor_user_id, event_type, entity_type, entity_id)
		VALUES ($1, $2, 'MEMBER_REVOKED', 'relationship', $3)`,
		active.Senior.ID, owner.UserID, active.Relationship.ID)
	if err != nil {
		t.Fatalf("add history: %v", err)
	}
	disposition, err = f.service.Remove(ctx, active.Senior.ID, owner.UserID)
	if err != nil || disposition != seniors.RemovalArchived {
		t.Fatalf("remove with history = %q, %v", disposition, err)
	}
	listed, err := f.service.List(ctx, owner)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("archived profile remains listed: %d", len(listed))
	}
	var archived bool
	if err := f.pool.QueryRow(ctx, `SELECT archived_at IS NOT NULL FROM senior_profiles WHERE id=$1`, active.Senior.ID).Scan(&archived); err != nil || !archived {
		t.Fatalf("archived row = %v, %v", archived, err)
	}
}

func TestSelfProfileCannotUseManagedProfileRemoval(t *testing.T) {
	f := newFixture(t)
	owner := f.newUser(t, "self-removal@example.com")
	profile, err := f.service.Create(context.Background(), owner, seniors.CreateInput{
		Mode: seniors.CreateModeSelf, DisplayName: "Me",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = f.service.Remove(context.Background(), profile.Senior.ID, owner.UserID)
	if !errors.Is(err, seniors.ErrSelfProfileRemoval) {
		t.Fatalf("Remove error = %v", err)
	}
}

func TestCircleAdministrationIsNotAFamilyMemberDefault(t *testing.T) {
	// The other half of the same decision, and the reason it is granted to the
	// creator rather than to the role: a relative invited to help should not be
	// able to remove the person who invited them (docs/02).
	if slices.Contains(care.DefaultPermissions(care.RoleFamilyMember), care.PermissionMembersManage) {
		t.Error("members.manage has become a family-member default")
	}
	if slices.Contains(
		care.DefaultPermissions(care.RoleProfessionalCaregiver), care.PermissionMembersManage,
	) {
		t.Error("members.manage has become a professional-caregiver default")
	}
}
