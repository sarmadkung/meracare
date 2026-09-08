package invitations_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/genxcare/api/internal/auth"
	"github.com/genxcare/api/internal/care"
	"github.com/genxcare/api/internal/careevents"
	"github.com/genxcare/api/internal/invitations"
	"github.com/genxcare/api/internal/members"
	"github.com/genxcare/api/internal/relationships"
	"github.com/genxcare/api/internal/seniors"
	"github.com/genxcare/api/internal/testsupport"
	"github.com/genxcare/api/internal/users"
)

// Integration tests for the invitation lifecycle and the authorization rules
// around it. Skipped unless TEST_DATABASE_URL is set.

type fixture struct {
	events        *careevents.Service
	invitations   *invitations.Service
	invitationRep *invitations.Repository
	members       *members.Service
	seniors       *seniors.Service
	relationships *relationships.Repository
	users         *users.Repository
	now           func() time.Time
}

// userLookup mirrors the adapter the server wires in.
type userLookup struct{ repo *users.Repository }

func (l userLookup) GetByID(ctx context.Context, id uuid.UUID) (invitations.UserSummary, error) {
	user, err := l.repo.GetByID(ctx, id)
	if err != nil {
		return invitations.UserSummary{}, err
	}
	return invitations.UserSummary{ID: user.ID, DisplayName: user.DisplayName, Email: user.Email}, nil
}

func (l userLookup) FindIDByEmail(ctx context.Context, email string) (uuid.UUID, error) {
	return l.repo.FindIDByEmail(ctx, email)
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	pool := testsupport.RequireDatabase(t)
	relationshipRepo := relationships.NewRepository(pool)
	seniorRepo := seniors.NewRepository(pool)
	userRepo := users.NewRepository(pool)
	invitationRepo := invitations.NewRepository(pool)
	eventRepo := careevents.NewRepository(pool)
	recorder := careevents.NewRecorder(pool, eventRepo)

	f := &fixture{
		events:        careevents.NewService(eventRepo),
		invitationRep: invitationRepo,
		members:       members.NewService(relationshipRepo, recorder),
		seniors:       seniors.NewService(seniorRepo, relationshipRepo),
		relationships: relationshipRepo,
		users:         userRepo,
		now:           time.Now,
	}
	f.invitations = invitations.NewService(
		invitationRepo, relationshipRepo, userLookup{repo: userRepo}, seniorRepo, recorder,
	).WithClock(func() time.Time { return f.now() })

	return f
}

func (f *fixture) newUser(t *testing.T, email string) auth.Principal {
	t.Helper()
	user, err := f.users.EnsureByAuthUserID(
		context.Background(), uuid.New(), email, users.DefaultDisplayName(email))
	if err != nil {
		t.Fatalf("create user %s: %v", email, err)
	}
	return auth.Principal{UserID: user.ID, AuthUserID: user.AuthUserID, Email: user.Email}
}

// newCircle creates a senior with `owner` as the family member running it.
func (f *fixture) newCircle(t *testing.T, owner auth.Principal, name string) seniors.Membership {
	t.Helper()
	membership, err := f.seniors.Create(context.Background(), owner, seniors.CreateInput{
		Mode:        seniors.CreateModeFamily,
		DisplayName: name,
	})
	if err != nil {
		t.Fatalf("create circle %s: %v", name, err)
	}
	return membership
}

// invite issues an invitation and returns it with its token.
func (f *fixture) invite(
	t *testing.T,
	inviter relationships.Relationship,
	seniorID uuid.UUID,
	email string,
	role care.Role,
	permissions ...care.Permission,
) invitations.Created {
	t.Helper()
	created, err := f.invitations.Create(context.Background(), inviter, invitations.CreateInput{
		SeniorID:    seniorID,
		Email:       email,
		Role:        role,
		Permissions: permissions,
	})
	if err != nil {
		t.Fatalf("invite %s: %v", email, err)
	}
	return created
}

