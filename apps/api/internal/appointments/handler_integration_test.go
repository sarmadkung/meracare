package appointments_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
// who may only view appointments cancel one — are answered by the middleware
// stack and the handlers together, and only the whole stack can be trusted to
// answer them.

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

// bookAppointment posts an appointment and returns its ID.
func (h *harness) bookAppointment(
	t *testing.T,
	token, seniorID, title string,
	at time.Time,
) string {
	t.Helper()

	rec := h.do(t, token, http.MethodPost, "/v1/seniors/"+seniorID+"/appointments", map[string]any{
		"title":        title,
		"scheduledAt":  at.UTC().Format(time.RFC3339),
		"providerName": "Dr Ahmed",
		"location":     "City Hospital",
	})
	body := decodeBody(t, rec, http.StatusCreated)

	id, _ := body["id"].(string)
	if id == "" {
		t.Fatalf("no appointment id in response: %s", rec.Body.String())
	}
	return id
}

func itemTitles(t *testing.T, body map[string]any) []string {
	t.Helper()

	items, _ := body["items"].([]any)
	names := make([]string, 0, len(items))
	for _, item := range items {
		appointment, _ := item.(map[string]any)
		title, _ := appointment["title"].(string)
		names = append(names, title)
	}
	return names
}

// --- Authentication ----------------------------------------------------------

