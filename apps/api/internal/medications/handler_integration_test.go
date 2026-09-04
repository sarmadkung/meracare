package medications_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/genxcare/api/internal/auth"
	"github.com/genxcare/api/internal/care"
	"github.com/genxcare/api/internal/config"
	"github.com/genxcare/api/internal/database"
	"github.com/genxcare/api/internal/relationships"
	"github.com/genxcare/api/internal/server"
	"github.com/genxcare/api/internal/testsupport"
	"github.com/genxcare/api/pkg/logging"
)

// These tests drive the real router over HTTP against a real database, because
// the questions they ask — what status does an intruder see, can a caregiver
// record a dose without being able to change the prescription — are answered by
// the middleware stack and the handlers together, and only the whole stack can
// be trusted to answer them.

// stubVerifier stands in for Supabase, mapping a bearer token to an identity.
// Nothing here exercises token validation; internal/auth covers that.
type stubVerifier struct {
	accounts map[string]auth.Claims
}

func (v stubVerifier) Verify(_ context.Context, rawToken string) (*auth.Claims, error) {
	claims, ok := v.accounts[rawToken]
	if !ok {
		return nil, auth.ErrInvalidToken
	}
	return &claims, nil
}

type harness struct {
	handler       http.Handler
	verifier      stubVerifier
	relationships *relationships.Repository
	pool          *database.Pool
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	pool := testsupport.RequireDatabase(t)
	verifier := stubVerifier{accounts: map[string]auth.Claims{}}

	return &harness{
		handler: server.New(server.Dependencies{
			Config: &config.Config{
				Env:            config.EnvTest,
				Port:           8080,
				RequestTimeout: 10 * time.Second,
			},
			Logger:   logging.New(io.Discard, logging.Options{Level: "error"}),
			Pool:     pool,
			Verifier: verifier,
		}),
		verifier:      verifier,
		relationships: relationships.NewRepository(pool),
		pool:          pool,
	}
}

// account registers a bearer token for a new identity. The application user is
// created on the first authenticated request, exactly as in production.
func (h *harness) account(token, email string) {
	h.verifier.accounts[token] = auth.Claims{
		AuthUserID: uuid.New(),
		Email:      email,
		Provider:   "email",
		IssuedAt:   time.Now().Add(-time.Minute),
		ExpiresAt:  time.Now().Add(time.Hour),
	}
}

func (h *harness) do(
	t *testing.T,
	token, method, path string,
	body any,
) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode request: %v", err)
		}
		reader = bytes.NewReader(encoded)
	}

	request := httptest.NewRequest(method, path, reader)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}

	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, request)
	return recorder
}

// decodeBody reads a JSON response body, failing the test on unexpected statuses.
func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, want int) map[string]any {
	t.Helper()

	if rec.Code != want {
		t.Fatalf("status = %d, want %d\nbody: %s", rec.Code, want, rec.Body.String())
	}

	var body map[string]any
	if rec.Body.Len() == 0 {
		return body
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v\nbody: %s", err, rec.Body.String())
	}
	return body
}

// createCircle signs a token in and creates a senior, returning the senior's ID.
func (h *harness) createCircle(t *testing.T, token, name, timezone string) string {
	t.Helper()

	rec := h.do(t, token, http.MethodPost, "/v1/seniors", map[string]any{
		"mode":        "family_member",
		"displayName": name,
		"timezone":    timezone,
	})
	body := decodeBody(t, rec, http.StatusCreated)

	id, _ := body["id"].(string)
	if id == "" {
		t.Fatalf("no senior id in response: %s", rec.Body.String())
	}
	return id
}

