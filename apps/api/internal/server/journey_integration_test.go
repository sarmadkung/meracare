package server_test

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
	"github.com/genxcare/api/internal/config"
	"github.com/genxcare/api/internal/database"
	"github.com/genxcare/api/internal/server"
	"github.com/genxcare/api/internal/testsupport"
	"github.com/genxcare/api/pkg/logging"
)

// The MVP end-to-end journey (plans/phase9.md §40).
//
// Every phase has tested its own domain thoroughly. What none of them could
// test is the thing the product actually is: a family and a paid caregiver
// looking after one person together, over the same data, with different
// permissions, through the real HTTP surface. This does that once, in order,
// with no shortcuts through repositories — every step is a request a client
// could make.
//
// It is deliberately one long test rather than several. The value is in the
// sequence: the caregiver's access has to be granted before it can be used and
// used before it can be revoked, and a test that set up each step directly
// would prove less than one that walks the path a real circle walks.

// journeyVerifier maps bearer tokens to identities, standing in for Supabase.
type journeyVerifier struct {
	accounts map[string]auth.Claims
}

func (v journeyVerifier) Verify(_ context.Context, rawToken string) (*auth.Claims, error) {
	claims, ok := v.accounts[rawToken]
	if !ok {
		return nil, auth.ErrInvalidToken
	}
	return &claims, nil
}

type journey struct {
	t        *testing.T
	handler  http.Handler
	verifier journeyVerifier
	pool     *database.Pool
}

func newJourney(t *testing.T) *journey {
	t.Helper()

	pool := testsupport.RequireDatabase(t)
	verifier := journeyVerifier{accounts: map[string]auth.Claims{}}

	return &journey{
		t: t,
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
		verifier: verifier,
		pool:     pool,
	}
}

// signUp registers a bearer token for a new identity, as Supabase would.
func (j *journey) signUp(token, email string) {
	j.verifier.accounts[token] = auth.Claims{
		AuthUserID: uuid.New(),
		Email:      email,
		Provider:   "email",
		IssuedAt:   time.Now().Add(-time.Minute),
		ExpiresAt:  time.Now().Add(time.Hour),
	}
}

// request performs one call and asserts the status, returning the decoded body.
func (j *journey) request(
	step, token, method, path string,
	body any,
	want int,
) map[string]any {
	j.t.Helper()

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			j.t.Fatalf("%s: encode request: %v", step, err)
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
	j.handler.ServeHTTP(recorder, request)

	if recorder.Code != want {
		j.t.Fatalf("%s: status = %d, want %d\nbody: %s", step, recorder.Code, want, recorder.Body.String())
	}

	var decoded map[string]any
	if recorder.Body.Len() > 0 {
		if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
			j.t.Fatalf("%s: body is not JSON: %v", step, err)
		}
	}
	return decoded
}

// id pulls a string field out of a response.
func id(t *testing.T, step string, body map[string]any, field string) string {
	t.Helper()

	value, _ := body[field].(string)
	if value == "" {
		t.Fatalf("%s: no %q in response", step, field)
	}
	return value
}

func items(body map[string]any, field string) []map[string]any {
	raw, _ := body[field].([]any)
	list := make([]map[string]any, 0, len(raw))
	for _, entry := range raw {
		item, _ := entry.(map[string]any)
		list = append(list, item)
	}
	return list
}

// week is an explicit from/to range covering the next seven days.
//
// The journey reads scheduled care through a window rather than through
// "today", so that it asserts the same thing whatever time of day the suite
// runs at: a daily 09:00 task created at half past ten at night correctly has
// no occurrence today, and a test that depended on the hour would be a test
// that failed overnight.
func week(now time.Time) string {
	return "&from=" + now.Add(-time.Minute).UTC().Format(time.RFC3339) +
		"&to=" + now.AddDate(0, 0, 7).UTC().Format(time.RFC3339)
}