func TestAppointmentRoutesRequireAuthentication(t *testing.T) {
	h := newHarness(t)

	seniorID := uuid.New().String()
	appointmentID := uuid.New().String()

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v1/seniors/" + seniorID + "/appointments"},
		{http.MethodPost, "/v1/seniors/" + seniorID + "/appointments"},
		{http.MethodGet, "/v1/appointments/" + appointmentID},
		{http.MethodPatch, "/v1/appointments/" + appointmentID},
		{http.MethodPost, "/v1/appointments/" + appointmentID + "/cancel"},
		{http.MethodPost, "/v1/appointments/" + appointmentID + "/complete"},
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

// Somebody with no relationship to the senior must not be able to tell an
// appointment that exists from one that does not (plans/phase6.md §12).
func TestAStrangerSeesTheSameAnswerForRealAndImaginaryAppointments(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")
	h.account("stranger", "stranger@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "Asia/Karachi")
	appointmentID := h.bookAppointment(t, "owner", seniorID, "Cardiology review",
		time.Now().Add(24*time.Hour))

	// The stranger must exist as a user, so this is not merely an unknown token.
	if rec := h.do(t, "stranger", http.MethodGet, "/v1/seniors", nil); rec.Code != http.StatusOK {
		t.Fatalf("sign in: status = %d", rec.Code)
	}

	real := h.do(t, "stranger", http.MethodGet, "/v1/appointments/"+appointmentID, nil)
	imaginary := h.do(t, "stranger", http.MethodGet, "/v1/appointments/"+uuid.New().String(), nil)

	if real.Code != http.StatusNotFound {
		t.Errorf("real appointment: status = %d, want 404", real.Code)
	}
	if imaginary.Code != http.StatusNotFound {
		t.Errorf("imaginary appointment: status = %d, want 404", imaginary.Code)
	}
	if real.Body.String() != imaginary.Body.String() {
		t.Errorf("the two answers differ, which reveals that the appointment exists:\n"+
			" real: %s\nimaginary: %s", real.Body.String(), imaginary.Body.String())
	}
}

// A member whose relationship has been revoked loses access immediately, even
// though the row survives to keep their past entries attributed.
func TestARevokedMemberLosesAccessToAppointments(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")
	h.account("former", "former@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "Asia/Karachi")
	h.join(t, "former", seniorID, care.RoleFamilyMember,
		care.PermissionAppointmentsView, care.PermissionAppointmentsManage)

	appointmentID := h.bookAppointment(t, "former", seniorID, "Blood test",
		time.Now().Add(24*time.Hour))

	var relationshipID uuid.UUID
	claims := h.verifier.accounts["former"]
	err := h.pool.QueryRow(context.Background(), `
		SELECT cr.id FROM care_relationships cr
		JOIN users u ON u.id = cr.user_id
		WHERE u.auth_user_id = $1`, claims.AuthUserID).Scan(&relationshipID)
	if err != nil {
		t.Fatalf("find relationship: %v", err)
	}
	if _, err := h.relationships.RevokeMembership(context.Background(), relationshipID); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	if rec := h.do(t, "former", http.MethodGet, "/v1/appointments/"+appointmentID, nil); rec.Code != http.StatusNotFound {
		t.Errorf("read after revocation: status = %d, want 404", rec.Code)
	}
	if rec := h.do(t, "former", http.MethodGet, "/v1/seniors/"+seniorID+"/appointments", nil); rec.Code != http.StatusNotFound {
		t.Errorf("list after revocation: status = %d, want 404", rec.Code)
	}
}

// The permission split docs/02 defines: view lets somebody read the calendar,
// manage lets them change it. A caregiver with only view can see an appointment
// and do nothing to it.
func TestViewingDoesNotCarryTheRightToChange(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")
	h.account("caregiver", "caregiver@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "Asia/Karachi")
	h.join(t, "caregiver", seniorID, care.RoleProfessionalCaregiver,
		care.PermissionSeniorView, care.PermissionAppointmentsView)

	appointmentID := h.bookAppointment(t, "owner", seniorID, "Physiotherapy",
		time.Now().Add(24*time.Hour))

	if rec := h.do(t, "caregiver", http.MethodGet, "/v1/appointments/"+appointmentID, nil); rec.Code != http.StatusOK {
		t.Fatalf("read with appointments.view: status = %d, want 200\nbody: %s",
			rec.Code, rec.Body.String())
	}

	refused := map[string]*httptest.ResponseRecorder{
		"create": h.do(t, "caregiver", http.MethodPost, "/v1/seniors/"+seniorID+"/appointments",
			map[string]any{"title": "Sneaky", "scheduledAt": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}),
		"edit":     h.do(t, "caregiver", http.MethodPatch, "/v1/appointments/"+appointmentID, map[string]any{"title": "Moved"}),
		"cancel":   h.do(t, "caregiver", http.MethodPost, "/v1/appointments/"+appointmentID+"/cancel", nil),
		"complete": h.do(t, "caregiver", http.MethodPost, "/v1/appointments/"+appointmentID+"/complete", nil),
	}

	for action, rec := range refused {
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s without appointments.manage: status = %d, want 404\nbody: %s",
				action, rec.Code, rec.Body.String())
		}
	}

	// And nothing actually changed.
	body := decodeBody(t, h.do(t, "owner", http.MethodGet, "/v1/appointments/"+appointmentID, nil), http.StatusOK)
	if body["title"] != "Physiotherapy" || body["status"] != "scheduled" {
		t.Errorf("the appointment was modified by somebody who may only view it: %v", body)
	}
}

// --- Creating ----------------------------------------------------------------

// The creator comes from the session. A body that tries to name one is rejected
// outright rather than quietly ignored (plans/phase6.md §13).
func TestTheClientCannotChooseWhoBookedIt(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")
	h.account("other", "other@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "Asia/Karachi")
	otherID := h.join(t, "other", seniorID, care.RoleFamilyMember, care.PermissionSeniorView)

	at := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)

	named := h.do(t, "owner", http.MethodPost, "/v1/seniors/"+seniorID+"/appointments",
		map[string]any{"title": "Cardiology", "scheduledAt": at, "createdBy": otherID.String()})
	if named.Code != http.StatusBadRequest {
		t.Errorf("a body naming createdBy: status = %d, want 400\nbody: %s",
			named.Code, named.Body.String())
	}

	body := decodeBody(t,
		h.do(t, "owner", http.MethodPost, "/v1/seniors/"+seniorID+"/appointments",
			map[string]any{"title": "Cardiology", "scheduledAt": at}),
		http.StatusCreated)

	if body["createdBy"] == otherID.String() {
		t.Error("the appointment was attributed to somebody who did not book it")
	}
}

func TestAnUnusableDateOrTimeIsRefused(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")
	seniorID := h.createCircle(t, "owner", "Mrs Khan", "Asia/Karachi")

	start := time.Now().Add(24 * time.Hour)

	cases := map[string]map[string]any{
		"no time at all":      {"title": "Cardiology"},
		"not a timestamp":     {"title": "Cardiology", "scheduledAt": "next Tuesday"},
		"a date with no time": {"title": "Cardiology", "scheduledAt": "2026-08-20"},
		"ending before it has begun": {
			"title":       "Cardiology",
			"scheduledAt": start.UTC().Format(time.RFC3339),
			"endsAt":      start.Add(-time.Hour).UTC().Format(time.RFC3339),
		},
		"ending exactly when it starts": {
			"title":       "Cardiology",
			"scheduledAt": start.UTC().Format(time.RFC3339),
			"endsAt":      start.UTC().Format(time.RFC3339),
		},
		"no title": {"scheduledAt": start.UTC().Format(time.RFC3339)},
	}

	// 422 rather than 400: the body was readable JSON, and the field-level
	// details are what let the form highlight the row that is wrong.
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			rec := h.do(t, "owner", http.MethodPost, "/v1/seniors/"+seniorID+"/appointments", body)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("status = %d, want 422\nbody: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// An appointment cannot be booked into a circle the caller cannot reach, and
// the refusal reveals nothing about whether the circle exists.
func TestBookingIntoAnotherCircleIsRefused(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")
	h.account("stranger", "stranger@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "Asia/Karachi")
	if rec := h.do(t, "stranger", http.MethodGet, "/v1/seniors", nil); rec.Code != http.StatusOK {
		t.Fatalf("sign in: status = %d", rec.Code)
	}

	rec := h.do(t, "stranger", http.MethodPost, "/v1/seniors/"+seniorID+"/appointments",
		map[string]any{
			"title":       "Cardiology",
			"scheduledAt": time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339),
		})
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// --- Listing -----------------------------------------------------------------

func TestTheListOffersUpcomingTodayAndThePast(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "Asia/Karachi")
	now := time.Now()

	h.bookAppointment(t, "owner", seniorID, "Next week", now.Add(7*24*time.Hour))
	h.bookAppointment(t, "owner", seniorID, "Last week", now.Add(-7*24*time.Hour))

	upcoming := decodeBody(t,
		h.do(t, "owner", http.MethodGet, "/v1/seniors/"+seniorID+"/appointments", nil),
		http.StatusOK)
	if got := itemTitles(t, upcoming); len(got) != 1 || got[0] != "Next week" {
		t.Errorf("default view = %v, want just the upcoming appointment", got)
	}
	if upcoming["nextCursor"] != nil {
		t.Errorf("nextCursor = %v, want null for an unpaged view", upcoming["nextCursor"])
	}

	past := decodeBody(t,
		h.do(t, "owner", http.MethodGet, "/v1/seniors/"+seniorID+"/appointments?scope=past", nil),
		http.StatusOK)
	if got := itemTitles(t, past); len(got) != 1 || got[0] != "Last week" {
		t.Errorf("past view = %v, want just the past appointment", got)
	}

	if rec := h.do(t, "owner", http.MethodGet, "/v1/seniors/"+seniorID+"/appointments?scope=today", nil); rec.Code != http.StatusOK {
		t.Errorf("today view: status = %d, want 200\nbody: %s", rec.Code, rec.Body.String())
	}

	if rec := h.do(t, "owner", http.MethodGet, "/v1/seniors/"+seniorID+"/appointments?scope=everything", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown view: status = %d, want 400", rec.Code)
	}
}

func TestAnUnreadableCursorIsRefusedRatherThanRestarted(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")
	seniorID := h.createCircle(t, "owner", "Mrs Khan", "Asia/Karachi")

	rec := h.do(t, "owner", http.MethodGet,
		"/v1/seniors/"+seniorID+"/appointments?scope=past&cursor=not-a-cursor", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400\nbody: %s", rec.Code, rec.Body.String())
	}
}

// --- Settling ----------------------------------------------------------------

func TestCancellingAndCompletingOverHTTP(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")
	seniorID := h.createCircle(t, "owner", "Mrs Khan", "Asia/Karachi")

	completed := h.bookAppointment(t, "owner", seniorID, "Blood test", time.Now().Add(-2*time.Hour))
	body := decodeBody(t,
		h.do(t, "owner", http.MethodPost, "/v1/appointments/"+completed+"/complete", nil),
		http.StatusOK)
	if body["status"] != "completed" {
		t.Errorf("status = %v, want completed", body["status"])
	}
	if body["completedBy"] == nil || body["completedAt"] == nil {
		t.Errorf("completion was not attributed: %v", body)
	}

	// Sending it again is a retry, not a conflict.
	if rec := h.do(t, "owner", http.MethodPost, "/v1/appointments/"+completed+"/complete", nil); rec.Code != http.StatusOK {
		t.Errorf("repeat completion: status = %d, want 200\nbody: %s", rec.Code, rec.Body.String())
	}

	// Cancelling something already completed is refused, and says so.
	if rec := h.do(t, "owner", http.MethodPost, "/v1/appointments/"+completed+"/cancel", nil); rec.Code != http.StatusConflict {
		t.Errorf("contradictory outcome: status = %d, want 409\nbody: %s", rec.Code, rec.Body.String())
	}
}

func TestEditingASettledAppointmentIsRefusedOverHTTP(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")
	seniorID := h.createCircle(t, "owner", "Mrs Khan", "Asia/Karachi")

	appointmentID := h.bookAppointment(t, "owner", seniorID, "Dentist", time.Now().Add(24*time.Hour))
	decodeBody(t, h.do(t, "owner", http.MethodPost, "/v1/appointments/"+appointmentID+"/cancel", nil),
		http.StatusOK)

	rec := h.do(t, "owner", http.MethodPatch, "/v1/appointments/"+appointmentID,
		map[string]any{"title": "Rewritten"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409\nbody: %s", rec.Code, rec.Body.String())
	}

	body := decodeBody(t, h.do(t, "owner", http.MethodGet, "/v1/appointments/"+appointmentID, nil),
		http.StatusOK)
	if body["title"] != "Dentist" {
		t.Errorf("title = %v, want the original", body["title"])
	}
}

// --- Editing -----------------------------------------------------------------

func TestEditingOverHTTPMovesTheTimeAndKeepsTheRest(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")
	seniorID := h.createCircle(t, "owner", "Mrs Khan", "Asia/Karachi")

	start := time.Now().Add(24 * time.Hour).Truncate(time.Second)
	appointmentID := h.bookAppointment(t, "owner", seniorID, "Cardiology review", start)

	moved := start.Add(90 * time.Minute)
	body := decodeBody(t,
		h.do(t, "owner", http.MethodPatch, "/v1/appointments/"+appointmentID, map[string]any{
			"scheduledAt": moved.UTC().Format(time.RFC3339),
			"endsAt":      moved.Add(time.Hour).UTC().Format(time.RFC3339),
		}),
		http.StatusOK)

	if body["scheduledAt"] != moved.UTC().Format(time.RFC3339) {
		t.Errorf("scheduledAt = %v, want %v", body["scheduledAt"], moved.UTC().Format(time.RFC3339))
	}
	if body["title"] != "Cardiology review" || body["providerName"] != "Dr Ahmed" {
		t.Errorf("an edit that moved the time changed something else: %v", body)
	}
}

// Moving an appointment forward past its existing end time must succeed: the
// end is checked against the start the appointment will have, not the one it is
// being moved away from.
func TestMovingAnAppointmentPastItsOldEndTimeIsAllowed(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")
	seniorID := h.createCircle(t, "owner", "Mrs Khan", "Asia/Karachi")

	start := time.Now().Add(24 * time.Hour).Truncate(time.Second)
	rec := h.do(t, "owner", http.MethodPost, "/v1/seniors/"+seniorID+"/appointments", map[string]any{
		"title":       "Physiotherapy",
		"scheduledAt": start.UTC().Format(time.RFC3339),
		"endsAt":      start.Add(time.Hour).UTC().Format(time.RFC3339),
	})
	created := decodeBody(t, rec, http.StatusCreated)
	appointmentID, _ := created["id"].(string)

	moved := start.Add(4 * time.Hour)
	body := decodeBody(t,
		h.do(t, "owner", http.MethodPatch, "/v1/appointments/"+appointmentID, map[string]any{
			"scheduledAt": moved.UTC().Format(time.RFC3339),
			"endsAt":      moved.Add(time.Hour).UTC().Format(time.RFC3339),
		}),
		http.StatusOK)

	if body["scheduledAt"] != moved.UTC().Format(time.RFC3339) {
		t.Errorf("scheduledAt = %v, want %v", body["scheduledAt"], moved.UTC().Format(time.RFC3339))
	}
}