// --- 1. Create invitation ---------------------------------------------------

func TestCreateInvitation(t *testing.T) {
	f := newFixture(t)
	sara := f.newUser(t, "sara@example.com")
	circle := f.newCircle(t, sara, "Mrs Khan")

	created := f.invite(t, circle.Relationship, circle.Senior.ID, "Maria@Example.com",
		care.RoleProfessionalCaregiver)

	if created.Invitation.Status != invitations.StatusPending {
		t.Errorf("status = %q, want pending", created.Invitation.Status)
	}
	// Stored lowercased so matching on accept is reliable.
	if created.Invitation.InviteeEmail != "maria@example.com" {
		t.Errorf("email = %q, want it normalised", created.Invitation.InviteeEmail)
	}
	if !created.Token.Valid() {
		t.Error("a usable token should be returned")
	}
	if !created.Invitation.ExpiresAt.After(time.Now()) {
		t.Error("a new invitation should expire in the future")
	}
	// Defaults for the role, since none were requested.
	if !created.Invitation.Permissions.Has(care.PermissionTasksComplete) {
		t.Error("caregiver defaults should include tasks.complete")
	}
	if created.Invitation.Permissions.Has(care.PermissionSeniorEdit) {
		t.Error("caregiver defaults should not include senior.edit")
	}
}

// --- 2. Invalid permission --------------------------------------------------

func TestCreateInvitationDropsUnrecognisedPermissions(t *testing.T) {
	f := newFixture(t)
	sara := f.newUser(t, "sara-invalid@example.com")
	circle := f.newCircle(t, sara, "Mrs Khan")

	created := f.invite(t, circle.Relationship, circle.Senior.ID, "maria@example.com",
		care.RoleProfessionalCaregiver, "owner", "*", care.PermissionTasksView)

	// An invented permission is dropped, never stored, and never granted.
	if len(created.Invitation.Permissions) != 1 ||
		!created.Invitation.Permissions.Has(care.PermissionTasksView) {
		t.Errorf("permissions = %v, want only tasks.view", created.Invitation.Permissions)
	}
}

// --- 3. Escalation ----------------------------------------------------------

// The escalation the spec calls out: granting rights the inviter lacks.
func TestCreateInvitationRefusesEscalation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sara := f.newUser(t, "sara-esc@example.com")
	circle := f.newCircle(t, sara, "Mrs Khan")

	// A caregiver who has been allowed to invite, but holds nothing else beyond
	// the caregiver defaults.
	caregiver := f.newUser(t, "maria-esc@example.com")
	limited, err := f.relationships.Create(ctx, relationships.CreateParams{
		SeniorID: circle.Senior.ID,
		UserID:   caregiver.UserID,
		Role:     care.RoleProfessionalCaregiver,
		Permissions: care.Normalise(append(
			care.DefaultPermissions(care.RoleProfessionalCaregiver),
			care.PermissionMembersInvite,
		)),
		Status: care.StatusActive,
	})
	if err != nil {
		t.Fatalf("create caregiver membership: %v", err)
	}

	// They cannot hand out member management, which they do not hold.
	_, err = f.invitations.Create(ctx, limited, invitations.CreateInput{
		SeniorID:    circle.Senior.ID,
		Email:       "accomplice@example.com",
		Role:        care.RoleFamilyMember,
		Permissions: []care.Permission{care.PermissionMembersManage},
	})
	if !errors.Is(err, invitations.ErrPermissionEscalation) {
		t.Fatalf("error = %v, want ErrPermissionEscalation", err)
	}

	// Nor senior.edit.
	_, err = f.invitations.Create(ctx, limited, invitations.CreateInput{
		SeniorID:    circle.Senior.ID,
		Email:       "accomplice@example.com",
		Role:        care.RoleFamilyMember,
		Permissions: []care.Permission{care.PermissionSeniorEdit},
	})
	if !errors.Is(err, invitations.ErrPermissionEscalation) {
		t.Fatalf("error = %v, want ErrPermissionEscalation", err)
	}

	// And asking for the role's defaults does not smuggle them through: the
	// family defaults include both, and both are silently withheld.
	created, err := f.invitations.Create(ctx, limited, invitations.CreateInput{
		SeniorID: circle.Senior.ID,
		Email:    "accomplice@example.com",
		Role:     care.RoleFamilyMember,
	})
	if err != nil {
		t.Fatalf("Create with defaults: %v", err)
	}
	for _, forbidden := range []care.Permission{
		care.PermissionMembersManage,
		care.PermissionSeniorEdit,
		care.PermissionTasksManage,
	} {
		if created.Invitation.Permissions.Has(forbidden) {
			t.Errorf("defaults leaked %q past a granter who lacks it", forbidden)
		}
	}
}

