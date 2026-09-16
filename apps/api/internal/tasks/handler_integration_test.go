package tasks_test

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
// reach a task by its ID — are answered by the middleware stack and the
// handlers together, and only the whole stack can be trusted to answer them.

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

func (h *harness) do(t *testing.T, token, method, path string, body any) *httptest.ResponseRecorder {
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

// decode reads a JSON response body, failing the test on unexpected statuses.
func decode(t *testing.T, rec *httptest.ResponseRecorder, want int) map[string]any {
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
	body := decode(t, rec, http.StatusCreated)

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

// createTask posts a one-time task and returns its ID.
func (h *harness) createTask(t *testing.T, token, seniorID string, due time.Time) string {
	t.Helper()

	rec := h.do(t, token, http.MethodPost, "/v1/seniors/"+seniorID+"/tasks", map[string]any{
		"title":        "Check blood pressure",
		"scheduledFor": due.UTC().Format(time.RFC3339),
	})
	body := decode(t, rec, http.StatusCreated)

	items, _ := body["tasks"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected one task, got: %s", rec.Body.String())
	}
	task, _ := items[0].(map[string]any)
	id, _ := task["id"].(string)
	if id == "" {
		t.Fatalf("no task id in response: %s", rec.Body.String())
	}
	return id
}

// --- Authorization ----------------------------------------------------------

func TestTaskRoutesRequireAuthentication(t *testing.T) {
	h := newHarness(t)

	paths := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v1/tasks"},
		{http.MethodGet, "/v1/tasks/" + uuid.New().String()},
		{http.MethodPost, "/v1/tasks/" + uuid.New().String() + "/complete"},
		{http.MethodPost, "/v1/tasks/" + uuid.New().String() + "/skip"},
		{http.MethodGet, "/v1/seniors/" + uuid.New().String() + "/tasks"},
	}

	for _, route := range paths {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			rec := h.do(t, "", route.method, route.path, nil)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", rec.Code)
			}
		})
	}
}

// §16: knowing a task's ID must not be enough to reach it.
func TestIntruderCannotReachATaskByID(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")
	h.account("intruder", "intruder@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "Asia/Karachi")
	taskID := h.createTask(t, "owner", seniorID, time.Now().Add(time.Hour))

	// The intruder needs an account, but no relationship to this circle.
	h.do(t, "intruder", http.MethodGet, "/v1/seniors", nil)

	cases := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"read", http.MethodGet, "/v1/tasks/" + taskID, nil},
		{"edit", http.MethodPatch, "/v1/tasks/" + taskID, map[string]any{"title": "Mine now"}},
		{"complete", http.MethodPost, "/v1/tasks/" + taskID + "/complete", nil},
		{"skip", http.MethodPost, "/v1/tasks/" + taskID + "/skip", nil},
		{"cancel", http.MethodDelete, "/v1/tasks/" + taskID, nil},
		{"list", http.MethodGet, "/v1/seniors/" + seniorID + "/tasks", nil},
		{"create", http.MethodPost, "/v1/seniors/" + seniorID + "/tasks", map[string]any{
			"title": "Mine", "scheduledFor": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := h.do(t, "intruder", tc.method, tc.path, tc.body)
			// 404 rather than 403: a different answer would confirm the task
			// exists, letting somebody map other people's care by probing IDs.
			if rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want 404\nbody: %s", rec.Code, rec.Body.String())
			}
		})
	}

	// And the task is untouched.
	rec := h.do(t, "owner", http.MethodGet, "/v1/tasks/"+taskID, nil)
	body := decode(t, rec, http.StatusOK)
	if body["status"] != "pending" {
		t.Errorf("status = %v, want pending", body["status"])
	}
	if body["title"] != "Check blood pressure" {
		t.Errorf("title = %v, want the original", body["title"])
	}
}

