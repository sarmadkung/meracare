package notifications_test

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

// These tests drive the real router over HTTP against a real database. The
// questions they ask — does a revoked caregiver stop being reminded, can one
// user reach another's devices, does a push token ever leave the server — are
// answered by the middleware, the handlers, and the schema together.

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

// signIn makes one authenticated request so the application user exists, and
// returns its id.
func (h *harness) signIn(t *testing.T, token string) uuid.UUID {
	t.Helper()

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
	return userID
}

func (h *harness) join(
	t *testing.T,
	token, seniorID string,
	role care.Role,
	permissions ...care.Permission,
) (uuid.UUID, uuid.UUID) {
	t.Helper()

	userID := h.signIn(t, token)

	senior, err := uuid.Parse(seniorID)
	if err != nil {
		t.Fatalf("parse senior id: %v", err)
	}

	relationship, err := h.relationships.Create(context.Background(), relationships.CreateParams{
		SeniorID:    senior,
		UserID:      userID,
		Role:        role,
		Permissions: care.Normalise(permissions),
		Status:      care.StatusActive,
	})
	if err != nil {
		t.Fatalf("join circle: %v", err)
	}
	return userID, relationship.ID
}

// reminders fetches the caller's plan and returns the reminder list.
func (h *harness) reminders(t *testing.T, token string) []map[string]any {
	t.Helper()

	rec := h.do(t, token, http.MethodGet, "/v1/notifications/reminders", nil)
	body := decodeBody(t, rec, http.StatusOK)

	raw, _ := body["reminders"].([]any)
	reminders := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		reminder, _ := item.(map[string]any)
		reminders = append(reminders, reminder)
	}
	return reminders
}

// remindersOfType filters a plan down to one category.
func remindersOfType(plan []map[string]any, wanted string) []map[string]any {
	matching := make([]map[string]any, 0, len(plan))
	for _, reminder := range plan {
		if reminder["type"] == wanted {
			matching = append(matching, reminder)
		}
	}
	return matching
}

// --- Authentication ---------------------------------------------------------

func TestNotificationRoutesRequireAuthentication(t *testing.T) {
	h := newHarness(t)

	cases := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"preferences", http.MethodGet, "/v1/notifications/preferences", nil},
		{"update preferences", http.MethodPatch, "/v1/notifications/preferences",
			map[string]any{"taskReminders": false}},
		{"register device", http.MethodPost, "/v1/notifications/devices",
			map[string]any{"deviceId": "d1", "platform": "ios"}},
		{"deactivate device", http.MethodDelete, "/v1/notifications/devices/d1", nil},
		{"reminders", http.MethodGet, "/v1/notifications/reminders", nil},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			rec := h.do(t, "", testCase.method, testCase.path, testCase.body)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", rec.Code)
			}
		})
	}
}

// --- Preferences ------------------------------------------------------------

func TestPreferencesDefaultToEveryCategoryOn(t *testing.T) {
	h := newHarness(t)
	h.account("sara", "sara@example.com")
	h.signIn(t, "sara")

	rec := h.do(t, "sara", http.MethodGet, "/v1/notifications/preferences", nil)
	body := decodeBody(t, rec, http.StatusOK)

	for _, field := range []string{"taskReminders", "medicationReminders", "appointmentReminders"} {
		if body[field] != true {
			t.Errorf("%s = %v, want true", field, body[field])
		}
	}
}

func TestUpdatingOneCategoryLeavesTheOthersAlone(t *testing.T) {
	// The first update is also the row's creation, which is where a naive
	// upsert would quietly switch the untouched categories off.
	h := newHarness(t)
	h.account("sara", "sara@example.com")
	h.signIn(t, "sara")

	rec := h.do(t, "sara", http.MethodPatch, "/v1/notifications/preferences",
		map[string]any{"medicationReminders": false})
	body := decodeBody(t, rec, http.StatusOK)

	if body["medicationReminders"] != false {
		t.Errorf("medicationReminders = %v, want false", body["medicationReminders"])
	}
	if body["taskReminders"] != true {
		t.Errorf("taskReminders = %v, want it left on", body["taskReminders"])
	}
	if body["appointmentReminders"] != true {
		t.Errorf("appointmentReminders = %v, want it left on", body["appointmentReminders"])
	}

	// And it survives the round trip.
	again := decodeBody(t, h.do(t, "sara", http.MethodGet, "/v1/notifications/preferences", nil),
		http.StatusOK)
	if again["medicationReminders"] != false {
		t.Errorf("after reload medicationReminders = %v, want false", again["medicationReminders"])
	}
}

