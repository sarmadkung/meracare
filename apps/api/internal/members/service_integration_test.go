package members_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/genxcare/api/internal/auth"
	"github.com/genxcare/api/internal/care"
	"github.com/genxcare/api/internal/careevents"
	"github.com/genxcare/api/internal/members"
	"github.com/genxcare/api/internal/relationships"
	"github.com/genxcare/api/internal/seniors"
	"github.com/genxcare/api/internal/testsupport"
	"github.com/genxcare/api/internal/users"
)

type fixture struct {
	members       *members.Service
	seniors       *seniors.Service
	relationships *relationships.Repository
	users         *users.Repository
}

func newFixture(t *testing.T) fixture {
	t.Helper()

	pool := testsupport.RequireDatabase(t)
	relationshipRepo := relationships.NewRepository(pool)

	return fixture{
		members:       members.NewService(relationshipRepo, careevents.NewRecorder(pool, careevents.NewRepository(pool))),
		seniors:       seniors.NewService(seniors.NewRepository(pool), relationshipRepo),
		relationships: relationshipRepo,
		users:         users.NewRepository(pool),
	}
}

func (f fixture) newUser(t *testing.T, email string) auth.Principal {
	t.Helper()
	user, err := f.users.EnsureByAuthUserID(
		context.Background(), uuid.New(), email, users.DefaultDisplayName(email))
	if err != nil {
		t.Fatalf("create user %s: %v", email, err)
	}
	return auth.Principal{UserID: user.ID, AuthUserID: user.AuthUserID, Email: user.Email}
}

func (f fixture) newCircle(t *testing.T, owner auth.Principal, name string) seniors.Membership {
	t.Helper()
	membership, err := f.seniors.Create(context.Background(), owner, seniors.CreateInput{
		Mode:        seniors.CreateModeFamily,
		DisplayName: name,
	})
	if err != nil {
		t.Fatalf("create circle: %v", err)
	}
	return membership
}

func (f fixture) addMember(
	t *testing.T,
	seniorID uuid.UUID,
	user auth.Principal,
	role care.Role,
	permissions ...care.Permission,
) relationships.Relationship {
	t.Helper()

	set := care.Normalise(permissions)
	if len(permissions) == 0 {
		set = care.Normalise(care.DefaultPermissions(role))
	}

	relationship, err := f.relationships.Create(context.Background(), relationships.CreateParams{
		SeniorID:    seniorID,
		UserID:      user.UserID,
		Role:        role,
		Permissions: set,
		Status:      care.StatusActive,
	})
	if err != nil {
		t.Fatalf("add member: %v", err)
	}
	return relationship
}

// --- 10. List members -------------------------------------------------------

func TestListMembers(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sara := f.newUser(t, "sara-list@example.com")
	circle := f.newCircle(t, sara, "Mrs Khan")
	maria := f.newUser(t, "maria-list@example.com")
	f.addMember(t, circle.Senior.ID, maria, care.RoleProfessionalCaregiver)

	found, err := f.members.List(ctx, circle.Senior.ID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("circle has %d members, want 2", len(found))
	}
	for _, member := range found {
		if member.DisplayName == "" {
			t.Error("each member should carry a display name")
		}
	}
}

func TestCaregiverCanLeaveOnlyWhenAnotherManagerRemains(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	owner := f.newUser(t, "leave-owner@example.com")
	circle := f.newCircle(t, owner, "Mrs Khan")

	if _, err := f.members.Revoke(ctx, circle.Senior.ID, circle.Relationship.ID, owner.UserID); !errors.Is(err, members.ErrCannotRemoveSelf) {
		t.Fatalf("self revoke error = %v", err)
	}
	if _, err := f.members.Leave(ctx, circle.Relationship); !errors.Is(err, members.ErrLastCoordinator) {
		t.Fatalf("last coordinator leave error = %v", err)
	}

	replacementUser := f.newUser(t, "replacement-manager@example.com")
	f.addMember(t, circle.Senior.ID, replacementUser, care.RoleFamilyMember, care.PermissionMembersManage)
	left, err := f.members.Leave(ctx, circle.Relationship)
	if err != nil {
		t.Fatalf("Leave: %v", err)
	}
	if left.Status != care.StatusRevoked {
		t.Fatalf("status = %q", left.Status)
	}
}

// --- 13. Multiple family members --------------------------------------------