// A caregiver may carry out care without being able to restructure the
// schedule: tasks.complete without tasks.manage.
func TestCaregiverCanCompleteButNotManage(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")
	h.account("caregiver", "caregiver@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "Asia/Karachi")
	h.join(t, "caregiver", seniorID, care.RoleProfessionalCaregiver,
		care.PermissionSeniorView, care.PermissionTasksView, care.PermissionTasksComplete)

	taskID := h.createTask(t, "owner", seniorID, time.Now().Add(time.Hour))

	// Reading and completing are allowed.
	decode(t, h.do(t, "caregiver", http.MethodGet, "/v1/tasks/"+taskID, nil), http.StatusOK)
	completed := decode(t,
		h.do(t, "caregiver", http.MethodPost, "/v1/tasks/"+taskID+"/complete", nil), http.StatusOK)

	if completed["status"] != "completed" {
		t.Errorf("status = %v, want completed", completed["status"])
	}

	// Creating and editing are not — and are refused as 404, matching the
	// treatment of a senior the caller cannot see at all.
	refused := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"create", http.MethodPost, "/v1/seniors/" + seniorID + "/tasks", map[string]any{
			"title": "Extra", "scheduledFor": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		}},
		{"edit", http.MethodPatch, "/v1/tasks/" + taskID, map[string]any{"title": "Renamed"}},
		{"cancel", http.MethodDelete, "/v1/tasks/" + taskID, nil},
	}

	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			rec := h.do(t, "caregiver", tc.method, tc.path, tc.body)
			if rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want 404\nbody: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// tasks.view alone must not allow recording care.