func TestOneUsersPreferencesDoNotAffectAnother(t *testing.T) {
	// Two caregivers in the same circle, one of whom wants silence
	// (plans/phase8.md §4).
	h := newHarness(t)
	h.account("sara", "sara@example.com")
	h.account("bilal", "bilal@example.com")
	h.signIn(t, "sara")
	h.signIn(t, "bilal")

	decodeBody(t, h.do(t, "sara", http.MethodPatch, "/v1/notifications/preferences",
		map[string]any{"taskReminders": false}), http.StatusOK)

	body := decodeBody(t, h.do(t, "bilal", http.MethodGet, "/v1/notifications/preferences", nil),
		http.StatusOK)
	if body["taskReminders"] != true {
		t.Errorf("bilal's taskReminders = %v, want true", body["taskReminders"])
	}
}

func TestPreferencesCannotBeSetForSomebodyElse(t *testing.T) {
	// The body has no user field. Sending one must be refused outright rather
	// than ignored, so a client cannot believe it worked (plans/phase8.md §40).
	h := newHarness(t)
	h.account("sara", "sara@example.com")
	h.account("bilal", "bilal@example.com")
	h.signIn(t, "sara")
	bilal := h.signIn(t, "bilal")

	rec := h.do(t, "sara", http.MethodPatch, "/v1/notifications/preferences", map[string]any{
		"userId":        bilal.String(),
		"taskReminders": false,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for an unknown field\nbody: %s", rec.Code, rec.Body.String())
	}

	body := decodeBody(t, h.do(t, "bilal", http.MethodGet, "/v1/notifications/preferences", nil),
		http.StatusOK)
	if body["taskReminders"] != true {
		t.Errorf("bilal's taskReminders = %v, want it untouched", body["taskReminders"])
	}
}

// --- Devices ----------------------------------------------------------------

func TestRegisteringADeviceTwiceKeepsOneRegistration(t *testing.T) {
	// The app registers on every launch, because push tokens rotate. Two rows
	// for one install would mean two notifications on one phone
	// (plans/phase8.md §§25, 27).
	h := newHarness(t)
	h.account("sara", "sara@example.com")
	userID := h.signIn(t, "sara")

	register := func(pushToken string) map[string]any {
		return decodeBody(t, h.do(t, "sara", http.MethodPost, "/v1/notifications/devices",
			map[string]any{
				"deviceId":   "install-1",
				"platform":   "ios",
				"pushToken":  pushToken,
				"appVersion": "0.1.0",
			}), http.StatusOK)
	}

	first := register("ExponentPushToken[aaa]")
	second := register("ExponentPushToken[bbb]")

	if first["id"] != second["id"] {
		t.Errorf("re-registering created a new device: %v then %v", first["id"], second["id"])
	}

	var rows int
	if err := h.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM notification_devices WHERE user_id = $1`, userID).Scan(&rows); err != nil {
		t.Fatalf("count devices: %v", err)
	}
	if rows != 1 {
		t.Errorf("device rows = %d, want 1", rows)
	}

	// The newer token replaced the older one.
	var stored string
	if err := h.pool.QueryRow(context.Background(),
		`SELECT push_token FROM notification_devices WHERE user_id = $1`, userID).Scan(&stored); err != nil {
		t.Fatalf("read token: %v", err)
	}
	if stored != "ExponentPushToken[bbb]" {
		t.Errorf("stored token = %q, want the refreshed one", stored)
	}
}

func TestOneUserMayRegisterSeveralDevices(t *testing.T) {
	h := newHarness(t)
	h.account("sara", "sara@example.com")
	userID := h.signIn(t, "sara")

	for _, install := range []struct{ id, platform string }{
		{"phone", "ios"}, {"tablet", "ios"}, {"android", "android"},
	} {
		decodeBody(t, h.do(t, "sara", http.MethodPost, "/v1/notifications/devices",
			map[string]any{"deviceId": install.id, "platform": install.platform}), http.StatusOK)
	}

	var rows int
	if err := h.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM notification_devices WHERE user_id = $1 AND active`,
		userID).Scan(&rows); err != nil {
		t.Fatalf("count devices: %v", err)
	}
	if rows != 3 {
		t.Errorf("active devices = %d, want 3", rows)
	}
}