// join adds an account to a circle with an exact permission set, so a test can
// pin one boundary at a time.
func (h *harness) join(
	t *testing.T,
	token, seniorID string,
	role care.Role,
	permissions ...care.Permission,
) uuid.UUID {
	t.Helper()

	// The application user exists only after the account's first authenticated
	// request, so make one.
	if rec := h.do(t, token, http.MethodGet, "/v1/seniors", nil); rec.Code != http.StatusOK {
		t.Fatalf("sign in: status = %d", rec.Code)
	}

	claims := h.verifier.accounts[token]
	var userID uuid.UUID
	err := h.pool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE auth_user_id = $1`, claims.AuthUserID).Scan(&userID)
	if err != nil {
		t.Fatalf("find user: %v", err)
	}

	senior, err := uuid.Parse(seniorID)
	if err != nil {
		t.Fatalf("parse senior id: %v", err)
	}

	if _, err := h.relationships.Create(context.Background(), relationships.CreateParams{
		SeniorID:    senior,
		UserID:      userID,
		Role:        role,
		Permissions: care.Normalise(permissions),
		Status:      care.StatusActive,
	}); err != nil {
		t.Fatalf("join circle: %v", err)
	}
	return userID
}

// createMedication posts a medication with no schedule and returns its ID.
func (h *harness) createMedication(t *testing.T, token, seniorID, name string) string {
	t.Helper()

	rec := h.do(t, token, http.MethodPost, "/v1/seniors/"+seniorID+"/medications", map[string]any{
		"name":   name,
		"dosage": "500 mg",
		"form":   "tablet",
	})
	body := decodeBody(t, rec, http.StatusCreated)

	medication, _ := body["medication"].(map[string]any)
	id, _ := medication["id"].(string)
	if id == "" {
		t.Fatalf("no medication id in response: %s", rec.Body.String())
	}
	return id
}

// addDose posts a single dose at an instant and returns its ID.
func (h *harness) addDose(t *testing.T, token, medicationID string, at time.Time) string {
	t.Helper()

	rec := h.do(t, token, http.MethodPost, "/v1/medications/"+medicationID+"/doses", map[string]any{
		"scheduledFor": at.UTC().Format(time.RFC3339),
	})
	body := decodeBody(t, rec, http.StatusCreated)

	id, _ := body["id"].(string)
	if id == "" {
		t.Fatalf("no dose id in response: %s", rec.Body.String())
	}
	return id
}

func dosePath(medicationID, doseID, action string) string {
	return "/v1/medications/" + medicationID + "/instances/" + doseID + "/" + action
}

// --- Authentication ----------------------------------------------------------

func TestMedicationRoutesRequireAuthentication(t *testing.T) {
	h := newHarness(t)

	medicationID := uuid.New().String()
	doseID := uuid.New().String()

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v1/seniors/" + uuid.New().String() + "/medications"},
		{http.MethodPost, "/v1/seniors/" + uuid.New().String() + "/medications"},
		{http.MethodGet, "/v1/seniors/" + uuid.New().String() + "/medications/doses"},
		{http.MethodGet, "/v1/medications/" + medicationID},
		{http.MethodPatch, "/v1/medications/" + medicationID},
		{http.MethodGet, "/v1/medications/" + medicationID + "/instances"},
		{http.MethodPost, dosePath(medicationID, doseID, "take")},
		{http.MethodPost, dosePath(medicationID, doseID, "skip")},
	}

	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			rec := h.do(t, "", route.method, route.path, nil)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", rec.Code)
			}
		})
	}
}

// --- Authorization -----------------------------------------------------------

// §25: knowing a medication's ID must not be enough to reach it, and the answer
// must not reveal that it exists.
func TestStrangerCannotReachAMedicationByID(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")
	h.account("stranger", "stranger@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "Asia/Karachi")
	medicationID := h.createMedication(t, "owner", seniorID, "Metformin")
	doseID := h.addDose(t, "owner", medicationID, time.Now().Add(time.Hour))

	// The stranger needs an account, but no relationship to this circle.
	h.do(t, "stranger", http.MethodGet, "/v1/seniors", nil)

	cases := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"read it", http.MethodGet, "/v1/medications/" + medicationID, nil},
		{"edit it", http.MethodPatch, "/v1/medications/" + medicationID,
			map[string]any{"name": "Something else"}},
		{"read its schedules", http.MethodGet, "/v1/medications/" + medicationID + "/schedules", nil},
		{"add a time", http.MethodPost, "/v1/medications/" + medicationID + "/schedules",
			map[string]any{"recurrence": map[string]any{"frequency": "daily"}, "scheduledTime": "08:00"}},
		{"read its history", http.MethodGet, "/v1/medications/" + medicationID + "/instances", nil},
		{"take a dose", http.MethodPost, dosePath(medicationID, doseID, "take"), nil},
		{"skip a dose", http.MethodPost, dosePath(medicationID, doseID, "skip"), nil},
		{"list the circle's medications", http.MethodGet,
			"/v1/seniors/" + seniorID + "/medications", nil},
		{"add one", http.MethodPost, "/v1/seniors/" + seniorID + "/medications",
			map[string]any{"name": "Aspirin"}},
		{"read today's doses", http.MethodGet,
			"/v1/seniors/" + seniorID + "/medications/doses", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := h.do(t, "stranger", tc.method, tc.path, tc.body)
			if rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want 404\nbody: %s", rec.Code, rec.Body.String())
			}
			// A 404 that named the medicine would answer the question it is
			// there to refuse.
			if strings.Contains(rec.Body.String(), "Metformin") {
				t.Errorf("the refusal leaked the medication: %s", rec.Body.String())
			}
		})
	}

	// And nothing the stranger tried changed anything.
	body := decodeBody(t,
		h.do(t, "owner", http.MethodGet, "/v1/medications/"+medicationID, nil), http.StatusOK)
	if body["name"] != "Metformin" {
		t.Errorf("name = %v, want it untouched", body["name"])
	}
}

// A caregiver in one circle must not reach another circle's medication.
func TestOneCircleCannotReachAnothers(t *testing.T) {
	h := newHarness(t)
	h.account("first", "first@example.com")
	h.account("second", "second@example.com")

	firstSenior := h.createCircle(t, "first", "Mrs Khan", "Asia/Karachi")
	medicationID := h.createMedication(t, "first", firstSenior, "Metformin")

	// The second account runs a circle of its own, with every permission in it.
	h.createCircle(t, "second", "Mr Ali", "Europe/London")

	rec := h.do(t, "second", http.MethodGet, "/v1/medications/"+medicationID, nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// §9: a caregiver must not receive every medication permission merely because
// they are a caregiver. Permissions belong to the relationship.
func TestViewingDoesNotAllowChangingTheMedication(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")
	h.account("watcher", "watcher@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "Asia/Karachi")
	medicationID := h.createMedication(t, "owner", seniorID, "Metformin")
	doseID := h.addDose(t, "owner", medicationID, time.Now().Add(time.Hour))

	h.join(t, "watcher", seniorID, care.RoleFamilyMember,
		care.PermissionSeniorView, care.PermissionMedicationsView)

	// Reading is allowed.
	if rec := h.do(t, "watcher", http.MethodGet, "/v1/medications/"+medicationID, nil); rec.Code != http.StatusOK {
		t.Fatalf("read: status = %d, want 200", rec.Code)
	}
	if rec := h.do(t, "watcher", http.MethodGet,
		"/v1/medications/"+medicationID+"/instances", nil); rec.Code != http.StatusOK {
		t.Fatalf("history: status = %d, want 200", rec.Code)
	}

	// Changing it, and recording a dose, are not.
	refused := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"edit", http.MethodPatch, "/v1/medications/" + medicationID,
			map[string]any{"dosage": "1000 mg"}},
		{"add a time", http.MethodPost, "/v1/medications/" + medicationID + "/schedules",
			map[string]any{"recurrence": map[string]any{"frequency": "daily"}, "scheduledTime": "08:00"}},
		{"take", http.MethodPost, dosePath(medicationID, doseID, "take"), nil},
		{"skip", http.MethodPost, dosePath(medicationID, doseID, "skip"), nil},
	}

	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			rec := h.do(t, "watcher", tc.method, tc.path, tc.body)
			if rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want 404\nbody: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// The permission a visiting caregiver actually needs: hand somebody their
// tablets without being able to change the prescription (§9).
func TestRecordingIsAllowedWithoutManaging(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")
	h.account("caregiver", "caregiver@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "Asia/Karachi")
	medicationID := h.createMedication(t, "owner", seniorID, "Metformin")
	doseID := h.addDose(t, "owner", medicationID, time.Now().Add(-time.Minute))

	caregiverID := h.join(t, "caregiver", seniorID, care.RoleProfessionalCaregiver,
		care.PermissionSeniorView, care.PermissionMedicationsView, care.PermissionMedicationsRecord)

	body := decodeBody(t,
		h.do(t, "caregiver", http.MethodPost, dosePath(medicationID, doseID, "take"), nil),
		http.StatusOK)

	if body["status"] != "taken" {
		t.Errorf("status = %v, want taken", body["status"])
	}
	// The actor comes from the session, never from the request.
	if body["takenBy"] != caregiverID.String() {
		t.Errorf("takenBy = %v, want the caregiver who was signed in", body["takenBy"])
	}

	// But they still cannot change the medicine itself.
	rec := h.do(t, "caregiver", http.MethodPatch, "/v1/medications/"+medicationID,
		map[string]any{"dosage": "1000 mg"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("edit: status = %d, want 404", rec.Code)
	}
}

// A client that could name the actor could attribute somebody else's medicine
// to them (§6).
func TestTheClientCannotChooseWhoTookIt(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")
	h.account("caregiver", "caregiver@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "Asia/Karachi")
	medicationID := h.createMedication(t, "owner", seniorID, "Metformin")
	doseID := h.addDose(t, "owner", medicationID, time.Now().Add(-time.Minute))

	caregiverID := h.join(t, "caregiver", seniorID, care.RoleProfessionalCaregiver,
		care.PermissionSeniorView, care.PermissionMedicationsView, care.PermissionMedicationsRecord)

	// A body that names somebody else is refused outright rather than quietly
	// ignored: the request asked for something the endpoint will never do, and
	// saying so is better than appearing to have obeyed.
	rec := h.do(t, "caregiver", http.MethodPost, dosePath(medicationID, doseID, "take"),
		map[string]any{"takenBy": uuid.New().String()})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("naming the actor: status = %d, want 400\nbody: %s", rec.Code, rec.Body.String())
	}

	// The only thing the body may carry is a note, and the actor still comes
	// from the session.
	body := decodeBody(t,
		h.do(t, "caregiver", http.MethodPost, dosePath(medicationID, doseID, "take"),
			map[string]any{"notes": "Given with breakfast"}),
		http.StatusOK)

	if body["takenBy"] != caregiverID.String() {
		t.Errorf("takenBy = %v, want the authenticated caller", body["takenBy"])
	}
	if body["notes"] != "Given with breakfast" {
		t.Errorf("notes = %v", body["notes"])
	}
}

// Revoking a membership must end access immediately (§29).
func TestARevokedMemberLosesAccess(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")
	h.account("former", "former@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "Asia/Karachi")
	medicationID := h.createMedication(t, "owner", seniorID, "Metformin")

	userID := h.join(t, "former", seniorID, care.RoleProfessionalCaregiver,
		care.PermissionSeniorView, care.PermissionMedicationsView, care.PermissionMedicationsRecord)

	if rec := h.do(t, "former", http.MethodGet,
		"/v1/medications/"+medicationID, nil); rec.Code != http.StatusOK {
		t.Fatalf("before revocation: status = %d, want 200", rec.Code)
	}

	if _, err := h.pool.Exec(context.Background(),
		`UPDATE care_relationships SET status = 'revoked' WHERE user_id = $1`, userID,
	); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	if rec := h.do(t, "former", http.MethodGet,
		"/v1/medications/"+medicationID, nil); rec.Code != http.StatusNotFound {
		t.Errorf("after revocation: status = %d, want 404", rec.Code)
	}
}

// A dose belonging to a different medicine must not be reachable through this
// one, even by somebody who may record doses here.
func TestADoseCannotBeRecordedThroughAnotherMedication(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "Asia/Karachi")
	metformin := h.createMedication(t, "owner", seniorID, "Metformin")
	vitaminD := h.createMedication(t, "owner", seniorID, "Vitamin D")

	doseID := h.addDose(t, "owner", vitaminD, time.Now().Add(-time.Minute))

	rec := h.do(t, "owner", http.MethodPost, dosePath(metformin, doseID, "take"), nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// --- Behaviour over HTTP ------------------------------------------------------

func TestCreatingAMedicationWithTimesReturnsItsDoses(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "Asia/Karachi")

	body := decodeBody(t,
		h.do(t, "owner", http.MethodPost, "/v1/seniors/"+seniorID+"/medications",
			map[string]any{
				"name":         "Metformin",
				"dosage":       "500 mg",
				"form":         "tablet",
				"instructions": "With food",
				"schedules": []any{
					map[string]any{
						"recurrence":    map[string]any{"frequency": "daily"},
						"scheduledTime": "08:00",
					},
				},
			}),
		http.StatusCreated)

	medication, _ := body["medication"].(map[string]any)
	schedules, _ := medication["schedules"].([]any)
	if len(schedules) != 1 {
		t.Fatalf("got %d schedules, want 1: %v", len(schedules), medication)
	}

	doses, _ := body["doses"].([]any)
	if len(doses) == 0 {
		t.Error("a daily schedule produced no doses")
	}

	// The stored RRULE never reaches a client: the repeat rule travels as
	// structured data the app turns into words (§31, plans/phase4.md §21).
	medicationID, _ := medication["id"].(string)
	for _, path := range []string{
		"/v1/medications/" + medicationID,
		"/v1/medications/" + medicationID + "/schedules",
		"/v1/seniors/" + seniorID + "/medications/doses",
	} {
		if strings.Contains(rawBody(t, h, "owner", path), "FREQ=") {
			t.Errorf("%s leaked the internal rule format", path)
		}
	}
}

func TestAnInvalidScheduleIsRefused(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "Asia/Karachi")

	cases := []struct {
		name     string
		schedule map[string]any
	}{
		{"a frequency nobody supports", map[string]any{
			"recurrence":    map[string]any{"frequency": "hourly"},
			"scheduledTime": "08:00",
		}},
		{"a weekly rule with no days", map[string]any{
			"recurrence":    map[string]any{"frequency": "weekly", "weekdays": []any{}},
			"scheduledTime": "08:00",
		}},
		{"a day that is not a day", map[string]any{
			"recurrence":    map[string]any{"frequency": "weekly", "weekdays": []any{"someday"}},
			"scheduledTime": "08:00",
		}},
		{"a time that is not a time", map[string]any{
			"recurrence":    map[string]any{"frequency": "daily"},
			"scheduledTime": "25:00",
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := h.do(t, "owner", http.MethodPost, "/v1/seniors/"+seniorID+"/medications",
				map[string]any{"name": "Metformin", "schedules": []any{tc.schedule}})
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("status = %d, want 422\nbody: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestAnUnrecognisedFormIsRefused(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "Asia/Karachi")

	rec := h.do(t, "owner", http.MethodPost, "/v1/seniors/"+seniorID+"/medications",
		map[string]any{"name": "Metformin", "form": "elixir of life"})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422\nbody: %s", rec.Code, rec.Body.String())
	}
}

func TestAMedicationNeedsAName(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "Asia/Karachi")

	rec := h.do(t, "owner", http.MethodPost, "/v1/seniors/"+seniorID+"/medications",
		map[string]any{"name": "   "})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422\nbody: %s", rec.Code, rec.Body.String())
	}
}

// §21: a replayed mutation succeeds. §22: a contradictory one is refused.
func TestTakingTwiceSucceedsAndSkippingAfterwardsConflicts(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "Asia/Karachi")
	medicationID := h.createMedication(t, "owner", seniorID, "Metformin")
	doseID := h.addDose(t, "owner", medicationID, time.Now().Add(-time.Minute))

	first := decodeBody(t,
		h.do(t, "owner", http.MethodPost, dosePath(medicationID, doseID, "take"), nil),
		http.StatusOK)
	second := decodeBody(t,
		h.do(t, "owner", http.MethodPost, dosePath(medicationID, doseID, "take"), nil),
		http.StatusOK)

	if first["takenAt"] != second["takenAt"] {
		t.Error("a replayed take moved the time the medicine was taken")
	}

	rec := h.do(t, "owner", http.MethodPost, dosePath(medicationID, doseID, "skip"), nil)
	if rec.Code != http.StatusConflict {
		t.Errorf("skip after take: status = %d, want 409\nbody: %s", rec.Code, rec.Body.String())
	}
}

func TestADosePastItsWindowIsReportedMissedOverHTTP(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "Asia/Karachi")
	medicationID := h.createMedication(t, "owner", seniorID, "Metformin")
	h.addDose(t, "owner", medicationID, time.Now().Add(-4*time.Hour))

	body := decodeBody(t,
		h.do(t, "owner", http.MethodGet,
			"/v1/seniors/"+seniorID+"/medications/doses?scope=missed", nil),
		http.StatusOK)

	items, _ := body["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("got %d missed doses, want 1: %s", len(items), body)
	}

	dose, _ := items[0].(map[string]any)
	if dose["status"] != "missed" {
		t.Errorf("status = %v, want missed", dose["status"])
	}
}

func TestStoppingAMedicationKeepsItReadable(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "Asia/Karachi")
	medicationID := h.createMedication(t, "owner", seniorID, "Metformin")
	doseID := h.addDose(t, "owner", medicationID, time.Now().Add(-time.Minute))

	decodeBody(t,
		h.do(t, "owner", http.MethodPost, dosePath(medicationID, doseID, "take"), nil),
		http.StatusOK)

	stopped := decodeBody(t,
		h.do(t, "owner", http.MethodPatch, "/v1/medications/"+medicationID,
			map[string]any{"active": false}),
		http.StatusOK)

	if stopped["active"] != false {
		t.Errorf("active = %v, want false", stopped["active"])
	}
	if stopped["nextDoseAt"] != nil {
		t.Errorf("nextDoseAt = %v, want null for a stopped medicine", stopped["nextDoseAt"])
	}

	// It is still in the list, and its history is intact.
	list := decodeBody(t,
		h.do(t, "owner", http.MethodGet, "/v1/seniors/"+seniorID+"/medications", nil),
		http.StatusOK)
	items, _ := list["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("got %d medications after stopping, want 1", len(items))
	}

	history := decodeBody(t,
		h.do(t, "owner", http.MethodGet, "/v1/medications/"+medicationID+"/instances", nil),
		http.StatusOK)
	doses, _ := history["items"].([]any)
	if len(doses) != 1 {
		t.Fatalf("got %d doses in history, want the taken one kept", len(doses))
	}
	if doses[0].(map[string]any)["status"] != "taken" {
		t.Errorf("history status = %v, want taken", doses[0].(map[string]any)["status"])
	}
}

func TestHistoryIsPagedOverHTTP(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "Asia/Karachi")
	medicationID := h.createMedication(t, "owner", seniorID, "Metformin")

	now := time.Now()
	for day := 1; day <= 3; day++ {
		h.addDose(t, "owner", medicationID, now.AddDate(0, 0, -day))
	}

	first := decodeBody(t,
		h.do(t, "owner", http.MethodGet,
			"/v1/medications/"+medicationID+"/instances?limit=2", nil),
		http.StatusOK)

	items, _ := first["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("got %d doses, want 2", len(items))
	}
	cursor, _ := first["nextCursor"].(string)
	if cursor == "" {
		t.Fatal("no cursor for a history with more to read")
	}

	second := decodeBody(t,
		h.do(t, "owner", http.MethodGet,
			"/v1/medications/"+medicationID+"/instances?limit=2&cursor="+cursor, nil),
		http.StatusOK)

	rest, _ := second["items"].([]any)
	if len(rest) != 1 {
		t.Fatalf("got %d doses on the last page, want 1", len(rest))
	}
	if second["nextCursor"] != nil {
		t.Errorf("nextCursor = %v, want null at the end", second["nextCursor"])
	}
}

func TestANonsenseCursorIsRefused(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "Asia/Karachi")
	medicationID := h.createMedication(t, "owner", seniorID, "Metformin")

	rec := h.do(t, "owner", http.MethodGet,
		"/v1/medications/"+medicationID+"/instances?cursor=nonsense", nil)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400\nbody: %s", rec.Code, rec.Body.String())
	}
}

// rawBody fetches a path and returns the response body as text, for assertions
// about what a response must not contain.
func rawBody(t *testing.T, h *harness, token, path string) string {
	t.Helper()

	rec := h.do(t, token, http.MethodGet, path, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status = %d", path, rec.Code)
	}
	return rec.Body.String()
}
