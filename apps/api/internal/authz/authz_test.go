package authz_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/genxcare/api/internal/auth"
	"github.com/genxcare/api/internal/authz"
	"github.com/genxcare/api/internal/care"
	"github.com/genxcare/api/internal/relationships"
	"github.com/genxcare/api/pkg/httpx"
)

// stubResolver stands in for the relationships repository.
type stubResolver struct {
	relationship relationships.Relationship
	err          error
	calls        int
}

func (s *stubResolver) FindByUserAndSenior(_ context.Context, _, _ uuid.UUID) (relationships.Relationship, error) {
	s.calls++
	if s.err != nil {
		return relationships.Relationship{}, s.err
	}
	return s.relationship, nil
}

// guarded builds a router with one protected route, mimicking how the server
// mounts senior-scoped endpoints.
func guarded(
	t *testing.T,
	resolver authz.RelationshipResolver,
	principal auth.Principal,
	required []care.Permission,
	handler http.HandlerFunc,
) http.Handler {
	t.Helper()

	guard := authz.NewGuard(resolver)
	router := chi.NewRouter()
	router.NotFound(httpx.NotFoundHandler())

	// Stand in for the authentication middleware.
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), principal)))
		})
	})

	router.With(guard.RequirePermission(required...)).
		Get("/v1/seniors/{"+authz.SeniorIDParam+"}", handler)

	return router
}

func activeRelationship(seniorID, userID uuid.UUID, role care.Role) relationships.Relationship {
	return relationships.Relationship{
		ID:          uuid.New(),
		SeniorID:    seniorID,
		UserID:      userID,
		Role:        role,
		Permissions: care.Normalise(care.DefaultPermissions(role)),
		Status:      care.StatusActive,
	}
}

func get(t *testing.T, handler http.Handler, seniorID string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/seniors/"+seniorID, nil))
	return rec
}

func TestGuardAllowsPermittedMember(t *testing.T) {
	seniorID, userID := uuid.New(), uuid.New()
	relationship := activeRelationship(seniorID, userID, care.RoleFamilyMember)
	resolver := &stubResolver{relationship: relationship}

	reached := false
	handler := guarded(t, resolver, auth.Principal{UserID: userID},
		[]care.Permission{care.PermissionSeniorView},
		func(w http.ResponseWriter, r *http.Request) {
			reached = true
			// The handler receives the authorized relationship, so it never
			// re-queries or re-derives access.
			got := authz.MustRelationship(r.Context())
			if got.ID != relationship.ID {
				t.Errorf("relationship ID = %s, want %s", got.ID, relationship.ID)
			}
			w.WriteHeader(http.StatusOK)
		})

	rec := get(t, handler, seniorID.String())

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if !reached {
		t.Error("handler did not run for a permitted member")
	}
}

// A stranger must not be able to tell an inaccessible senior from a
// non-existent one.
func TestGuardHidesSeniorsTheCallerCannotReach(t *testing.T) {
	seniorID := uuid.New()

	cases := map[string]*stubResolver{
		"no relationship": {err: relationships.ErrNotFound},
		"pending invitation": {relationship: relationships.Relationship{
			SeniorID:    seniorID,
			Role:        care.RoleFamilyMember,
			Permissions: care.Normalise(care.DefaultPermissions(care.RoleFamilyMember)),
			Status:      care.StatusPending,
		}},
		"revoked membership": {relationship: relationships.Relationship{
			SeniorID:    seniorID,
			Role:        care.RoleFamilyMember,
			Permissions: care.Normalise(care.DefaultPermissions(care.RoleFamilyMember)),
			Status:      care.StatusRevoked,
		}},
	}

	for name, resolver := range cases {
		t.Run(name, func(t *testing.T) {
			called := false
			handler := guarded(t, resolver, auth.Principal{UserID: uuid.New()},
				[]care.Permission{care.PermissionSeniorView},
				func(http.ResponseWriter, *http.Request) { called = true })

			rec := get(t, handler, seniorID.String())

			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", rec.Code)
			}
			if called {
				t.Error("handler ran for an unauthorized caller")
			}

			var body httpx.ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("response is not the error envelope: %v", err)
			}
			if body.Error.Code != httpx.CodeNotFound {
				t.Errorf("code = %q, want %q", body.Error.Code, httpx.CodeNotFound)
			}
			// The message must not confirm that the senior exists.
			if body.Error.Message == "" {
				t.Error("expected a user-facing message")
			}
		})
	}
}