func TestCreateInvitationRefusesTheSeniorRole(t *testing.T) {
	f := newFixture(t)
	sara := f.newUser(t, "sara-role@example.com")
	circle := f.newCircle(t, sara, "Mrs Khan")

	_, err := f.invitations.Create(context.Background(), circle.Relationship, invitations.CreateInput{
		SeniorID: circle.Senior.ID,
		Email:    "someone@example.com",
		Role:     care.RoleSenior,
	})
	if !errors.Is(err, invitations.ErrRoleNotInvitable) {
		t.Fatalf("error = %v, want ErrRoleNotInvitable", err)
	}
}

// --- 4 & 5. Retrieve and accept ---------------------------------------------

// A code is read off a share message and typed back in, so it returns grouped
// and in whatever case the keyboard produced. It must still find its
// invitation.
func TestInvitationIsFoundByACodeAsAPersonTypesIt(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sara := f.newUser(t, "sara-typed@example.com")
	circle := f.newCircle(t, sara, "Mrs Khan")
	maria := f.newUser(t, "maria-typed@example.com")

	created := f.invite(t, circle.Relationship, circle.Senior.ID, maria.Email,
		care.RoleProfessionalCaregiver)

	raw := string(created.Token)
	grouped := invitations.Token(strings.ToLower(raw[:4] + "-" + raw[4:8] + "-" + raw[8:]))

	if _, err := f.invitations.Preview(ctx, grouped); err != nil {
		t.Fatalf("Preview with a grouped, lowercased code: %v", err)
	}

	relationship, err := f.invitations.Accept(ctx, maria, grouped)
	if err != nil {
		t.Fatalf("Accept with a grouped, lowercased code: %v", err)
	}
	if !relationship.IsActive() {
		t.Errorf("relationship = %+v, want an active membership", relationship)
	}
}

func TestAcceptInvitationCreatesMembership(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sara := f.newUser(t, "sara-accept@example.com")
	circle := f.newCircle(t, sara, "Mrs Khan")
	maria := f.newUser(t, "maria-accept@example.com")

	created := f.invite(t, circle.Relationship, circle.Senior.ID, maria.Email,
		care.RoleProfessionalCaregiver)

	preview, err := f.invitations.Preview(ctx, created.Token)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if preview.SeniorName != "Mrs Khan" || preview.InviterName == "" {
		t.Errorf("preview = %+v, want the senior and inviter named", preview)
	}

	relationship, err := f.invitations.Accept(ctx, maria, created.Token)
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if !relationship.IsActive() || relationship.Role != care.RoleProfessionalCaregiver {
		t.Errorf("relationship = %+v, want an active caregiver membership", relationship)
	}

	// The senior is now reachable.
	memberships, err := f.seniors.List(ctx, maria)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(memberships) != 1 || memberships[0].Senior.ID != circle.Senior.ID {
		t.Fatal("the accepted senior should be reachable")
	}
}

// --- 6, 7, 8. Unusable invitations ------------------------------------------