func TestTheWholeMVPJourney(t *testing.T) {
	j := newJourney(t)

	j.signUp("sara", "sara@example.com")       // the daughter, who sets things up
	j.signUp("bilal", "bilal@example.com")     // the son, family member
	j.signUp("nadia", "nadia@example.com")     // the professional caregiver
	j.signUp("stranger", "nobody@example.com") // nobody at all

	// --- The daughter creates the circle -------------------------------------

	// The application user is created on the first authenticated request, from
	// the verified token — never from anything the client sent.
	me := j.request("sign in", "sara", http.MethodGet, "/v1/me", nil, http.StatusOK)
	id(t, "sign in", me, "id")

	senior := j.request("create senior", "sara", http.MethodPost, "/v1/seniors", map[string]any{
		"mode":        "family_member",
		"displayName": "Amma",
		"timezone":    "Asia/Karachi",
		"phone":       "+92 300 1234567",
	}, http.StatusCreated)
	seniorID := id(t, "create senior", senior, "id")

	// --- Care is set up ------------------------------------------------------

	task := j.request("create task", "sara", http.MethodPost,
		"/v1/seniors/"+seniorID+"/tasks", map[string]any{
			"title":      "Morning walk",
			"recurrence": map[string]any{"frequency": "daily"},
			"dueTime":    "09:00",
		}, http.StatusCreated)
	taskInstances := items(task, "tasks")
	if len(taskInstances) == 0 {
		t.Fatal("creating a recurring task produced no occurrences")
	}

	medication := j.request("create medication", "sara", http.MethodPost,
		"/v1/seniors/"+seniorID+"/medications", map[string]any{
			"name":   "Metformin",
			"dosage": "500 mg",
			"form":   "tablet",
			"schedules": []map[string]any{
				{"scheduledTime": "08:00", "recurrence": map[string]any{"frequency": "daily"}},
			},
		}, http.StatusCreated)
	medicationDetail, _ := medication["medication"].(map[string]any)
	medicationID := id(t, "create medication", medicationDetail, "id")

	appointment := j.request("create appointment", "sara", http.MethodPost,
		"/v1/seniors/"+seniorID+"/appointments", map[string]any{
			"title":        "Cardiology review",
			"scheduledAt":  time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339),
			"providerName": "Dr Ahmed",
			"location":     "City Hospital",
		}, http.StatusCreated)
	appointmentID := id(t, "create appointment", appointment, "id")

	// --- Nobody outside the circle can see any of it -------------------------

	j.request("stranger reads senior", "stranger", http.MethodGet,
		"/v1/seniors/"+seniorID, nil, http.StatusNotFound)
	j.request("stranger reads tasks", "stranger", http.MethodGet,
		"/v1/seniors/"+seniorID+"/tasks?scope=today", nil, http.StatusNotFound)
	j.request("stranger reads medication", "stranger", http.MethodGet,
		"/v1/seniors/"+seniorID+"/medications", nil, http.StatusNotFound)
	j.request("stranger reads activity", "stranger", http.MethodGet,
		"/v1/seniors/"+seniorID+"/activity", nil, http.StatusNotFound)

	strangerPlan := j.request("stranger's reminders", "stranger", http.MethodGet,
		"/v1/notifications/reminders", nil, http.StatusOK)
	if got := len(items(strangerPlan, "reminders")); got != 0 {
		t.Errorf("a stranger has %d reminders, want 0", got)
	}

	// --- The family is invited ----------------------------------------------

	j.request("sign in", "bilal", http.MethodGet, "/v1/me", nil, http.StatusOK)

	familyInvite := j.request("invite the son", "sara", http.MethodPost,
		"/v1/seniors/"+seniorID+"/invitations", map[string]any{
			"email": "bilal@example.com",
			"role":  "family_member",
		}, http.StatusCreated)
	familyToken := id(t, "invite the son", familyInvite, "token")

	j.request("son accepts", "bilal", http.MethodPost,
		"/v1/invitations/"+familyToken+"/accept", nil, http.StatusOK)

	// --- The caregiver is invited, with a deliberately narrow permission set --

	j.request("sign in", "nadia", http.MethodGet, "/v1/me", nil, http.StatusOK)

	// She may see and record the day's care, and nothing else: no editing the
	// profile, no changing the prescription, no managing the circle
	// (plans/phase9.md §4).
	caregiverInvite := j.request("invite the caregiver", "sara", http.MethodPost,
		"/v1/seniors/"+seniorID+"/invitations", map[string]any{
			"email": "nadia@example.com",
			"role":  "professional_caregiver",
			"permissions": []string{
				"senior.view",
				"tasks.view", "tasks.complete",
				"medications.view", "medications.record",
				"appointments.view",
				"activity.view",
			},
		}, http.StatusCreated)
	caregiverToken := id(t, "invite the caregiver", caregiverInvite, "token")

	preview := j.request("caregiver previews", "", http.MethodGet,
		"/v1/invitations/"+caregiverToken, nil, http.StatusOK)
	if preview["seniorName"] != "Amma" {
		t.Errorf("invitation preview names %v", preview["seniorName"])
	}
	// The preview is unauthenticated, so it must carry nothing but what somebody
	// needs in order to decide whether to accept.
	for _, leaked := range []string{"+92 300 1234567", "Metformin", "Cardiology"} {
		encoded, _ := json.Marshal(preview)
		if strings.Contains(string(encoded), leaked) {
			t.Errorf("the unauthenticated invitation preview leaks %q", leaked)
		}
	}

	j.request("caregiver accepts", "nadia", http.MethodPost,
		"/v1/invitations/"+caregiverToken+"/accept", nil, http.StatusOK)

	// --- The caregiver does her round ---------------------------------------

	visible := j.request("caregiver's seniors", "nadia", http.MethodGet,
		"/v1/seniors", nil, http.StatusOK)
	if got := len(items(visible, "items")); got != 1 {
		t.Fatalf("the caregiver sees %d seniors, want 1", got)
	}

	upcoming := j.request("caregiver reads the week's tasks", "nadia", http.MethodGet,
		"/v1/seniors/"+seniorID+"/tasks?scope=window"+week(time.Now()), nil, http.StatusOK)
	scheduledTasks := items(upcoming, "items")
	if len(scheduledTasks) == 0 {
		t.Fatal("the caregiver sees no scheduled tasks")
	}
	taskID := id(t, "the week's tasks", scheduledTasks[0], "id")

	j.request("caregiver completes a task", "nadia", http.MethodPost,
		"/v1/tasks/"+taskID+"/complete", nil, http.StatusOK)

	doses := j.request("caregiver reads the week's doses", "nadia", http.MethodGet,
		"/v1/seniors/"+seniorID+"/medications/doses?scope=window"+week(time.Now()), nil,
		http.StatusOK)
	scheduledDoses := items(doses, "items")
	if len(scheduledDoses) == 0 {
		t.Fatal("the caregiver sees no scheduled doses")
	}
	doseID := id(t, "the week's doses", scheduledDoses[0], "id")

	j.request("caregiver records a dose", "nadia", http.MethodPost,
		"/v1/medications/"+medicationID+"/instances/"+doseID+"/take", nil, http.StatusOK)

	j.request("caregiver reads the appointment", "nadia", http.MethodGet,
		"/v1/appointments/"+appointmentID, nil, http.StatusOK)

	// --- What she was not granted, she cannot do ----------------------------

	j.request("caregiver edits the profile", "nadia", http.MethodPatch,
		"/v1/seniors/"+seniorID, map[string]any{"phone": "changed"}, http.StatusNotFound)
	j.request("caregiver changes the prescription", "nadia", http.MethodPatch,
		"/v1/medications/"+medicationID, map[string]any{"dosage": "1000 mg"}, http.StatusNotFound)
	j.request("caregiver cancels the appointment", "nadia", http.MethodPost,
		"/v1/appointments/"+appointmentID+"/cancel", nil, http.StatusNotFound)
	j.request("caregiver invites somebody", "nadia", http.MethodPost,
		"/v1/seniors/"+seniorID+"/invitations",
		map[string]any{"email": "someone@example.com", "role": "family_member"},
		http.StatusNotFound)
	j.request("caregiver reads the circle", "nadia", http.MethodGet,
		"/v1/seniors/"+seniorID+"/members", nil, http.StatusNotFound)

	// --- Her work shows up in the timeline, for the family ------------------

	timeline := j.request("family reads activity", "bilal", http.MethodGet,
		"/v1/seniors/"+seniorID+"/activity", nil, http.StatusOK)
	events := items(timeline, "items")

	recorded := map[string]bool{}
	for _, event := range events {
		eventType, _ := event["type"].(string)
		recorded[eventType] = true
	}
	for _, wanted := range []string{
		"TASK_CREATED", "TASK_COMPLETED",
		"MEDICATION_CREATED", "MEDICATION_TAKEN",
		"APPOINTMENT_CREATED",
		"MEMBER_INVITED", "MEMBER_JOINED",
	} {
		if !recorded[wanted] {
			t.Errorf("the timeline is missing %s", wanted)
		}
	}

	// --- And she is reminded about the care she is responsible for ----------

	caregiverPlan := j.request("caregiver's reminders", "nadia", http.MethodGet,
		"/v1/notifications/reminders", nil, http.StatusOK)
	if got := len(items(caregiverPlan, "reminders")); got == 0 {
		t.Error("the caregiver has no reminders for a circle full of scheduled care")
	}

	// --- Then her engagement ends -------------------------------------------

	circle := j.request("family reads the circle", "sara", http.MethodGet,
		"/v1/seniors/"+seniorID+"/members", nil, http.StatusOK)

	var caregiverRelationshipID string
	for _, member := range items(circle, "items") {
		if member["role"] == "professional_caregiver" {
			caregiverRelationshipID, _ = member["id"].(string)
		}
	}
	if caregiverRelationshipID == "" {
		t.Fatal("the caregiver is not in the circle")
	}

	// 200 with the revoked membership, not 204: the relationship is preserved
	// rather than deleted, so the record of who was involved survives
	// (plans/phase9.md §15).
	revoked := j.request("revoke the caregiver", "sara", http.MethodDelete,
		"/v1/seniors/"+seniorID+"/members/"+caregiverRelationshipID, nil, http.StatusOK)
	if revoked["status"] != "revoked" {
		t.Errorf("membership status = %v, want revoked", revoked["status"])
	}

	// Access is gone, everywhere, immediately.
	j.request("revoked caregiver reads senior", "nadia", http.MethodGet,
		"/v1/seniors/"+seniorID, nil, http.StatusNotFound)
	j.request("revoked caregiver reads tasks", "nadia", http.MethodGet,
		"/v1/seniors/"+seniorID+"/tasks?scope=today", nil, http.StatusNotFound)
	j.request("revoked caregiver reads activity", "nadia", http.MethodGet,
		"/v1/seniors/"+seniorID+"/activity", nil, http.StatusNotFound)
	j.request("revoked caregiver acts on a task", "nadia", http.MethodPost,
		"/v1/tasks/"+taskID+"/skip", nil, http.StatusNotFound)

	after := j.request("revoked caregiver's seniors", "nadia", http.MethodGet,
		"/v1/seniors", nil, http.StatusOK)
	if got := len(items(after, "items")); got != 0 {
		t.Errorf("the revoked caregiver still sees %d seniors, want 0", got)
	}

	// Future reminders stop.
	revokedPlan := j.request("revoked caregiver's reminders", "nadia", http.MethodGet,
		"/v1/notifications/reminders", nil, http.StatusOK)
	if got := len(items(revokedPlan, "reminders")); got != 0 {
		t.Errorf("the revoked caregiver has %d reminders, want 0", got)
	}

	// But what she did remains. Care that was given is a fact about the past,
	// and revoking somebody's access must not erase the record of their work
	// (plans/phase9.md §40, docs/04-care-events-and-workflows.md).
	stillThere := j.request("family reads activity again", "sara", http.MethodGet,
		"/v1/seniors/"+seniorID+"/activity", nil, http.StatusOK)

	var completions int
	for _, event := range items(stillThere, "items") {
		if event["type"] == "TASK_COMPLETED" || event["type"] == "MEDICATION_TAKEN" {
			completions++
		}
	}
	if completions < 2 {
		t.Errorf("the caregiver's recorded care did not survive revocation: %d entries", completions)
	}

	// And the dose itself still says it was taken. Read through the schedule
	// rather than through history, because the dose the caregiver recorded may
	// still be in the future — history is what has already passed.
	afterDoses := j.request("doses after revocation", "sara", http.MethodGet,
		"/v1/seniors/"+seniorID+"/medications/doses?scope=window"+week(time.Now()), nil,
		http.StatusOK)

	var taken bool
	for _, dose := range items(afterDoses, "items") {
		if dose["id"] == doseID && dose["status"] == "taken" {
			taken = true
		}
	}
	if !taken {
		t.Error("the dose the caregiver recorded is no longer marked as taken")
	}

	// --- The family continues without her -----------------------------------

	j.request("family completes the appointment", "sara", http.MethodPost,
		"/v1/appointments/"+appointmentID+"/complete", nil, http.StatusOK)
}