// A professional caregiver is a legitimate member but must not edit the profile.
func TestGuardRejectsMemberWithoutTheRequiredPermission(t *testing.T) {
	seniorID, userID := uuid.New(), uuid.New()
	resolver := &stubResolver{
		relationship: activeRelationship(seniorID, userID, care.RoleProfessionalCaregiver),
	}

	called := false
	handler := guarded(t, resolver, auth.Principal{UserID: userID},
		[]care.Permission{care.PermissionSeniorEdit},
		func(http.ResponseWriter, *http.Request) { called = true })

	rec := get(t, handler, seniorID.String())

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if called {
		t.Error("handler ran without the required permission")
	}
}

// Every listed permission is required, not merely one of them.
func TestGuardRequiresEveryListedPermission(t *testing.T) {
	seniorID, userID := uuid.New(), uuid.New()
	resolver := &stubResolver{
		relationship: activeRelationship(seniorID, userID, care.RoleProfessionalCaregiver),
	}

	handler := guarded(t, resolver, auth.Principal{UserID: userID},
		// The caregiver holds the first but not the second.
		[]care.Permission{care.PermissionSeniorView, care.PermissionMembersManage},
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	if rec := get(t, handler, seniorID.String()); rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestGuardRejectsMalformedSeniorIDWithoutQueryingTheDatabase(t *testing.T) {
	resolver := &stubResolver{relationship: activeRelationship(uuid.New(), uuid.New(), care.RoleSenior)}

	handler := guarded(t, resolver, auth.Principal{UserID: uuid.New()},
		[]care.Permission{care.PermissionSeniorView},
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	rec := get(t, handler, "not-a-uuid")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if resolver.calls != 0 {
		t.Errorf("resolver called %d times for a malformed ID, want 0", resolver.calls)
	}
}

// A database failure must not read as "you have no access".
func TestGuardReturnsInternalWhenTheLookupFails(t *testing.T) {
	resolver := &stubResolver{err: errors.New("connection reset")}

	handler := guarded(t, resolver, auth.Principal{UserID: uuid.New()},
		[]care.Permission{care.PermissionSeniorView},
		func(http.ResponseWriter, *http.Request) { t.Error("handler ran despite a lookup failure") })

	rec := get(t, handler, uuid.NewString())

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "connection reset") {
		t.Errorf("response leaked the internal error: %s", rec.Body.String())
	}
}

func TestRelationshipFromEmptyContext(t *testing.T) {
	if _, ok := authz.RelationshipFrom(context.Background()); ok {
		t.Error("RelationshipFrom returned a relationship for an unguarded context")
	}

	defer func() {
		if recover() == nil {
			t.Error("MustRelationship did not panic outside a guarded route")
		}
	}()
	authz.MustRelationship(context.Background())
}

func TestRelationshipCan(t *testing.T) {
	permissions := care.PermissionSet{care.PermissionSeniorView}

	cases := map[care.RelationshipStatus]bool{
		care.StatusActive:  true,
		care.StatusPending: false,
		care.StatusRevoked: false,
	}

	for status, want := range cases {
		relationship := relationships.Relationship{Permissions: permissions, Status: status}
		if got := relationship.Can(care.PermissionSeniorView); got != want {
			t.Errorf("status %q: Can() = %v, want %v", status, got, want)
		}
	}

	active := relationships.Relationship{Permissions: permissions, Status: care.StatusActive}
	if active.Can(care.PermissionSeniorEdit) {
		t.Error("Can() granted a permission that was never held")
	}
}