func TestAcceptExpiredInvitation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sara := f.newUser(t, "sara-exp@example.com")
	circle := f.newCircle(t, sara, "Mrs Khan")
	maria := f.newUser(t, "maria-exp@example.com")

	created, err := f.invitations.Create(ctx, circle.Relationship, invitations.CreateInput{
		SeniorID: circle.Senior.ID,
		Email:    maria.Email,
		Role:     care.RoleProfessionalCaregiver,
		Lifetime: time.Hour,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Move the clock past expiry. Nothing swept the row; the API itself must
	// refuse it.
	f.now = func() time.Time { return time.Now().Add(2 * time.Hour) }

	if _, err := f.invitations.Accept(ctx, maria, created.Token); !errors.Is(err, invitations.ErrNotAcceptable) {
		t.Fatalf("error = %v, want ErrNotAcceptable", err)
	}

	preview, err := f.invitations.Preview(ctx, created.Token)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if preview.Status != string(invitations.StatusExpired) {
		t.Errorf("preview status = %q, want expired", preview.Status)
	}
}

func TestAcceptRevokedInvitation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sara := f.newUser(t, "sara-rev@example.com")
	circle := f.newCircle(t, sara, "Mrs Khan")
	maria := f.newUser(t, "maria-rev@example.com")

	created := f.invite(t, circle.Relationship, circle.Senior.ID, maria.Email,
		care.RoleProfessionalCaregiver)

	if _, err := f.invitations.Revoke(ctx, created.Invitation.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	if _, err := f.invitations.Accept(ctx, maria, created.Token); !errors.Is(err, invitations.ErrNotAcceptable) {
		t.Fatalf("error = %v, want ErrNotAcceptable", err)
	}
}

// A token is single-use.
func TestAcceptedInvitationCannotBeReused(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sara := f.newUser(t, "sara-reuse@example.com")
	circle := f.newCircle(t, sara, "Mrs Khan")
	maria := f.newUser(t, "maria-reuse@example.com")

	created := f.invite(t, circle.Relationship, circle.Senior.ID, maria.Email,
		care.RoleProfessionalCaregiver)

	if _, err := f.invitations.Accept(ctx, maria, created.Token); err != nil {
		t.Fatalf("first Accept: %v", err)
	}
	if _, err := f.invitations.Accept(ctx, maria, created.Token); !errors.Is(err, invitations.ErrNotAcceptable) {
		t.Fatalf("second Accept error = %v, want ErrNotAcceptable", err)
	}

	// And nobody else can redeem it either.
	other := f.newUser(t, "other-reuse@example.com")
	if _, err := f.invitations.Accept(ctx, other, created.Token); err == nil {
		t.Fatal("a spent token must not work for anybody")
	}
}

// An invitation is addressed to a person, not to whoever holds the link.
func TestUserCannotAcceptAnotherPersonsInvitation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sara := f.newUser(t, "sara-wrong@example.com")
	circle := f.newCircle(t, sara, "Mrs Khan")
	intended := f.newUser(t, "intended@example.com")
	interloper := f.newUser(t, "interloper@example.com")

	created := f.invite(t, circle.Relationship, circle.Senior.ID, intended.Email,
		care.RoleProfessionalCaregiver)

	if _, err := f.invitations.Accept(ctx, interloper, created.Token); !errors.Is(err, invitations.ErrWrongRecipient) {
		t.Fatalf("error = %v, want ErrWrongRecipient", err)
	}

	// The invitation survives for the person it was meant for.
	if _, err := f.invitations.Accept(ctx, intended, created.Token); err != nil {
		t.Fatalf("the intended recipient could not accept: %v", err)
	}
}

func TestInvalidTokenIsRejected(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	someone := f.newUser(t, "someone@example.com")

	for name, token := range map[string]invitations.Token{
		"empty":       "",
		"garbage":     "not-a-token",
		"wrong shape": invitations.Token(uuid.NewString()),
		"well-formed but unissued": func() invitations.Token {
			token, err := invitations.NewToken()
			if err != nil {
				t.Fatalf("NewToken: %v", err)
			}
			return token
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := f.invitations.Preview(ctx, token); !errors.Is(err, invitations.ErrNotFound) {
				t.Errorf("Preview error = %v, want ErrNotFound", err)
			}
			if _, err := f.invitations.Accept(ctx, someone, token); !errors.Is(err, invitations.ErrNotFound) {
				t.Errorf("Accept error = %v, want ErrNotFound", err)
			}
		})
	}
}