func TestSoloSelfCareNeedsNobodyElse(t *testing.T) {
	// The whole solo journey, which must work without a single caregiver:
	// somebody managing their own care is the product's first mode, not a
	// degraded version of the family one (plans/phase9.md §2).
	j := newJourney(t)
	j.signUp("aisha", "aisha@example.com")

	senior := j.request("create own profile", "aisha", http.MethodPost, "/v1/seniors",
		map[string]any{
			"mode":        "self",
			"displayName": "Aisha",
			"timezone":    "Asia/Karachi",
		}, http.StatusCreated)
	seniorID := id(t, "create own profile", senior, "id")

	if senior["isSelf"] != true {
		t.Errorf("isSelf = %v, want true for a self-created profile", senior["isSelf"])
	}

	j.request("add a task", "aisha", http.MethodPost, "/v1/seniors/"+seniorID+"/tasks",
		map[string]any{
			"title":      "Evening walk",
			"recurrence": map[string]any{"frequency": "daily"},
			"dueTime":    "18:00",
		}, http.StatusCreated)

	medication := j.request("add a medication", "aisha", http.MethodPost,
		"/v1/seniors/"+seniorID+"/medications", map[string]any{
			"name":   "Vitamin D",
			"dosage": "1000 IU",
			"schedules": []map[string]any{
				{"scheduledTime": "09:00", "recurrence": map[string]any{"frequency": "daily"}},
			},
		}, http.StatusCreated)
	detail, _ := medication["medication"].(map[string]any)
	medicationID := id(t, "add a medication", detail, "id")

	j.request("book an appointment", "aisha", http.MethodPost,
		"/v1/seniors/"+seniorID+"/appointments", map[string]any{
			"title":       "Dentist",
			"scheduledAt": time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339),
		}, http.StatusCreated)

	// A senior manages their own care in full: the role carries every
	// permission, so nothing above needed a second person to authorize it.
	doses := j.request("own doses", "aisha", http.MethodGet,
		"/v1/seniors/"+seniorID+"/medications/doses?scope=window"+week(time.Now()), nil,
		http.StatusOK)
	scheduledDoses := items(doses, "items")
	if len(scheduledDoses) == 0 {
		t.Fatal("no scheduled doses")
	}

	j.request("take a dose", "aisha", http.MethodPost,
		"/v1/medications/"+medicationID+"/instances/"+
			id(t, "own doses", scheduledDoses[0], "id")+"/take", nil, http.StatusOK)

	timeline := j.request("own activity", "aisha", http.MethodGet,
		"/v1/seniors/"+seniorID+"/activity", nil, http.StatusOK)
	if len(items(timeline, "items")) == 0 {
		t.Error("a solo user's own care left no activity")
	}

	plan := j.request("own reminders", "aisha", http.MethodGet,
		"/v1/notifications/reminders", nil, http.StatusOK)
	if len(items(plan, "reminders")) == 0 {
		t.Error("a solo user with daily medication and tasks has no reminders")
	}
}