func TestARegistrationWithoutATokenDoesNotEraseAKnownOne(t *testing.T) {
	// The app registers as soon as it signs in, before the permission prompt.
	// That launch must not silently unsubscribe a phone that already works.
	h := newHarness(t)
	h.account("sara", "sara@example.com")
	userID := h.signIn(t, "sara")

	decodeBody(t, h.do(t, "sara", http.MethodPost, "/v1/notifications/devices", map[string]any{
		"deviceId": "install-1", "platform": "ios", "pushToken": "ExponentPushToken[aaa]",
	}), http.StatusOK)

	body := decodeBody(t, h.do(t, "sara", http.MethodPost, "/v1/notifications/devices",
		map[string]any{"deviceId": "install-1", "platform": "ios"}), http.StatusOK)

	if body["pushTokenRegistered"] != true {
		t.Errorf("pushTokenRegistered = %v, want true", body["pushTokenRegistered"])
	}

	var stored string
	if err := h.pool.QueryRow(context.Background(),
		`SELECT push_token FROM notification_devices WHERE user_id = $1`, userID).Scan(&stored); err != nil {
		t.Fatalf("read token: %v", err)
	}
	if stored != "ExponentPushToken[aaa]" {
		t.Errorf("stored token = %q, want it kept", stored)
	}
}