// --- 9. Revoke --------------------------------------------------------------

func TestRevokeInvitationIsIdempotentlyRefused(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sara := f.newUser(t, "sara-revtwice@example.com")
	circle := f.newCircle(t, sara, "Mrs Khan")

	created := f.invite(t, circle.Relationship, circle.Senior.ID, "maria@example.com",
		care.RoleProfessionalCaregiver)

	if _, err := f.invitations.Revoke(ctx, created.Invitation.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	// A second revoke has nothing pending to act on.
	if _, err := f.invitations.Revoke(ctx, created.Invitation.ID); !errors.Is(err, invitations.ErrNotAcceptable) {
		t.Fatalf("second Revoke error = %v, want ErrNotAcceptable", err)
	}
}

// Re-inviting the same person while one invitation is outstanding is refused,
// so a circle cannot accumulate several live ways in for one address.
func TestDuplicatePendingInvitationIsRefused(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sara := f.newUser(t, "sara-dupe@example.com")
	circle := f.newCircle(t, sara, "Mrs Khan")

	f.invite(t, circle.Relationship, circle.Senior.ID, "maria@example.com", care.RoleProfessionalCaregiver)

	_, err := f.invitations.Create(ctx, circle.Relationship, invitations.CreateInput{
		SeniorID: circle.Senior.ID,
		Email:    "maria@example.com",
		Role:     care.RoleProfessionalCaregiver,
	})
	if !errors.Is(err, invitations.ErrAlreadyInvited) {
		t.Fatalf("error = %v, want ErrAlreadyInvited", err)
	}
}

func TestCannotInviteAnExistingMember(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sara := f.newUser(t, "sara-member@example.com")
	circle := f.newCircle(t, sara, "Mrs Khan")
	maria := f.newUser(t, "maria-member@example.com")

	created := f.invite(t, circle.Relationship, circle.Senior.ID, maria.Email, care.RoleProfessionalCaregiver)
	if _, err := f.invitations.Accept(ctx, maria, created.Token); err != nil {
		t.Fatalf("Accept: %v", err)
	}

	_, err := f.invitations.Create(ctx, circle.Relationship, invitations.CreateInput{
		SeniorID: circle.Senior.ID,
		Email:    maria.Email,
		Role:     care.RoleFamilyMember,
	})
	if !errors.Is(err, invitations.ErrAlreadyMember) {
		t.Fatalf("error = %v, want ErrAlreadyMember", err)
	}
}

// Somebody who left can be invited back; the revived membership is the same row.
func TestRevokedMemberCanBeInvitedBack(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sara := f.newUser(t, "sara-back@example.com")
	circle := f.newCircle(t, sara, "Mrs Khan")
	maria := f.newUser(t, "maria-back@example.com")

	first := f.invite(t, circle.Relationship, circle.Senior.ID, maria.Email, care.RoleProfessionalCaregiver)
	original, err := f.invitations.Accept(ctx, maria, first.Token)
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}

	if _, err := f.members.Revoke(ctx, circle.Senior.ID, original.ID, circle.Relationship.UserID); err != nil {
		t.Fatalf("Revoke membership: %v", err)
	}

	second := f.invite(t, circle.Relationship, circle.Senior.ID, maria.Email, care.RoleFamilyMember)
	revived, err := f.invitations.Accept(ctx, maria, second.Token)
	if err != nil {
		t.Fatalf("second Accept: %v", err)
	}

	// Same row, revived — so anything Maria authored keeps its author.
	if revived.ID != original.ID {
		t.Errorf("membership ID changed from %s to %s; history would be orphaned", original.ID, revived.ID)
	}
	if !revived.IsActive() || revived.Role != care.RoleFamilyMember {
		t.Errorf("revived membership = %+v, want an active family member", revived)
	}
}

