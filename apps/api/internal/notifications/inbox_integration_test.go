package notifications_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/meracare/api/internal/care"
)

// The inbox over the real router and the real database. The questions here are
// the ones a schema and a handler answer together: can one person reach
// another's notifications, does the badge match the list, does a cursor page
// without dropping anything (plans/phase11.md §§8, 28, 29, 41).

// seed writes one notification directly, so a test can arrange an inbox without
// waiting for a scheduler pass. It is the only place in these tests that writes
// SQL; everything read back goes through the API.
func (h *harness) seed(
	t *testing.T,
	recipient uuid.UUID,
	seniorID string,
	notificationType string,
	scheduledFor time.Time,
) uuid.UUID {
	t.Helper()

	senior, err := uuid.Parse(seniorID)
	if err != nil {
		t.Fatalf("parse senior id: %v", err)
	}

	var id uuid.UUID
	err = h.pool.QueryRow(context.Background(), `
		INSERT INTO notifications (
			recipient_user_id, senior_id, notification_type, title, body,
			entity_type, entity_id, scheduled_for, dedupe_key, available_at
		)
		VALUES ($1, $2, $3, 'Medication reminder', 'A dose is due for Amma at 08:00.',
		        'medication_dose', gen_random_uuid(), $4, $5, $4)
		RETURNING id`,
		recipient, senior, notificationType, scheduledFor, uuid.NewString(),
	).Scan(&id)
	if err != nil {
		t.Fatalf("seed notification: %v", err)
	}
	return id
}

func (h *harness) inbox(t *testing.T, token, query string) map[string]any {
	t.Helper()

	path := "/v1/notifications"
	if query != "" {
		path += "?" + query
	}
	return decodeBody(t, h.do(t, token, http.MethodGet, path, nil), http.StatusOK)
}

func inboxItems(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()

	raw, ok := body["items"].([]any)
	if !ok {
		t.Fatalf("no items in inbox response: %v", body)
	}

	items := make([]map[string]any, 0, len(raw))
	for _, entry := range raw {
		item, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("inbox item is not an object: %v", entry)
		}
		items = append(items, item)
	}
	return items
}

func unreadCount(t *testing.T, body map[string]any) int {
	t.Helper()

	count, ok := body["unreadCount"].(float64)
	if !ok {
		t.Fatalf("no unreadCount in response: %v", body)
	}
	return int(count)
}

func TestInboxShowsOnlyTheCallersOwnNotifications(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.account("daughter", "daughter@example.com")
	h.account("son", "son@example.com")

	seniorID := h.createCircle(t, "daughter", "Amma", "Asia/Kolkata")
	daughter := h.signIn(t, "daughter")
	son, _ := h.join(t, "son", seniorID, care.RoleFamilyMember, care.PermissionMedicationsView)

	past := time.Now().Add(-time.Hour)
	h.seed(t, daughter, seniorID, "MEDICATION_REMINDER", past)
	h.seed(t, daughter, seniorID, "TASK_REMINDER", past.Add(time.Minute))
	sonsOwn := h.seed(t, son, seniorID, "MEDICATION_REMINDER", past)

	daughtersInbox := inboxItems(t, h.inbox(t, "daughter", ""))
	if len(daughtersInbox) != 2 {
		t.Fatalf("daughter sees %d notifications, want her own 2", len(daughtersInbox))
	}
	for _, item := range daughtersInbox {
		if item["id"] == sonsOwn.String() {
			t.Fatal("the daughter can see the son's notification")
		}
	}

	sonsInbox := inboxItems(t, h.inbox(t, "son", ""))
	if len(sonsInbox) != 1 || sonsInbox[0]["id"] != sonsOwn.String() {
		t.Fatalf("son sees %v, want only his own", sonsInbox)
	}
}

func TestANotificationScheduledForTheFutureIsNotInTheInboxYet(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.account("daughter", "daughter@example.com")

	seniorID := h.createCircle(t, "daughter", "Amma", "Asia/Kolkata")
	daughter := h.signIn(t, "daughter")

	h.seed(t, daughter, seniorID, "MEDICATION_REMINDER", time.Now().Add(-time.Minute))
	h.seed(t, daughter, seniorID, "MEDICATION_REMINDER", time.Now().Add(45*time.Minute))

	body := h.inbox(t, "daughter", "")
	if items := inboxItems(t, body); len(items) != 1 {
		t.Fatalf("inbox has %d items, want only the one that has arrived", len(items))
	}
	if got := unreadCount(t, body); got != 1 {
		t.Errorf("unreadCount = %d, want 1 — a notification that has not arrived is not unread", got)
	}
}