func TestViewerCannotCompleteATask(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")
	h.account("viewer", "viewer@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "UTC")
	h.join(t, "viewer", seniorID, care.RoleFamilyMember,
		care.PermissionSeniorView, care.PermissionTasksView)

	taskID := h.createTask(t, "owner", seniorID, time.Now().Add(time.Hour))

	// Reading is fine.
	decode(t, h.do(t, "viewer", http.MethodGet, "/v1/tasks/"+taskID, nil), http.StatusOK)

	for _, action := range []string{"complete", "skip"} {
		t.Run(action, func(t *testing.T) {
			rec := h.do(t, "viewer", http.MethodPost, "/v1/tasks/"+taskID+"/"+action, nil)
			if rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want 404\nbody: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// A task in one circle must not be reachable through another circle the caller
// does belong to.
func TestTaskFromAnotherCircleIsUnreachable(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")
	h.account("other", "other@example.com")

	theirs := h.createCircle(t, "owner", "Mrs Khan", "UTC")
	taskID := h.createTask(t, "owner", theirs, time.Now().Add(time.Hour))

	// The other account runs a circle of their own, with full permissions in it.
	mine := h.createCircle(t, "other", "Mr Ali", "UTC")

	rec := h.do(t, "other", http.MethodGet, "/v1/tasks/"+taskID, nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}

	// Nor by naming their own senior in the path.
	rec = h.do(t, "other", http.MethodGet, "/v1/seniors/"+mine+"/tasks", nil)
	body := decode(t, rec, http.StatusOK)
	if items, _ := body["items"].([]any); len(items) != 0 {
		t.Errorf("got %d tasks in an empty circle", len(items))
	}
}

// --- Assignment -------------------------------------------------------------

// §17: the backend validates the assignee; a client cannot name anybody it likes.
func TestAssigningToSomebodyOutsideTheCircleIsRefused(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")
	h.account("stranger", "stranger@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "UTC")

	// Give the stranger an account, but no membership.
	h.do(t, "stranger", http.MethodGet, "/v1/seniors", nil)
	var strangerID uuid.UUID
	claims := h.verifier.accounts["stranger"]
	if err := h.pool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE auth_user_id = $1`, claims.AuthUserID).Scan(&strangerID); err != nil {
		t.Fatalf("find stranger: %v", err)
	}

	rec := h.do(t, "owner", http.MethodPost, "/v1/seniors/"+seniorID+"/tasks", map[string]any{
		"title":          "Morning visit",
		"scheduledFor":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		"assignedUserId": strangerID.String(),
	})

	body := decode(t, rec, http.StatusUnprocessableEntity)
	details, _ := body["error"].(map[string]any)["details"].(map[string]any)
	if details["assignedUserId"] == nil {
		t.Errorf("expected a field error on assignedUserId, got: %s", rec.Body.String())
	}
}

// --- Idempotency and conflict ----------------------------------------------

// §27: the offline queue retries, and a retry must not be an error.
func TestCompletingTwiceOverHTTPSucceedsBothTimes(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "UTC")
	taskID := h.createTask(t, "owner", seniorID, time.Now().Add(time.Hour))

	first := decode(t,
		h.do(t, "owner", http.MethodPost, "/v1/tasks/"+taskID+"/complete", nil), http.StatusOK)
	second := decode(t,
		h.do(t, "owner", http.MethodPost, "/v1/tasks/"+taskID+"/complete", nil), http.StatusOK)

	if first["completedAt"] != second["completedAt"] {
		t.Error("the retry moved the completion timestamp")
	}
	if second["status"] != "completed" {
		t.Errorf("status = %v, want completed", second["status"])
	}
}

// §28: a different outcome is refused, and the first one stands.
func TestSkippingACompletedTaskConflicts(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "UTC")
	taskID := h.createTask(t, "owner", seniorID, time.Now().Add(time.Hour))

	decode(t, h.do(t, "owner", http.MethodPost, "/v1/tasks/"+taskID+"/complete", nil), http.StatusOK)

	rec := h.do(t, "owner", http.MethodPost, "/v1/tasks/"+taskID+"/skip", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409\nbody: %s", rec.Code, rec.Body.String())
	}

	after := decode(t, h.do(t, "owner", http.MethodGet, "/v1/tasks/"+taskID, nil), http.StatusOK)
	if after["status"] != "completed" {
		t.Errorf("status = %v, want the completion to stand", after["status"])
	}
}

// --- Views ------------------------------------------------------------------

// §12: a task past its due time reads as overdue without anything being written.
func TestOverdueIsReportedOverHTTP(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "UTC")
	taskID := h.createTask(t, "owner", seniorID, time.Now().Add(-2*time.Hour))

	body := decode(t, h.do(t, "owner", http.MethodGet, "/v1/tasks/"+taskID, nil), http.StatusOK)
	if body["status"] != "overdue" {
		t.Errorf("status = %v, want overdue", body["status"])
	}

	overdue := decode(t,
		h.do(t, "owner", http.MethodGet, "/v1/seniors/"+seniorID+"/tasks?scope=overdue", nil),
		http.StatusOK)
	items, _ := overdue["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("got %d overdue tasks, want 1", len(items))
	}
}

func TestRecurringTaskAppearsOnTodaysList(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "UTC")

	// Due late enough that it is still ahead whenever this test runs.
	rec := h.do(t, "owner", http.MethodPost, "/v1/seniors/"+seniorID+"/tasks", map[string]any{
		"title":      "Morning walk",
		"recurrence": map[string]any{"frequency": "daily"},
		"dueTime":    "23:59",
	})
	created := decode(t, rec, http.StatusCreated)

	if created["template"] == nil {
		t.Fatal("a recurring task should return its template")
	}

	today := decode(t,
		h.do(t, "owner", http.MethodGet, "/v1/seniors/"+seniorID+"/tasks?scope=today", nil),
		http.StatusOK)
	items, _ := today["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("got %d tasks today, want 1\nbody: %s", len(items), rec.Body.String())
	}

	task, _ := items[0].(map[string]any)
	if task["recurring"] != true {
		t.Error("the occurrence did not report itself recurring")
	}
	if task["title"] != "Morning walk" {
		t.Errorf("title = %v", task["title"])
	}
}

// The internal RRULE string must never reach a client.
func TestRecurrenceIsReturnedStructuredNotAsARule(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "UTC")

	rec := h.do(t, "owner", http.MethodPost, "/v1/seniors/"+seniorID+"/tasks", map[string]any{
		"title": "Physio",
		"recurrence": map[string]any{
			"frequency": "weekly",
			"weekdays":  []string{"monday", "thursday"},
		},
		"dueTime": "10:30",
	})
	created := decode(t, rec, http.StatusCreated)

	template, _ := created["template"].(map[string]any)
	recurrence, _ := template["recurrence"].(map[string]any)

	if recurrence["frequency"] != "weekly" {
		t.Errorf("frequency = %v, want weekly", recurrence["frequency"])
	}
	days, _ := recurrence["weekdays"].([]any)
	if len(days) != 2 || days[0] != "monday" || days[1] != "thursday" {
		t.Errorf("weekdays = %v, want [monday thursday]", days)
	}
	if template["dueTime"] != "10:30" {
		t.Errorf("dueTime = %v, want 10:30", template["dueTime"])
	}

	// Nothing in the payload resembles the stored rule.
	if bytes.Contains(rec.Body.Bytes(), []byte("FREQ=")) {
		t.Errorf("the stored recurrence rule leaked to the client: %s", rec.Body.String())
	}
}

func TestCreateValidatesTheSchedule(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")
	seniorID := h.createCircle(t, "owner", "Mrs Khan", "UTC")

	cases := []struct {
		name string
		body map[string]any
	}{
		{"no title", map[string]any{
			"scheduledFor": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		}},
		{"no schedule at all", map[string]any{"title": "Something"}},
		{"both at once", map[string]any{
			"title":        "Something",
			"scheduledFor": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			"recurrence":   map[string]any{"frequency": "daily"},
			"dueTime":      "09:00",
		}},
		{"recurring with no time", map[string]any{
			"title":      "Something",
			"recurrence": map[string]any{"frequency": "daily"},
		}},
		{"weekly with no days", map[string]any{
			"title":      "Something",
			"recurrence": map[string]any{"frequency": "weekly"},
			"dueTime":    "09:00",
		}},
		{"unsupported frequency", map[string]any{
			"title":      "Something",
			"recurrence": map[string]any{"frequency": "hourly"},
			"dueTime":    "09:00",
		}},
		{"impossible time", map[string]any{
			"title":      "Something",
			"recurrence": map[string]any{"frequency": "daily"},
			"dueTime":    "25:00",
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := h.do(t, "owner", http.MethodPost, "/v1/seniors/"+seniorID+"/tasks", tc.body)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("status = %d, want 422\nbody: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// The senior's timezone travels with the profile, so a family member abroad can
// render the schedule in the senior's own day.
func TestSeniorCarriesItsTimezone(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "Asia/Karachi")

	body := decode(t, h.do(t, "owner", http.MethodGet, "/v1/seniors/"+seniorID, nil), http.StatusOK)
	if body["timezone"] != "Asia/Karachi" {
		t.Errorf("timezone = %v, want Asia/Karachi", body["timezone"])
	}
}

func TestUnknownTimezoneIsRefused(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")

	rec := h.do(t, "owner", http.MethodPost, "/v1/seniors", map[string]any{
		"mode":        "family_member",
		"displayName": "Mrs Khan",
		"timezone":    "Mars/Olympus_Mons",
	})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422\nbody: %s", rec.Code, rec.Body.String())
	}
}

// The home screen lists the caller's own work across every circle.
func TestAssignedTasksAreListedForTheCaller(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")
	h.account("caregiver", "caregiver@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan", "UTC")
	caregiverID := h.join(t, "caregiver", seniorID, care.RoleProfessionalCaregiver,
		care.PermissionSeniorView, care.PermissionTasksView, care.PermissionTasksComplete)

	rec := h.do(t, "owner", http.MethodPost, "/v1/seniors/"+seniorID+"/tasks", map[string]any{
		"title":          "Morning visit",
		"scheduledFor":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		"assignedUserId": caregiverID.String(),
	})
	decode(t, rec, http.StatusCreated)

	// Assigned to the caregiver, so it appears on their list and not the
	// owner's.
	mine := decode(t, h.do(t, "caregiver", http.MethodGet, "/v1/tasks", nil), http.StatusOK)
	items, _ := mine["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("caregiver got %d assigned tasks, want 1", len(items))
	}

	theirs := decode(t, h.do(t, "owner", http.MethodGet, "/v1/tasks", nil), http.StatusOK)
	ownerItems, _ := theirs["items"].([]any)
	if len(ownerItems) != 0 {
		t.Errorf("owner got %d assigned tasks, want 0", len(ownerItems))
	}
}
