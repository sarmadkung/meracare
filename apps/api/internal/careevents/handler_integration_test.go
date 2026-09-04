package careevents_test

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

// These tests drive the real router, because the questions they ask — what does
// a stranger see, does a member without activity.view get the same answer as
// somebody who does not exist — are answered by the middleware stack and the
// handler together.

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

func (h *harness) createCircle(t *testing.T, token, name string) string {
	t.Helper()

	rec := h.do(t, token, http.MethodPost, "/v1/seniors", map[string]any{
		"mode":        "family_member",
		"displayName": name,
		"timezone":    "Asia/Karachi",
	})
	body := decodeBody(t, rec, http.StatusCreated)

	id, _ := body["id"].(string)
	if id == "" {
		t.Fatalf("no senior id in response: %s", rec.Body.String())
	}
	return id
}

func (h *harness) join(
	t *testing.T,
	token, seniorID string,
	role care.Role,
	permissions ...care.Permission,
) uuid.UUID {
	t.Helper()

	if rec := h.do(t, token, http.MethodGet, "/v1/seniors", nil); rec.Code != http.StatusOK {
		t.Fatalf("sign in: status = %d", rec.Code)
	}

	claims := h.verifier.accounts[token]
	var userID uuid.UUID
	if err := h.pool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE auth_user_id = $1`, claims.AuthUserID).Scan(&userID); err != nil {
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

// completeATask produces a real event through a real domain endpoint, which is
// the only way an event can come into existence.
func (h *harness) completeATask(t *testing.T, token, seniorID, title string) {
	t.Helper()

	rec := h.do(t, token, http.MethodPost, "/v1/seniors/"+seniorID+"/tasks", map[string]any{
		"title":        title,
		"scheduledFor": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	})
	body := decodeBody(t, rec, http.StatusCreated)

	items, _ := body["tasks"].([]any)
	if len(items) == 0 {
		t.Fatalf("no task in response: %s", rec.Body.String())
	}
	task, _ := items[0].(map[string]any)
	taskID, _ := task["id"].(string)

	if rec := h.do(t, token, http.MethodPost, "/v1/tasks/"+taskID+"/complete", nil); rec.Code != http.StatusOK {
		t.Fatalf("complete task: status = %d\nbody: %s", rec.Code, rec.Body.String())
	}
}

func eventTypes(t *testing.T, body map[string]any) []string {
	t.Helper()

	items, _ := body["items"].([]any)
	found := make([]string, 0, len(items))
	for _, item := range items {
		event, _ := item.(map[string]any)
		eventType, _ := event["type"].(string)
		found = append(found, eventType)
	}
	return found
}

// --- Authentication ----------------------------------------------------------

func TestActivityRequiresAuthentication(t *testing.T) {
	h := newHarness(t)

	rec := h.do(t, "", http.MethodGet, "/v1/seniors/"+uuid.New().String()+"/activity", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

// --- Authorization -----------------------------------------------------------

func TestAMemberWithActivityViewSeesTheTimeline(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan")
	h.completeATask(t, "owner", seniorID, "Morning walk")

	body := decodeBody(t,
		h.do(t, "owner", http.MethodGet, "/v1/seniors/"+seniorID+"/activity", nil),
		http.StatusOK)

	found := eventTypes(t, body)
	if len(found) < 2 {
		t.Fatalf("timeline = %v, want the task's creation and its completion", found)
	}
	// Newest first: the completion happened after the creation.
	if found[0] != "TASK_COMPLETED" {
		t.Errorf("first entry = %q, want TASK_COMPLETED (newest first)", found[0])
	}
}

// A stranger must not be able to tell a senior with activity from one that does
// not exist (plans/phase7.md §22).
func TestAStrangerSeesTheSameAnswerForRealAndImaginarySeniors(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")
	h.account("stranger", "stranger@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan")
	h.completeATask(t, "owner", seniorID, "Morning walk")

	if rec := h.do(t, "stranger", http.MethodGet, "/v1/seniors", nil); rec.Code != http.StatusOK {
		t.Fatalf("sign in: status = %d", rec.Code)
	}

	real := h.do(t, "stranger", http.MethodGet, "/v1/seniors/"+seniorID+"/activity", nil)
	imaginary := h.do(t, "stranger", http.MethodGet,
		"/v1/seniors/"+uuid.New().String()+"/activity", nil)

	if real.Code != http.StatusNotFound {
		t.Errorf("real senior: status = %d, want 404", real.Code)
	}
	if imaginary.Code != http.StatusNotFound {
		t.Errorf("imaginary senior: status = %d, want 404", imaginary.Code)
	}
	if real.Body.String() != imaginary.Body.String() {
		t.Errorf("the two answers differ, which reveals the senior exists:\n"+
			"    real: %s\nimaginary: %s", real.Body.String(), imaginary.Body.String())
	}
}

// activity.view is a permission a circle can withhold from an individual
// relationship, and withholding it must actually work.
func TestAMemberWithoutActivityViewCannotReadTheTimeline(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")
	h.account("caregiver", "caregiver@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan")
	h.completeATask(t, "owner", seniorID, "Morning walk")

	// Everything a caregiver normally holds, except activity.view.
	h.join(t, "caregiver", seniorID, care.RoleProfessionalCaregiver,
		care.PermissionSeniorView, care.PermissionTasksView, care.PermissionTasksComplete)

	rec := h.do(t, "caregiver", http.MethodGet, "/v1/seniors/"+seniorID+"/activity", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404\nbody: %s", rec.Code, rec.Body.String())
	}
}

func TestARevokedMemberLosesAccessToTheTimeline(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")
	h.account("former", "former@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan")
	h.completeATask(t, "owner", seniorID, "Morning walk")
	h.join(t, "former", seniorID, care.RoleFamilyMember, care.PermissionActivityView)

	if rec := h.do(t, "former", http.MethodGet, "/v1/seniors/"+seniorID+"/activity", nil); rec.Code != http.StatusOK {
		t.Fatalf("before revocation: status = %d", rec.Code)
	}

	claims := h.verifier.accounts["former"]
	var relationshipID uuid.UUID
	if err := h.pool.QueryRow(context.Background(), `
		SELECT cr.id FROM care_relationships cr
		JOIN users u ON u.id = cr.user_id
		WHERE u.auth_user_id = $1`, claims.AuthUserID).Scan(&relationshipID); err != nil {
		t.Fatalf("find relationship: %v", err)
	}
	if _, err := h.relationships.RevokeMembership(context.Background(), relationshipID); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	if rec := h.do(t, "former", http.MethodGet, "/v1/seniors/"+seniorID+"/activity", nil); rec.Code != http.StatusNotFound {
		t.Errorf("after revocation: status = %d, want 404", rec.Code)
	}
}

// --- Fabrication -------------------------------------------------------------

// There is no endpoint that creates an event, and there must not be: a timeline
// a client can write to records what people claimed rather than what happened
// (plans/phase7.md §21).
func TestThereIsNoEndpointForCreatingAnEvent(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan")

	fabricated := map[string]any{
		"type":       "TASK_COMPLETED",
		"entityType": "task",
		"entityId":   uuid.New().String(),
	}

	attempts := map[string]*httptest.ResponseRecorder{
		"POST /v1/care-events": h.do(t, "owner", http.MethodPost, "/v1/care-events", fabricated),
		"POST senior activity": h.do(t, "owner", http.MethodPost,
			"/v1/seniors/"+seniorID+"/activity", fabricated),
		"PUT senior activity": h.do(t, "owner", http.MethodPut,
			"/v1/seniors/"+seniorID+"/activity", fabricated),
		"DELETE senior activity": h.do(t, "owner", http.MethodDelete,
			"/v1/seniors/"+seniorID+"/activity", nil),
	}

	for name, rec := range attempts {
		// 404 for a route that does not exist, 405 for a method the route does
		// not accept. Either way nothing was written.
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: status = %d, want 404 or 405\nbody: %s", name, rec.Code, rec.Body.String())
		}
	}

	body := decodeBody(t,
		h.do(t, "owner", http.MethodGet, "/v1/seniors/"+seniorID+"/activity", nil),
		http.StatusOK)
	if found := eventTypes(t, body); len(found) != 0 {
		t.Errorf("timeline = %v, want it empty — nothing real has happened", found)
	}
}

// --- Pagination over HTTP ----------------------------------------------------

func TestTheTimelinePagesOverHTTP(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan")
	for i := 0; i < 3; i++ {
		h.completeATask(t, "owner", seniorID, "Task "+string(rune('A'+i)))
	}

	seen := map[string]int{}
	cursor := ""
	for page := 0; ; page++ {
		if page > 10 {
			t.Fatal("the timeline never reached its end")
		}

		path := "/v1/seniors/" + seniorID + "/activity?limit=2"
		if cursor != "" {
			path += "&cursor=" + cursor
		}

		body := decodeBody(t, h.do(t, "owner", http.MethodGet, path, nil), http.StatusOK)

		items, _ := body["items"].([]any)
		if len(items) > 2 {
			t.Fatalf("page %d returned %d events, want at most the requested 2", page, len(items))
		}
		for _, item := range items {
			event, _ := item.(map[string]any)
			id, _ := event["id"].(string)
			seen[id]++
		}

		next, ok := body["nextCursor"].(string)
		if !ok || next == "" {
			break
		}
		cursor = next
	}

	// Three tasks, each created and completed.
	if len(seen) != 6 {
		t.Errorf("saw %d distinct events, want 6", len(seen))
	}
	for id, count := range seen {
		if count != 1 {
			t.Errorf("event %s appeared %d times, want exactly once", id, count)
		}
	}
}

func TestAnUnreadableCursorIsRefusedOverHTTP(t *testing.T) {
	h := newHarness(t)
	h.account("owner", "owner@example.com")

	seniorID := h.createCircle(t, "owner", "Mrs Khan")

	rec := h.do(t, "owner", http.MethodGet,
		"/v1/seniors/"+seniorID+"/activity?cursor=not-a-cursor", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400\nbody: %s", rec.Code, rec.Body.String())
	}
}