func TestMarkingOneReadChangesThatOneAndTheBadge(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.account("daughter", "daughter@example.com")

	seniorID := h.createCircle(t, "daughter", "Amma", "Asia/Kolkata")
	daughter := h.signIn(t, "daughter")

	past := time.Now().Add(-time.Hour)
	first := h.seed(t, daughter, seniorID, "MEDICATION_REMINDER", past)
	h.seed(t, daughter, seniorID, "TASK_REMINDER", past.Add(time.Minute))

	if got := unreadCount(t, h.inbox(t, "daughter", "")); got != 2 {
		t.Fatalf("unreadCount = %d, want 2 before anything is read", got)
	}

	marked := decodeBody(t,
		h.do(t, "daughter", http.MethodPatch, "/v1/notifications/"+first.String()+"/read", nil),
		http.StatusOK)
	if marked["read"] != true {
		t.Errorf("read = %v, want true", marked["read"])
	}
	if marked["readAt"] == "" {
		t.Error("readAt is empty on a notification that was just read")
	}

	body := h.inbox(t, "daughter", "")
	if got := unreadCount(t, body); got != 1 {
		t.Errorf("unreadCount = %d, want 1 after reading one", got)
	}
}

func TestMarkingReadIsIdempotent(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.account("daughter", "daughter@example.com")

	seniorID := h.createCircle(t, "daughter", "Amma", "Asia/Kolkata")
	daughter := h.signIn(t, "daughter")
	id := h.seed(t, daughter, seniorID, "MEDICATION_REMINDER", time.Now().Add(-time.Hour))

	path := "/v1/notifications/" + id.String() + "/read"
	first := decodeBody(t, h.do(t, "daughter", http.MethodPatch, path, nil), http.StatusOK)
	second := decodeBody(t, h.do(t, "daughter", http.MethodPatch, path, nil), http.StatusOK)

	// The original moment survives, so "when did they see it?" stays answerable.
	if first["readAt"] != second["readAt"] {
		t.Errorf("readAt changed on a second read: %v then %v", first["readAt"], second["readAt"])
	}
}

func TestOneUserCannotMarkAnothersNotificationRead(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.account("daughter", "daughter@example.com")
	h.account("son", "son@example.com")

	seniorID := h.createCircle(t, "daughter", "Amma", "Asia/Kolkata")
	daughter := h.signIn(t, "daughter")
	h.join(t, "son", seniorID, care.RoleFamilyMember, care.PermissionMedicationsView)

	id := h.seed(t, daughter, seniorID, "MEDICATION_REMINDER", time.Now().Add(-time.Hour))

	// Not 403: a distinguishable answer would confirm the notification is real
	// (plans/phase11.md §8).
	rec := h.do(t, "son", http.MethodPatch, "/v1/notifications/"+id.String()+"/read", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}

	// And it is genuinely still unread for its owner.
	if got := unreadCount(t, h.inbox(t, "daughter", "")); got != 1 {
		t.Errorf("unreadCount = %d, want 1 — the son's request must have changed nothing", got)
	}
}

func TestAnInventedNotificationIdIsNotFound(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.account("daughter", "daughter@example.com")
	h.signIn(t, "daughter")

	for _, id := range []string{uuid.NewString(), "not-a-uuid"} {
		rec := h.do(t, "daughter", http.MethodPatch, "/v1/notifications/"+id+"/read", nil)
		if rec.Code != http.StatusNotFound {
			t.Errorf("id %q: status = %d, want 404", id, rec.Code)
		}
	}
}

func TestMarkAllReadAffectsOnlyTheCaller(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.account("daughter", "daughter@example.com")
	h.account("son", "son@example.com")

	seniorID := h.createCircle(t, "daughter", "Amma", "Asia/Kolkata")
	daughter := h.signIn(t, "daughter")
	son, _ := h.join(t, "son", seniorID, care.RoleFamilyMember, care.PermissionMedicationsView)

	past := time.Now().Add(-time.Hour)
	h.seed(t, daughter, seniorID, "MEDICATION_REMINDER", past)
	h.seed(t, daughter, seniorID, "TASK_REMINDER", past.Add(time.Minute))
	h.seed(t, son, seniorID, "MEDICATION_REMINDER", past)

	body := decodeBody(t,
		h.do(t, "daughter", http.MethodPost, "/v1/notifications/read-all", nil), http.StatusOK)

	if marked, _ := body["markedRead"].(float64); marked != 2 {
		t.Errorf("markedRead = %v, want 2", body["markedRead"])
	}
	if unread, _ := body["unreadCount"].(float64); unread != 0 {
		t.Errorf("unreadCount = %v, want 0", body["unreadCount"])
	}

	if got := unreadCount(t, h.inbox(t, "son", "")); got != 1 {
		t.Errorf("the son's unreadCount = %d, want 1 — his inbox must be untouched", got)
	}
}