func TestMultipleFamilyMembersInOneCircle(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sara := f.newUser(t, "sara-family@example.com")
	circle := f.newCircle(t, sara, "Mrs Khan")

	for _, email := range []string{"son@example.com", "daughter@example.com", "spouse@example.com"} {
		f.addMember(t, circle.Senior.ID, f.newUser(t, email), care.RoleFamilyMember)
	}

	found, err := f.members.List(ctx, circle.Senior.ID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// The creator plus three relatives.
	if len(found) != 4 {
		t.Fatalf("circle has %d members, want 4", len(found))
	}
}

// --- 12. Professional caregiver across several seniors ----------------------

func TestCaregiverBelongsToMultipleSeniors(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	maria := f.newUser(t, "maria-multi@example.com")

	for _, name := range []string{"Senior A", "Senior B", "Senior C"} {
		owner := f.newUser(t, "owner-"+name+"@example.com")
		circle := f.newCircle(t, owner, name)
		f.addMember(t, circle.Senior.ID, maria, care.RoleProfessionalCaregiver)
	}

	memberships, err := f.seniors.List(ctx, maria)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(memberships) != 3 {
		t.Fatalf("caregiver reaches %d seniors, want 3", len(memberships))
	}
}

// --- 11 & 12. Update permissions, and refuse escalation ---------------------

func TestUpdatePermissions(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sara := f.newUser(t, "sara-update@example.com")
	circle := f.newCircle(t, sara, "Mrs Khan")
	// The creator needs members.manage to edit others.
	owner, err := f.relationships.UpdatePermissions(ctx, circle.Relationship.ID,
		care.Normalise(append(care.DefaultPermissions(care.RoleFamilyMember), care.PermissionMembersManage)))
	if err != nil {
		t.Fatalf("grant members.manage: %v", err)
	}

	maria := f.newUser(t, "maria-update@example.com")
	member := f.addMember(t, circle.Senior.ID, maria, care.RoleProfessionalCaregiver)

	updated, err := f.members.UpdatePermissions(ctx, owner, circle.Senior.ID, member.ID,
		[]care.Permission{care.PermissionSeniorView, care.PermissionTasksView})
	if err != nil {
		t.Fatalf("UpdatePermissions: %v", err)
	}

	if len(updated.Permissions) != 2 || !updated.Permissions.HasAll(
		care.PermissionSeniorView, care.PermissionTasksView) {
		t.Errorf("permissions = %v, want exactly the two granted", updated.Permissions)
	}
	// Narrowing takes effect immediately.
	if updated.Can(care.PermissionTasksComplete) {
		t.Error("a removed permission should stop granting access")
	}
}

// A member cannot grant, to anyone, a permission they do not hold themselves.
func TestUpdatePermissionsRefusesEscalation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sara := f.newUser(t, "sara-esc@example.com")
	circle := f.newCircle(t, sara, "Mrs Khan")

	// A caregiver allowed to manage members but holding nothing else beyond the
	// caregiver defaults.
	maria := f.newUser(t, "maria-esc@example.com")
	editor := f.addMember(t, circle.Senior.ID, maria, care.RoleProfessionalCaregiver,
		append(care.DefaultPermissions(care.RoleProfessionalCaregiver), care.PermissionMembersManage)...)

	target := f.addMember(t, circle.Senior.ID, f.newUser(t, "target-esc@example.com"),
		care.RoleFamilyMember, care.PermissionSeniorView)

	// Granting somebody else what they lack.
	_, err := f.members.UpdatePermissions(ctx, editor, circle.Senior.ID, target.ID,
		[]care.Permission{care.PermissionSeniorEdit})
	if !errors.Is(err, members.ErrPermissionEscalation) {
		t.Fatalf("error = %v, want ErrPermissionEscalation", err)
	}

	// Granting *themselves* more, which is the sharpest version of the attack.
	_, err = f.members.UpdatePermissions(ctx, editor, circle.Senior.ID, editor.ID,
		[]care.Permission{care.PermissionSeniorEdit, care.PermissionMembersManage})
	if !errors.Is(err, members.ErrPermissionEscalation) {
		t.Fatalf("self-escalation error = %v, want ErrPermissionEscalation", err)
	}

	// The target is untouched.
	unchanged, err := f.relationships.GetByID(ctx, target.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if unchanged.Can(care.PermissionSeniorEdit) {
		t.Error("the refused permission was written anyway")
	}
}

// The senior's own membership is untouchable: stripping it would be the
// sharpest escalation available to a member.
func TestSeniorsOwnMembershipCannotBeChangedOrRemoved(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	ahmed := f.newUser(t, "ahmed-self@example.com")
	own, err := f.seniors.Create(ctx, ahmed, seniors.CreateInput{
		Mode:        seniors.CreateModeSelf,
		DisplayName: "Ahmed",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	sara := f.newUser(t, "sara-self@example.com")
	editor := f.addMember(t, own.Senior.ID, sara, care.RoleFamilyMember,
		append(care.DefaultPermissions(care.RoleFamilyMember), care.PermissionMembersManage)...)

	_, err = f.members.UpdatePermissions(ctx, editor, own.Senior.ID, own.Relationship.ID,
		[]care.Permission{care.PermissionSeniorView})
	if !errors.Is(err, members.ErrCannotModifySenior) {
		t.Fatalf("update error = %v, want ErrCannotModifySenior", err)
	}

	_, err = f.members.Revoke(ctx, own.Senior.ID, own.Relationship.ID, sara.UserID)
	if !errors.Is(err, members.ErrCannotModifySenior) {
		t.Fatalf("revoke error = %v, want ErrCannotModifySenior", err)
	}

	// The senior still runs their own circle.
	unchanged, err := f.relationships.GetByID(ctx, own.Relationship.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !unchanged.IsActive() || !unchanged.Can(care.PermissionSeniorEdit) {
		t.Error("the senior lost access to their own profile")
	}
}

// --- 13, 14, 15. Revocation -------------------------------------------------

func TestRevokedMemberLosesAccessButKeepsTheirRecord(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sara := f.newUser(t, "sara-revoke@example.com")
	circle := f.newCircle(t, sara, "Mrs Khan")
	maria := f.newUser(t, "maria-revoke@example.com")
	member := f.addMember(t, circle.Senior.ID, maria, care.RoleProfessionalCaregiver)

	revoked, err := f.members.Revoke(ctx, circle.Senior.ID, member.ID, sara.UserID)
	if err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	// Access stops immediately.
	if revoked.IsActive() || revoked.Can(care.PermissionSeniorView) {
		t.Error("a revoked membership must grant nothing")
	}
	reachable, err := f.seniors.List(ctx, maria)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(reachable) != 0 {
		t.Errorf("a revoked member reaches %d seniors, want 0", len(reachable))
	}

	// The record survives, so anything Maria authored keeps its author.
	preserved, err := f.relationships.GetByID(ctx, member.ID)
	if err != nil {
		t.Fatalf("the relationship record was deleted: %v", err)
	}
	if preserved.ID != member.ID || preserved.UserID != maria.UserID {
		t.Error("the historical relationship was not preserved intact")
	}

	// And they drop out of the circle listing.
	remaining, err := f.members.List(ctx, circle.Senior.ID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(remaining) != 1 {
		t.Errorf("circle lists %d members, want 1", len(remaining))
	}
}

// A membership in another circle must not be reachable by ID.
func TestCannotManageAMemberOfAnotherCircle(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sara := f.newUser(t, "sara-other@example.com")
	mine := f.newCircle(t, sara, "Mrs Khan")
	editor, err := f.relationships.UpdatePermissions(ctx, mine.Relationship.ID,
		care.Normalise(append(care.DefaultPermissions(care.RoleFamilyMember), care.PermissionMembersManage)))
	if err != nil {
		t.Fatalf("grant members.manage: %v", err)
	}

	stranger := f.newUser(t, "stranger-other@example.com")
	theirs := f.newCircle(t, stranger, "Mr Ahmed")

	// Sara manages her own circle, and aims a member ID from another circle at it.
	_, err = f.members.UpdatePermissions(ctx, editor, mine.Senior.ID, theirs.Relationship.ID,
		[]care.Permission{care.PermissionSeniorView})
	if !errors.Is(err, members.ErrNotFound) {
		t.Fatalf("update error = %v, want ErrNotFound", err)
	}

	_, err = f.members.Revoke(ctx, mine.Senior.ID, theirs.Relationship.ID, sara.UserID)
	if !errors.Is(err, members.ErrNotFound) {
		t.Fatalf("revoke error = %v, want ErrNotFound", err)
	}

	// The other circle is untouched.
	untouched, err := f.relationships.GetByID(ctx, theirs.Relationship.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !untouched.IsActive() {
		t.Error("a membership in another circle was revoked")
	}
}

func TestRevokeUnknownMember(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sara := f.newUser(t, "sara-unknown@example.com")
	circle := f.newCircle(t, sara, "Mrs Khan")

	if _, err := f.members.Revoke(ctx, circle.Senior.ID, uuid.New(), sara.UserID); !errors.Is(err, members.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}