func TestAPushTokenIsNeverReturnedByTheAPI(t *testing.T) {
	// A token is a credential for making somebody's phone buzz
	// (plans/phase8.md §8).
	h := newHarness(t)
	h.account("sara", "sara@example.com")
	h.signIn(t, "sara")

	const token = "ExponentPushToken[secret-value]"
	rec := h.do(t, "sara", http.MethodPost, "/v1/notifications/devices", map[string]any{
		"deviceId": "install-1", "platform": "ios", "pushToken": token,
	})
	decodeBody(t, rec, http.StatusOK)

	if strings.Contains(rec.Body.String(), token) {
		t.Errorf("the registration response echoed the push token: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "pushToken\"") {
		t.Errorf("the response carries a pushToken field: %s", rec.Body.String())
	}
}

func TestDeactivatingADeviceKeepsTheRowAndDropsTheToken(t *testing.T) {
	h := newHarness(t)
	h.account("sara", "sara@example.com")
	userID := h.signIn(t, "sara")

	decodeBody(t, h.do(t, "sara", http.MethodPost, "/v1/notifications/devices", map[string]any{
		"deviceId": "install-1", "platform": "ios", "pushToken": "ExponentPushToken[aaa]",
	}), http.StatusOK)

	rec := h.do(t, "sara", http.MethodDelete, "/v1/notifications/devices/install-1", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204\nbody: %s", rec.Code, rec.Body.String())
	}

	var (
		active bool
		stored *string
	)
	err := h.pool.QueryRow(context.Background(),
		`SELECT active, push_token FROM notification_devices WHERE user_id = $1`,
		userID).Scan(&active, &stored)
	if err != nil {
		t.Fatalf("read device: %v", err)
	}
	if active {
		t.Error("device is still active")
	}
	if stored != nil {
		t.Errorf("push token survived deactivation: %q", *stored)
	}
}

func TestSigningBackInReactivatesTheSameDevice(t *testing.T) {
	h := newHarness(t)
	h.account("sara", "sara@example.com")
	userID := h.signIn(t, "sara")

	first := decodeBody(t, h.do(t, "sara", http.MethodPost, "/v1/notifications/devices",
		map[string]any{"deviceId": "install-1", "platform": "ios"}), http.StatusOK)

	h.do(t, "sara", http.MethodDelete, "/v1/notifications/devices/install-1", nil)

	again := decodeBody(t, h.do(t, "sara", http.MethodPost, "/v1/notifications/devices",
		map[string]any{"deviceId": "install-1", "platform": "ios"}), http.StatusOK)

	if first["id"] != again["id"] {
		t.Errorf("reactivation created a new device: %v then %v", first["id"], again["id"])
	}
	if again["active"] != true {
		t.Errorf("active = %v, want true", again["active"])
	}

	var rows int
	if err := h.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM notification_devices WHERE user_id = $1`, userID).Scan(&rows); err != nil {
		t.Fatalf("count devices: %v", err)
	}
	if rows != 1 {
		t.Errorf("device rows = %d, want 1", rows)
	}
}

func TestDeactivatingTwiceIsSafeThenUnknown(t *testing.T) {
	h := newHarness(t)
	h.account("sara", "sara@example.com")
	h.signIn(t, "sara")

	decodeBody(t, h.do(t, "sara", http.MethodPost, "/v1/notifications/devices",
		map[string]any{"deviceId": "install-1", "platform": "ios"}), http.StatusOK)

	// The row still matches, so the repeat succeeds rather than erroring: an
	// app retrying a sign-out must not see a failure.
	for range 2 {
		rec := h.do(t, "sara", http.MethodDelete, "/v1/notifications/devices/install-1", nil)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", rec.Code)
		}
	}
}

func TestAUserCannotTouchAnotherUsersDevice(t *testing.T) {
	h := newHarness(t)
	h.account("sara", "sara@example.com")
	h.account("intruder", "intruder@example.com")
	saraID := h.signIn(t, "sara")
	h.signIn(t, "intruder")

	decodeBody(t, h.do(t, "sara", http.MethodPost, "/v1/notifications/devices",
		map[string]any{"deviceId": "saras-phone", "platform": "ios"}), http.StatusOK)

	// 404, not 403: a different status would confirm the device exists
	// (plans/phase8.md §40).
	rec := h.do(t, "intruder", http.MethodDelete, "/v1/notifications/devices/saras-phone", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404\nbody: %s", rec.Code, rec.Body.String())
	}

	var active bool
	if err := h.pool.QueryRow(context.Background(),
		`SELECT active FROM notification_devices WHERE user_id = $1`, saraID).Scan(&active); err != nil {
		t.Fatalf("read device: %v", err)
	}
	if !active {
		t.Error("the intruder deactivated Sara's device")
	}
}

func TestTwoUsersMayShareADeviceIdentifier(t *testing.T) {
	// A shared family tablet: two accounts, one installation. Each keeps its
	// own registration, and neither can reach the other's.
	h := newHarness(t)
	h.account("sara", "sara@example.com")
	h.account("bilal", "bilal@example.com")
	h.signIn(t, "sara")
	h.signIn(t, "bilal")

	first := decodeBody(t, h.do(t, "sara", http.MethodPost, "/v1/notifications/devices",
		map[string]any{"deviceId": "family-tablet", "platform": "android"}), http.StatusOK)
	second := decodeBody(t, h.do(t, "bilal", http.MethodPost, "/v1/notifications/devices",
		map[string]any{"deviceId": "family-tablet", "platform": "android"}), http.StatusOK)

	if first["id"] == second["id"] {
		t.Error("two users share one device registration")
	}
}

func TestAnUnusableRegistrationIsRejected(t *testing.T) {
	h := newHarness(t)
	h.account("sara", "sara@example.com")
	h.signIn(t, "sara")

	cases := []struct {
		name string
		body map[string]any
	}{
		{"no device id", map[string]any{"deviceId": "  ", "platform": "ios"}},
		{"unknown platform", map[string]any{"deviceId": "d1", "platform": "blackberry"}},
		{"device id too long", map[string]any{
			"deviceId": strings.Repeat("x", 200), "platform": "ios",
		}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			rec := h.do(t, "sara", http.MethodPost, "/v1/notifications/devices", testCase.body)
			// 422 rather than 400: the request was understood, and the response
			// names the field that is wrong.
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422\nbody: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// --- Reminders --------------------------------------------------------------

func TestAScheduledDoseProducesAReminderFifteenMinutesEarlier(t *testing.T) {
	h := newHarness(t)
	h.account("sara", "sara@example.com")
	seniorID := h.createCircle(t, "sara", "Amma", "Asia/Karachi")

	medicationID := h.createMedication(t, "sara", seniorID, "Metformin")
	dueAt := time.Now().Add(4 * time.Hour).UTC().Truncate(time.Minute)
	doseID := h.addDose(t, "sara", medicationID, dueAt)

	doses := remindersOfType(h.reminders(t, "sara"), "MEDICATION_REMINDER")
	if len(doses) != 1 {
		t.Fatalf("got %d medication reminders, want 1", len(doses))
	}

	reminder := doses[0]
	if reminder["entityId"] != doseID {
		t.Errorf("entityId = %v, want the dose %s", reminder["entityId"], doseID)
	}
	if reminder["entityType"] != "medication_dose" {
		t.Errorf("entityType = %v, want medication_dose", reminder["entityType"])
	}
	if reminder["seniorTimezone"] != "Asia/Karachi" {
		t.Errorf("seniorTimezone = %v, want the senior's", reminder["seniorTimezone"])
	}

	fireAt, err := time.Parse(time.RFC3339, reminder["fireAt"].(string))
	if err != nil {
		t.Fatalf("parse fireAt: %v", err)
	}
	if want := dueAt.Add(-15 * time.Minute); !fireAt.Equal(want) {
		t.Errorf("fireAt = %s, want %s", fireAt, want)
	}
}

func TestAnAppointmentRemindsAnHourEarlier(t *testing.T) {
	h := newHarness(t)
	h.account("sara", "sara@example.com")
	seniorID := h.createCircle(t, "sara", "Amma", "Asia/Karachi")

	at := time.Now().Add(6 * time.Hour).UTC().Truncate(time.Minute)
	appointmentID := h.bookAppointment(t, "sara", seniorID, at)

	booked := remindersOfType(h.reminders(t, "sara"), "APPOINTMENT_REMINDER")
	if len(booked) != 1 {
		t.Fatalf("got %d appointment reminders, want 1", len(booked))
	}
	if booked[0]["entityId"] != appointmentID {
		t.Errorf("entityId = %v, want %s", booked[0]["entityId"], appointmentID)
	}

	fireAt, err := time.Parse(time.RFC3339, booked[0]["fireAt"].(string))
	if err != nil {
		t.Fatalf("parse fireAt: %v", err)
	}
	if want := at.Add(-time.Hour); !fireAt.Equal(want) {
		t.Errorf("fireAt = %s, want %s", fireAt, want)
	}
}

func TestCancellingAnAppointmentRemovesItsReminder(t *testing.T) {
	// Nothing cancels the reminder explicitly. The appointment stops being
	// scheduled, so the plan stops containing it (plans/phase8.md §22).
	h := newHarness(t)
	h.account("sara", "sara@example.com")
	seniorID := h.createCircle(t, "sara", "Amma", "Asia/Karachi")

	appointmentID := h.bookAppointment(t, "sara", seniorID, time.Now().Add(6*time.Hour))

	if got := len(remindersOfType(h.reminders(t, "sara"), "APPOINTMENT_REMINDER")); got != 1 {
		t.Fatalf("before cancelling, got %d reminders, want 1", got)
	}

	rec := h.do(t, "sara", http.MethodPost, "/v1/appointments/"+appointmentID+"/cancel", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel: status = %d\nbody: %s", rec.Code, rec.Body.String())
	}

	if got := len(remindersOfType(h.reminders(t, "sara"), "APPOINTMENT_REMINDER")); got != 0 {
		t.Errorf("after cancelling, got %d reminders, want 0", got)
	}
}

func TestMovingAnAppointmentReplacesItsReminder(t *testing.T) {
	h := newHarness(t)
	h.account("sara", "sara@example.com")
	seniorID := h.createCircle(t, "sara", "Amma", "Asia/Karachi")

	appointmentID := h.bookAppointment(t, "sara", seniorID, time.Now().Add(6*time.Hour))

	before := remindersOfType(h.reminders(t, "sara"), "APPOINTMENT_REMINDER")
	if len(before) != 1 {
		t.Fatalf("got %d reminders, want 1", len(before))
	}

	moved := time.Now().Add(9 * time.Hour).UTC().Truncate(time.Minute)
	rec := h.do(t, "sara", http.MethodPatch, "/v1/appointments/"+appointmentID,
		map[string]any{"scheduledAt": moved.Format(time.RFC3339)})
	if rec.Code != http.StatusOK {
		t.Fatalf("move: status = %d\nbody: %s", rec.Code, rec.Body.String())
	}

	after := remindersOfType(h.reminders(t, "sara"), "APPOINTMENT_REMINDER")
	if len(after) != 1 {
		t.Fatalf("got %d reminders after moving, want 1", len(after))
	}
	if before[0]["id"] == after[0]["id"] {
		t.Error("the reminder kept its id after the appointment moved, so the stale one would survive")
	}
}

func TestTakingADoseRemovesItsReminder(t *testing.T) {
	h := newHarness(t)
	h.account("sara", "sara@example.com")
	seniorID := h.createCircle(t, "sara", "Amma", "Asia/Karachi")

	medicationID := h.createMedication(t, "sara", seniorID, "Metformin")
	doseID := h.addDose(t, "sara", medicationID, time.Now().Add(4*time.Hour))

	if got := len(remindersOfType(h.reminders(t, "sara"), "MEDICATION_REMINDER")); got != 1 {
		t.Fatalf("got %d reminders, want 1", got)
	}

	rec := h.do(t, "sara", http.MethodPost,
		"/v1/medications/"+medicationID+"/instances/"+doseID+"/take", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("take: status = %d\nbody: %s", rec.Code, rec.Body.String())
	}

	if got := len(remindersOfType(h.reminders(t, "sara"), "MEDICATION_REMINDER")); got != 0 {
		t.Errorf("after taking the dose, got %d reminders, want 0", got)
	}
}

func TestAskingTwiceGivesTheSameReminderIdentifiers(t *testing.T) {
	// What makes the device's reconciliation idempotent: it can fetch the plan
	// after every launch and schedule nothing twice (plans/phase8.md §§25, 26).
	h := newHarness(t)
	h.account("sara", "sara@example.com")
	seniorID := h.createCircle(t, "sara", "Amma", "Asia/Karachi")

	medicationID := h.createMedication(t, "sara", seniorID, "Metformin")
	h.addDose(t, "sara", medicationID, time.Now().Add(4*time.Hour))
	h.bookAppointment(t, "sara", seniorID, time.Now().Add(6*time.Hour))

	first, second := h.reminders(t, "sara"), h.reminders(t, "sara")

	if len(first) != 2 || len(second) != 2 {
		t.Fatalf("got %d then %d reminders, want 2 each", len(first), len(second))
	}
	for i := range first {
		if first[i]["id"] != second[i]["id"] {
			t.Errorf("reminder %d changed id between requests", i)
		}
	}
}

func TestTurningACategoryOffEmptiesItFromThePlan(t *testing.T) {
	h := newHarness(t)
	h.account("sara", "sara@example.com")
	seniorID := h.createCircle(t, "sara", "Amma", "Asia/Karachi")

	medicationID := h.createMedication(t, "sara", seniorID, "Metformin")
	h.addDose(t, "sara", medicationID, time.Now().Add(4*time.Hour))
	h.bookAppointment(t, "sara", seniorID, time.Now().Add(6*time.Hour))

	decodeBody(t, h.do(t, "sara", http.MethodPatch, "/v1/notifications/preferences",
		map[string]any{"medicationReminders": false}), http.StatusOK)

	plan := h.reminders(t, "sara")
	if got := len(remindersOfType(plan, "MEDICATION_REMINDER")); got != 0 {
		t.Errorf("got %d medication reminders, want 0", got)
	}
	if got := len(remindersOfType(plan, "APPOINTMENT_REMINDER")); got != 1 {
		t.Errorf("got %d appointment reminders, want 1", got)
	}
}

func TestACaregiverWithoutMedicationAccessIsNotRemindedAboutDoses(t *testing.T) {
	h := newHarness(t)
	h.account("sara", "sara@example.com")
	h.account("bilal", "bilal@example.com")
	seniorID := h.createCircle(t, "sara", "Amma", "Asia/Karachi")

	// Bilal may see appointments, and nothing else.
	h.join(t, "bilal", seniorID, care.RoleFamilyMember,
		care.PermissionSeniorView, care.PermissionAppointmentsView)

	medicationID := h.createMedication(t, "sara", seniorID, "Metformin")
	h.addDose(t, "sara", medicationID, time.Now().Add(4*time.Hour))
	h.bookAppointment(t, "sara", seniorID, time.Now().Add(6*time.Hour))

	plan := h.reminders(t, "bilal")
	if got := len(remindersOfType(plan, "MEDICATION_REMINDER")); got != 0 {
		t.Errorf("got %d medication reminders, want 0", got)
	}
	if got := len(remindersOfType(plan, "APPOINTMENT_REMINDER")); got != 1 {
		t.Errorf("got %d appointment reminders, want 1", got)
	}
}

func TestAStrangerHasAnEmptyPlan(t *testing.T) {
	h := newHarness(t)
	h.account("sara", "sara@example.com")
	h.account("stranger", "stranger@example.com")
	h.signIn(t, "stranger")

	seniorID := h.createCircle(t, "sara", "Amma", "Asia/Karachi")
	medicationID := h.createMedication(t, "sara", seniorID, "Metformin")
	h.addDose(t, "sara", medicationID, time.Now().Add(4*time.Hour))

	if plan := h.reminders(t, "stranger"); len(plan) != 0 {
		t.Errorf("a stranger has %d reminders, want 0", len(plan))
	}
}

func TestRevokingACaregiverEmptiesTheirPlan(t *testing.T) {
	// Server-side eligibility, not a client-side courtesy: the next plan the
	// revoked caregiver's app fetches contains nothing, so its reconciliation
	// cancels everything it had scheduled (plans/phase8.md §23).
	h := newHarness(t)
	h.account("sara", "sara@example.com")
	h.account("bilal", "bilal@example.com")
	seniorID := h.createCircle(t, "sara", "Amma", "Asia/Karachi")

	_, relationshipID := h.join(t, "bilal", seniorID, care.RoleFamilyMember,
		care.PermissionSeniorView, care.PermissionMedicationsView)

	medicationID := h.createMedication(t, "sara", seniorID, "Metformin")
	h.addDose(t, "sara", medicationID, time.Now().Add(4*time.Hour))

	if got := len(h.reminders(t, "bilal")); got != 1 {
		t.Fatalf("before revocation Bilal has %d reminders, want 1", got)
	}

	if _, err := h.relationships.RevokeMembership(context.Background(), relationshipID); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	if got := len(h.reminders(t, "bilal")); got != 0 {
		t.Errorf("after revocation Bilal has %d reminders, want 0", got)
	}
	// Sara still gets hers.
	if got := len(h.reminders(t, "sara")); got != 1 {
		t.Errorf("Sara has %d reminders, want 1", got)
	}
}

func TestAnAssignedAppointmentRemindsOnlyTheAssignee(t *testing.T) {
	h := newHarness(t)
	h.account("sara", "sara@example.com")
	h.account("bilal", "bilal@example.com")
	seniorID := h.createCircle(t, "sara", "Amma", "Asia/Karachi")

	bilalID, _ := h.join(t, "bilal", seniorID, care.RoleFamilyMember,
		care.PermissionSeniorView, care.PermissionAppointmentsView)

	at := time.Now().Add(6 * time.Hour).UTC().Truncate(time.Minute)
	rec := h.do(t, "sara", http.MethodPost, "/v1/seniors/"+seniorID+"/appointments", map[string]any{
		"title":          "Cardiology",
		"scheduledAt":    at.Format(time.RFC3339),
		"assignedUserId": bilalID.String(),
	})
	decodeBody(t, rec, http.StatusCreated)

	if got := len(remindersOfType(h.reminders(t, "bilal"), "APPOINTMENT_REMINDER")); got != 1 {
		t.Errorf("the assignee has %d reminders, want 1", got)
	}
	if got := len(remindersOfType(h.reminders(t, "sara"), "APPOINTMENT_REMINDER")); got != 0 {
		t.Errorf("a non-assignee has %d reminders, want 0", got)
	}
}

func TestNothingInAPlanNamesAMedicine(t *testing.T) {
	// The plan reaches a lock screen. It must carry no drug name and no dosage
	// for the device to be able to render one (plans/phase8.md §§16, 17, 47).
	h := newHarness(t)
	h.account("sara", "sara@example.com")
	seniorID := h.createCircle(t, "sara", "Amma", "Asia/Karachi")

	medicationID := h.createMedication(t, "sara", seniorID, "Metformin")
	h.addDose(t, "sara", medicationID, time.Now().Add(4*time.Hour))

	rec := h.do(t, "sara", http.MethodGet, "/v1/notifications/reminders", nil)
	decodeBody(t, rec, http.StatusOK)

	for _, forbidden := range []string{"Metformin", "500 mg", "tablet", "title", "body"} {
		if strings.Contains(rec.Body.String(), forbidden) {
			t.Errorf("the plan contains %q: %s", forbidden, rec.Body.String())
		}
	}
}

// --- Fixtures ---------------------------------------------------------------

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

func (h *harness) bookAppointment(t *testing.T, token, seniorID string, at time.Time) string {
	t.Helper()

	rec := h.do(t, token, http.MethodPost, "/v1/seniors/"+seniorID+"/appointments", map[string]any{
		"title":       "Cardiology",
		"scheduledAt": at.UTC().Format(time.RFC3339),
	})
	body := decodeBody(t, rec, http.StatusCreated)

	id, _ := body["id"].(string)
	if id == "" {
		t.Fatalf("no appointment id in response: %s", rec.Body.String())
	}
	return id
}

func TestARecurringTaskProducesAReminderForEachOccurrence(t *testing.T) {
	// The occurrences are materialised by the task service when the plan asks
	// for the window, so a recurring task reminds without anybody having opened
	// the task screen and without a second recurrence engine
	// (plans/phase8.md §§13, 21).
	h := newHarness(t)
	h.account("sara", "sara@example.com")
	seniorID := h.createCircle(t, "sara", "Amma", "UTC")

	rec := h.do(t, "sara", http.MethodPost, "/v1/seniors/"+seniorID+"/tasks", map[string]any{
		"title":      "Morning walk",
		"recurrence": map[string]any{"frequency": "daily"},
		"dueTime":    "09:00",
	})
	decodeBody(t, rec, http.StatusCreated)

	reminders := remindersOfType(h.reminders(t, "sara"), "TASK_REMINDER")

	// A daily task over a seven-day horizon: at least six, since today's 09:00
	// may already have passed when the suite runs.
	if len(reminders) < 6 {
		t.Fatalf("got %d task reminders over the horizon, want at least 6", len(reminders))
	}

	for _, reminder := range reminders {
		if reminder["entityType"] != "task_instance" {
			t.Errorf("entityType = %v, want task_instance", reminder["entityType"])
		}

		fireAt, err := time.Parse(time.RFC3339, reminder["fireAt"].(string))
		if err != nil {
			t.Fatalf("parse fireAt: %v", err)
		}
		// 09:00 UTC less fifteen minutes.
		if fireAt.Hour() != 8 || fireAt.Minute() != 45 {
			t.Errorf("fireAt = %s, want 08:45 on its day", fireAt)
		}
	}

	// And every one of them is distinct, so the device schedules seven
	// notifications rather than one seven times.
	seen := map[string]bool{}
	for _, reminder := range reminders {
		id, _ := reminder["id"].(string)
		if seen[id] {
			t.Errorf("duplicate reminder id %s", id)
		}
		seen[id] = true
	}
}

func TestAPlanNeverExceedsTheDeviceLimit(t *testing.T) {
	// iOS silently drops pending local notifications past its own cap, so a
	// larger plan would be partly imaginary (plans/phase8.md §46).
	h := newHarness(t)
	h.account("sara", "sara@example.com")
	seniorID := h.createCircle(t, "sara", "Amma", "UTC")

	// Four daily tasks over a seven-day horizon is more than the cap once the
	// hourly doses below are added.
	for _, title := range []string{"Walk", "Stretch", "Read", "Call"} {
		decodeBody(t, h.do(t, "sara", http.MethodPost, "/v1/seniors/"+seniorID+"/tasks",
			map[string]any{
				"title":      title,
				"recurrence": map[string]any{"frequency": "daily"},
				"dueTime":    "09:00",
			}), http.StatusCreated)
	}

	medicationID := h.createMedication(t, "sara", seniorID, "Metformin")
	for hour := 2; hour < 40; hour++ {
		h.addDose(t, "sara", medicationID, time.Now().Add(time.Duration(hour)*time.Hour))
	}

	plan := h.reminders(t, "sara")
	if len(plan) > 50 {
		t.Fatalf("plan has %d reminders, want at most 50", len(plan))
	}
	if len(plan) != 50 {
		t.Fatalf("plan has %d reminders, want the cap of 50 to have been reached", len(plan))
	}

	// The survivors are the soonest, in order.
	var previous time.Time
	for i, reminder := range plan {
		fireAt, err := time.Parse(time.RFC3339, reminder["fireAt"].(string))
		if err != nil {
			t.Fatalf("parse fireAt: %v", err)
		}
		if i > 0 && fireAt.Before(previous) {
			t.Fatalf("reminder %d fires before its predecessor", i)
		}
		previous = fireAt
	}
}