func TestMarkAllReadOnAnEmptyInboxIsHarmless(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.account("daughter", "daughter@example.com")
	h.signIn(t, "daughter")

	body := decodeBody(t,
		h.do(t, "daughter", http.MethodPost, "/v1/notifications/read-all", nil), http.StatusOK)

	if marked, _ := body["markedRead"].(float64); marked != 0 {
		t.Errorf("markedRead = %v, want 0", body["markedRead"])
	}
}

func TestTheInboxPagesNewestFirstWithoutDroppingAnything(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.account("daughter", "daughter@example.com")

	seniorID := h.createCircle(t, "daughter", "Amma", "Asia/Kolkata")
	daughter := h.signIn(t, "daughter")

	// Several share an instant, which is the case a timestamp-only cursor gets
	// wrong: the tie-breaking id is what stops one being repeated or skipped at
	// the page boundary.
	base := time.Now().Add(-24 * time.Hour)
	const total = 7
	for index := range total {
		at := base.Add(time.Duration(index/3) * time.Minute)
		h.seed(t, daughter, seniorID, "MEDICATION_REMINDER", at)
	}

	seen := make(map[string]bool)
	cursor := ""
	pages := 0

	for {
		query := "limit=3"
		if cursor != "" {
			query += "&cursor=" + cursor
		}

		body := h.inbox(t, "daughter", query)
		items := inboxItems(t, body)
		pages++

		for _, item := range items {
			id, _ := item["id"].(string)
			if seen[id] {
				t.Fatalf("notification %s appeared on two pages", id)
			}
			seen[id] = true
		}

		next, ok := body["nextCursor"].(string)
		if !ok || next == "" {
			break
		}
		cursor = next

		if pages > 10 {
			t.Fatal("paging did not terminate")
		}
	}

	if len(seen) != total {
		t.Errorf("saw %d notifications across %d pages, want %d", len(seen), pages, total)
	}
}

func TestAnUnreadableCursorIsRefusedRatherThanRestarted(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.account("daughter", "daughter@example.com")
	h.signIn(t, "daughter")

	// Silently starting again would show somebody the top of a list they were
	// halfway down.
	rec := h.do(t, "daughter", http.MethodGet, "/v1/notifications?cursor=nonsense", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestTheInboxNeedsAuthentication(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	for _, request := range []struct{ method, path string }{
		{http.MethodGet, "/v1/notifications"},
		{http.MethodPost, "/v1/notifications/read-all"},
		{http.MethodPatch, "/v1/notifications/" + uuid.NewString() + "/read"},
	} {
		rec := h.do(t, "", request.method, request.path, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status = %d, want 401", request.method, request.path, rec.Code)
		}
	}
}

func TestTheInboxNeverExposesDeliveryState(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.account("daughter", "daughter@example.com")

	seniorID := h.createCircle(t, "daughter", "Amma", "Asia/Kolkata")
	daughter := h.signIn(t, "daughter")
	h.seed(t, daughter, seniorID, "MEDICATION_REMINDER", time.Now().Add(-time.Hour))

	items := inboxItems(t, h.inbox(t, "daughter", ""))
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}

	// Whether a push reached a phone is operational, not something the reader
	// has any use for (plans/phase11.md §6).
	for _, field := range []string{"deliveryStatus", "attempts", "lastError", "pushToken"} {
		if _, present := items[0][field]; present {
			t.Errorf("inbox item exposes %q: %v", field, items[0])
		}
	}
}

func TestPreferencesCoverTheTwoNewCategories(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.account("daughter", "daughter@example.com")
	h.signIn(t, "daughter")

	defaults := decodeBody(t,
		h.do(t, "daughter", http.MethodGet, "/v1/notifications/preferences", nil), http.StatusOK)

	for _, key := range []string{"overdueTaskAlerts", "careActivity"} {
		if defaults[key] != true {
			t.Errorf("%s defaults to %v, want true", key, defaults[key])
		}
	}

	updated := decodeBody(t, h.do(t, "daughter", http.MethodPatch, "/v1/notifications/preferences",
		map[string]any{"careActivity": false}), http.StatusOK)

	if updated["careActivity"] != false {
		t.Errorf("careActivity = %v after switching it off, want false", updated["careActivity"])
	}
	// Turning one category off must not silently switch the others off with it.
	for _, key := range []string{"overdueTaskAlerts", "medicationReminders", "taskReminders"} {
		if updated[key] != true {
			t.Errorf("%s = %v after an unrelated change, want true", key, updated[key])
		}
	}
}