// --- Care events (Phase 7) ---------------------------------------------------

// Inviting somebody and their joining are both things that happened to the care
// circle, and both belong in the one timeline rather than in an invitation
// history nobody else reads (plans/phase7.md §7).
func TestInvitingAndJoiningAppearInTheTimeline(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sara := f.newUser(t, "sara-events@example.com")
	circle := f.newCircle(t, sara, "Mrs Khan")
	maria := f.newUser(t, "maria-events@example.com")

	created := f.invite(t, circle.Relationship, circle.Senior.ID, maria.Email,
		care.RoleProfessionalCaregiver)

	page, err := f.events.Activity(ctx, circle.Senior.ID, "", 50)
	if err != nil {
		t.Fatalf("read activity: %v", err)
	}

	invited := requireEvent(t, page.Items, careevents.TypeMemberInvited)
	if invited.ActorUserID == nil || *invited.ActorUserID != sara.UserID {
		t.Errorf("actor = %v, want the inviter %v", invited.ActorUserID, sara.UserID)
	}
	if invited.EntityType != careevents.EntityInvitation || invited.EntityID != created.Invitation.ID {
		t.Errorf("event points at %s %v, want invitation %v",
			invited.EntityType, invited.EntityID, created.Invitation.ID)
	}
	if invited.Metadata[careevents.MetaRole] != string(care.RoleProfessionalCaregiver) {
		t.Errorf("metadata = %v, want the proposed role", invited.Metadata)
	}

	if _, err := f.invitations.Accept(ctx, maria, created.Token); err != nil {
		t.Fatalf("accept: %v", err)
	}

	page, err = f.events.Activity(ctx, circle.Senior.ID, "", 50)
	if err != nil {
		t.Fatalf("read activity: %v", err)
	}

	joined := requireEvent(t, page.Items, careevents.TypeMemberJoined)
	// The actor is the person joining, from their session — the one thing about
	// this flow a client could never be trusted to assert.
	if joined.ActorUserID == nil || *joined.ActorUserID != maria.UserID {
		t.Errorf("actor = %v, want the person who joined %v", joined.ActorUserID, maria.UserID)
	}
}

// A refused acceptance must leave no trace: no membership, and no event saying
// somebody joined (plans/phase7.md §26).
func TestARefusedAcceptanceRecordsNoEvent(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	sara := f.newUser(t, "sara-refused@example.com")
	circle := f.newCircle(t, sara, "Mrs Khan")
	maria := f.newUser(t, "maria-refused@example.com")
	intruder := f.newUser(t, "intruder-refused@example.com")

	created := f.invite(t, circle.Relationship, circle.Senior.ID, maria.Email,
		care.RoleProfessionalCaregiver)

	// An invitation is addressed to a person, not to whoever holds the link.
	if _, err := f.invitations.Accept(ctx, intruder, created.Token); err == nil {
		t.Fatal("somebody else redeemed the invitation")
	}

	page, err := f.events.Activity(ctx, circle.Senior.ID, "", 50)
	if err != nil {
		t.Fatalf("read activity: %v", err)
	}
	for _, event := range page.Items {
		if event.Type == careevents.TypeMemberJoined {
			t.Error("a refused acceptance wrote MEMBER_JOINED")
		}
	}
}

func requireEvent(
	t *testing.T,
	events []careevents.Event,
	wanted careevents.Type,
) careevents.Event {
	t.Helper()

	for _, event := range events {
		if event.Type == wanted {
			return event
		}
	}

	found := make([]careevents.Type, 0, len(events))
	for _, event := range events {
		found = append(found, event.Type)
	}
	t.Fatalf("no %s in the timeline; found %v", wanted, found)
	return careevents.Event{}
}
