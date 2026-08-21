package notifications_test

import (
	"context"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/meracare/api/internal/care"
	"github.com/meracare/api/internal/notifications"
	"github.com/meracare/api/pkg/logging"
)

// The delivery half of the scheduler, against a real database.
//
// These are the questions a fake repository cannot answer: does the unique index
// actually stop a second pass writing the same notification, do two workers
// claiming at the same instant take different rows, does a rejected token really
// disappear from the devices table (plans/phase11.md §§20, 37, 39, 62).

// countingSender records what it was asked to send and answers as told.
type countingSender struct {
	mu       sync.Mutex
	sent     []notifications.PushMessage
	outcome  func(notifications.PushMessage) notifications.PushOutcome
	failWith error
}

func (s *countingSender) Send(
	_ context.Context,
	messages []notifications.PushMessage,
) ([]notifications.PushOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.failWith != nil {
		return nil, s.failWith
	}

	s.sent = append(s.sent, messages...)

	outcomes := make([]notifications.PushOutcome, 0, len(messages))
	for _, message := range messages {
		if s.outcome != nil {
			outcomes = append(outcomes, s.outcome(message))
			continue
		}
		outcomes = append(outcomes, notifications.PushOutcome{Token: message.Token, Delivered: true})
	}
	return outcomes, nil
}

func (s *countingSender) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sent)
}

func (s *countingSender) messages() []notifications.PushMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]notifications.PushMessage(nil), s.sent...)
}

// fixedRoster is the membership list a pass sweeps.
type fixedRoster struct {
	memberships []notifications.UserMembership
}

func (r fixedRoster) ActiveMemberships(context.Context) ([]notifications.UserMembership, error) {
	return r.memberships, nil
}

// fixedSchedule returns the same due items for every senior.
type fixedSchedule struct {
	due []notifications.Due
}

func (f fixedSchedule) Upcoming(
	_ context.Context,
	_ uuid.UUID,
	from, to time.Time,
) ([]notifications.Due, error) {
	found := make([]notifications.Due, 0, len(f.due))
	for _, item := range f.due {
		if !item.At.Before(from) && item.At.Before(to) {
			found = append(found, item)
		}
	}
	return found, nil
}

// schedulerFixture is a scheduler wired to the real repository and fake sources.
type schedulerFixture struct {
	harness   *harness
	scheduler *notifications.Scheduler
	sender    *countingSender
	user      uuid.UUID
	seniorID  uuid.UUID
	doseAt    time.Time
}

// newSchedulerFixture builds a circle with one dose falling due, and a scheduler
// pointed at it.
func newSchedulerFixture(t *testing.T, sender *countingSender) *schedulerFixture {
	t.Helper()

	h := newHarness(t)
	h.account("daughter", "daughter@example.com")

	seniorID := h.createCircle(t, "daughter", "Amma", "Asia/Kolkata")
	user := h.signIn(t, "daughter")

	senior, err := uuid.Parse(seniorID)
	if err != nil {
		t.Fatalf("parse senior id: %v", err)
	}

	doseAt := time.Now().Add(20 * time.Minute).Truncate(time.Second)

	scheduler := notifications.NewScheduler(notifications.SchedulerDependencies{
		Repository: notifications.NewRepository(h.pool),
		Sender:     sender,
		Roster: fixedRoster{memberships: []notifications.UserMembership{{
			UserID: user,
			Senior: notifications.Senior{
				ID: senior, DisplayName: "Amma", Timezone: "Asia/Kolkata",
			},
			Permissions: care.Normalise([]care.Permission{
				care.PermissionMedicationsView,
				care.PermissionTasksView,
				care.PermissionAppointmentsView,
				care.PermissionActivityView,
			}),
		}}},
		Reminders: map[notifications.Type]notifications.ScheduleSource{
			notifications.TypeMedicationReminder: fixedSchedule{due: []notifications.Due{
				{EntityID: uuid.New(), At: doseAt},
			}},
		},
		Logger: logging.New(io.Discard, logging.Options{Level: "error"}),
	}, notifications.SchedulerOptions{})

	return &schedulerFixture{
		harness:   h,
		scheduler: scheduler,
		sender:    sender,
		user:      user,
		seniorID:  senior,
		doseAt:    doseAt,
	}
}

// countNotifications reports how many rows the user has, whatever their state.
func (f *schedulerFixture) countNotifications(t *testing.T) int {
	t.Helper()

	var count int
	err := f.harness.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM notifications WHERE recipient_user_id = $1`, f.user).Scan(&count)
	if err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	return count
}

func (f *schedulerFixture) state(t *testing.T) (status string, attempts int, availableAt time.Time) {
	t.Helper()

	err := f.harness.pool.QueryRow(context.Background(),
		`SELECT delivery_status, attempts, available_at FROM notifications
		  WHERE recipient_user_id = $1 LIMIT 1`, f.user).Scan(&status, &attempts, &availableAt)
	if err != nil {
		t.Fatalf("read notification state: %v", err)
	}
	return status, attempts, availableAt
}

// registerDevice gives the user a reachable installation.
func (f *schedulerFixture) registerDevice(t *testing.T, token string) {
	t.Helper()

	rec := f.harness.do(t, "daughter", http.MethodPost, "/v1/notifications/devices", map[string]any{
		"deviceId":  "install-" + token,
		"platform":  "ios",
		"pushToken": token,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("register device: status = %d, body %s", rec.Code, rec.Body.String())
	}
}

func (f *schedulerFixture) activeTokens(t *testing.T) int {
	t.Helper()

	var count int
	err := f.harness.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM notification_devices
		  WHERE user_id = $1 AND active AND push_token IS NOT NULL`, f.user).Scan(&count)
	if err != nil {
		t.Fatalf("count devices: %v", err)
	}
	return count
}

func TestRunningTheSamePassTwiceCreatesOneNotification(t *testing.T) {
	t.Parallel()

	f := newSchedulerFixture(t, &countingSender{})
	now := f.doseAt.Add(-16 * time.Minute)

	first, err := f.scheduler.RunOnce(context.Background(), now)
	if err != nil {
		t.Fatalf("first pass: %v", err)
	}
	if first.Created != 1 {
		t.Fatalf("first pass created %d, want 1", first.Created)
	}

	// The scheduler runs every minute over an overlapping window. Nothing about
	// the second pass may produce a second notification.
	second, err := f.scheduler.RunOnce(context.Background(), now.Add(30*time.Second))
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if second.Created != 0 {
		t.Errorf("second pass created %d, want 0", second.Created)
	}
	if got := f.countNotifications(t); got != 1 {
		t.Errorf("%d notifications exist, want 1", got)
	}
}

func TestANotificationWithNoReachableDeviceIsSkippedNotRetried(t *testing.T) {
	t.Parallel()

	sender := &countingSender{}
	f := newSchedulerFixture(t, sender)

	// Materialise, then deliver once the moment has arrived.
	if _, err := f.scheduler.RunOnce(context.Background(), f.doseAt.Add(-16*time.Minute)); err != nil {
		t.Fatalf("materialise: %v", err)
	}

	pass, err := f.scheduler.RunOnce(context.Background(), f.doseAt.Add(-14*time.Minute))
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if pass.Skipped != 1 {
		t.Fatalf("pass = %+v, want one skipped", pass)
	}
	if sender.count() != 0 {
		t.Errorf("the sender was called %d times with no device registered", sender.count())
	}

	status, _, _ := f.state(t)
	if status != "skipped" {
		t.Errorf("delivery_status = %q, want skipped", status)
	}

	// And the notification itself survives, unread — push being unavailable
	// costs the buzz, not the notification. It becomes visible in the inbox the
	// moment it is due; that the inbox shows it is covered by the inbox tests,
	// which can seed a row in the past rather than waiting for one
	// (plans/phase11.md §43).
	if got := f.countNotifications(t); got != 1 {
		t.Errorf("%d notifications survive, want the skipped one to remain", got)
	}
	var unread bool
	if err := f.harness.pool.QueryRow(context.Background(),
		`SELECT read_at IS NULL FROM notifications WHERE recipient_user_id = $1`,
		f.user).Scan(&unread); err != nil {
		t.Fatalf("read notification: %v", err)
	}
	if !unread {
		t.Error("a notification nobody could be sent was marked read")
	}
}

func TestADueNotificationIsPushedWithItsOwnWords(t *testing.T) {
	t.Parallel()

	sender := &countingSender{}
	f := newSchedulerFixture(t, sender)
	f.registerDevice(t, "ExponentPushToken[abc]")

	if _, err := f.scheduler.RunOnce(context.Background(), f.doseAt.Add(-16*time.Minute)); err != nil {
		t.Fatalf("materialise: %v", err)
	}

	pass, err := f.scheduler.RunOnce(context.Background(), f.doseAt.Add(-14*time.Minute))
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if pass.Delivered != 1 {
		t.Fatalf("pass = %+v, want one delivered", pass)
	}

	sent := sender.messages()
	if len(sent) != 1 {
		t.Fatalf("sent %d messages, want 1", len(sent))
	}
	if sent[0].Token != "ExponentPushToken[abc]" {
		t.Errorf("token = %q, want the registered one", sent[0].Token)
	}
	if sent[0].Title != "Medication reminder" {
		t.Errorf("title = %q", sent[0].Title)
	}
	// The payload is identifiers only. Anything else would be information
	// travelling outside MeraCare's authorization (plans/phase11.md §58).
	for _, key := range []string{"notificationId", "type", "seniorId", "entityType", "entityId"} {
		if sent[0].Data[key] == "" {
			t.Errorf("payload is missing %q: %v", key, sent[0].Data)
		}
	}
	if len(sent[0].Data) != 5 {
		t.Errorf("payload carries more than identifiers: %v", sent[0].Data)
	}

	status, _, _ := f.state(t)
	if status != "sent" {
		t.Errorf("delivery_status = %q, want sent", status)
	}
}

func TestADeliveredNotificationIsNotSentAgainOnTheNextPass(t *testing.T) {
	t.Parallel()

	sender := &countingSender{}
	f := newSchedulerFixture(t, sender)
	f.registerDevice(t, "ExponentPushToken[abc]")

	ctx := context.Background()
	if _, err := f.scheduler.RunOnce(ctx, f.doseAt.Add(-16*time.Minute)); err != nil {
		t.Fatalf("materialise: %v", err)
	}
	if _, err := f.scheduler.RunOnce(ctx, f.doseAt.Add(-14*time.Minute)); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if _, err := f.scheduler.RunOnce(ctx, f.doseAt.Add(-13*time.Minute)); err != nil {
		t.Fatalf("third pass: %v", err)
	}

	if sender.count() != 1 {
		t.Errorf("the sender was called %d times, want exactly once", sender.count())
	}
}

func TestARejectedTokenIsRetiredRatherThanRetried(t *testing.T) {
	t.Parallel()

	sender := &countingSender{
		outcome: func(message notifications.PushMessage) notifications.PushOutcome {
			return notifications.PushOutcome{
				Token:        message.Token,
				TokenInvalid: true,
				Error:        "DeviceNotRegistered",
			}
		},
	}
	f := newSchedulerFixture(t, sender)
	f.registerDevice(t, "ExponentPushToken[gone]")

	ctx := context.Background()
	if _, err := f.scheduler.RunOnce(ctx, f.doseAt.Add(-16*time.Minute)); err != nil {
		t.Fatalf("materialise: %v", err)
	}
	if _, err := f.scheduler.RunOnce(ctx, f.doseAt.Add(-14*time.Minute)); err != nil {
		t.Fatalf("deliver: %v", err)
	}

	if got := f.activeTokens(t); got != 0 {
		t.Errorf("%d tokens are still active after the provider rejected one", got)
	}
	if status, _, _ := f.state(t); status != "failed" {
		t.Errorf("delivery_status = %q, want failed", status)
	}

	// The device row survives so the same install signing in again is an update
	// rather than a duplicate.
	var rows int
	if err := f.harness.pool.QueryRow(ctx,
		`SELECT count(*) FROM notification_devices WHERE user_id = $1`, f.user).Scan(&rows); err != nil {
		t.Fatalf("count device rows: %v", err)
	}
	if rows != 1 {
		t.Errorf("%d device rows, want the registration to survive with its token cleared", rows)
	}
}

func TestATemporaryFailureIsRetriedAndThenGivenUpOn(t *testing.T) {
	t.Parallel()

	sender := &countingSender{
		outcome: func(message notifications.PushMessage) notifications.PushOutcome {
			return notifications.PushOutcome{
				Token:     message.Token,
				Retryable: true,
				Error:     "MessageRateExceeded",
			}
		},
	}
	f := newSchedulerFixture(t, sender)
	f.registerDevice(t, "ExponentPushToken[busy]")

	ctx := context.Background()
	if _, err := f.scheduler.RunOnce(ctx, f.doseAt.Add(-16*time.Minute)); err != nil {
		t.Fatalf("materialise: %v", err)
	}

	// Each attempt has to be made after the previous backoff has expired,
	// otherwise the row is not yet available — which is itself the behaviour
	// being relied on.
	at := f.doseAt.Add(-14 * time.Minute)
	for attempt := 1; attempt <= 3; attempt++ {
		pass, err := f.scheduler.RunOnce(ctx, at)
		if err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}

		status, attempts, availableAt := f.state(t)
		if attempts != attempt {
			t.Fatalf("attempt %d: attempts = %d", attempt, attempts)
		}

		if attempt < 3 {
			if pass.Retrying != 1 || status != "pending" {
				t.Fatalf("attempt %d: pass = %+v, status = %q, want a retry", attempt, pass, status)
			}
			if !availableAt.After(at) {
				t.Fatalf("attempt %d: available_at = %s is not in the future", attempt, availableAt)
			}
			at = availableAt.Add(time.Second)
			continue
		}

		if pass.Failed != 1 || status != "failed" {
			t.Fatalf("attempt 3: pass = %+v, status = %q, want it abandoned", pass, status)
		}
	}

	if sender.count() != 3 {
		t.Errorf("the sender was called %d times, want the 3 attempts and no more", sender.count())
	}

	// A fourth pass must not resurrect it.
	if _, err := f.scheduler.RunOnce(ctx, at.Add(time.Hour)); err != nil {
		t.Fatalf("fourth pass: %v", err)
	}
	if sender.count() != 3 {
		t.Errorf("the sender was called %d times after the notification was abandoned", sender.count())
	}
}

func TestTwoSchedulersDeliverEachNotificationOnce(t *testing.T) {
	t.Parallel()

	// Two API instances, sweeping at the same instant. The claim is a single
	// UPDATE ... FOR UPDATE SKIP LOCKED, so one takes the row and the other
	// takes nothing rather than waiting for it (plans/phase11.md §37).
	sender := &countingSender{}
	f := newSchedulerFixture(t, sender)
	f.registerDevice(t, "ExponentPushToken[abc]")

	ctx := context.Background()
	if _, err := f.scheduler.RunOnce(ctx, f.doseAt.Add(-16*time.Minute)); err != nil {
		t.Fatalf("materialise: %v", err)
	}

	// A second scheduler over the same database and the same sender.
	second := notifications.NewScheduler(notifications.SchedulerDependencies{
		Repository: notifications.NewRepository(f.harness.pool),
		Sender:     sender,
		Roster:     fixedRoster{},
		Logger:     logging.New(io.Discard, logging.Options{Level: "error"}),
	}, notifications.SchedulerOptions{})

	at := f.doseAt.Add(-14 * time.Minute)

	var wait sync.WaitGroup
	delivered := make([]int, 2)

	for index, scheduler := range []*notifications.Scheduler{f.scheduler, second} {
		wait.Add(1)
		go func() {
			defer wait.Done()
			pass, err := scheduler.RunOnce(ctx, at)
			if err != nil {
				t.Errorf("worker %d: %v", index, err)
				return
			}
			delivered[index] = pass.Delivered
		}()
	}
	wait.Wait()

	if total := delivered[0] + delivered[1]; total != 1 {
		t.Errorf("the two workers delivered %d notifications between them, want 1", total)
	}
	if sender.count() != 1 {
		t.Errorf("the sender was called %d times, want exactly once", sender.count())
	}
}

func TestTwoSchedulersMaterialiseTheSameNotificationOnce(t *testing.T) {
	t.Parallel()

	f := newSchedulerFixture(t, &countingSender{})

	// The same decision, reached independently four times at once. The unique
	// index on the dedupe key is what makes three of them no-ops.
	ctx := context.Background()
	at := f.doseAt.Add(-16 * time.Minute)

	var wait sync.WaitGroup
	created := make([]int, 4)

	for index := range created {
		wait.Add(1)
		go func() {
			defer wait.Done()
			pass, err := f.scheduler.RunOnce(ctx, at)
			if err != nil {
				t.Errorf("worker %d: %v", index, err)
				return
			}
			created[index] = pass.Created
		}()
	}
	wait.Wait()

	total := 0
	for _, count := range created {
		total += count
	}
	if total != 1 {
		t.Errorf("four concurrent passes created %d notifications, want 1", total)
	}
	if got := f.countNotifications(t); got != 1 {
		t.Errorf("%d notifications exist, want 1", got)
	}
}

func TestTheSchedulerStartsAndStopsCleanly(t *testing.T) {
	t.Parallel()

	f := newSchedulerFixture(t, &countingSender{})

	ctx := context.Background()
	if err := f.scheduler.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Starting twice is a programming error, not a second loop.
	if err := f.scheduler.Start(ctx); err == nil {
		t.Error("Start succeeded twice; there would be two loops on one database")
	}

	// The loop runs a pass before its first tick, so a restart delivers whatever
	// fell due while the process was down. Waited for rather than assumed:
	// stopping mid-pass cancels its queries, which is correct behaviour and
	// would make this assertion about the race rather than about the loop.
	deadline := time.Now().Add(10 * time.Second)
	for f.countNotifications(t) == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if got := f.countNotifications(t); got != 1 {
		t.Errorf("%d notifications after the first pass, want 1", got)
	}

	stopCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := f.scheduler.Stop(stopCtx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	// Stopping an already-stopped scheduler is harmless, which is what lets
	// shutdown call it without tracking state of its own.
	if err := f.scheduler.Stop(stopCtx); err != nil {
		t.Errorf("second Stop: %v", err)
	}
}

func TestASenderThatBreaksLeavesTheNotificationForTheNextPass(t *testing.T) {
	t.Parallel()

	sender := &countingSender{failWith: context.DeadlineExceeded}
	f := newSchedulerFixture(t, sender)
	f.registerDevice(t, "ExponentPushToken[abc]")

	ctx := context.Background()
	if _, err := f.scheduler.RunOnce(ctx, f.doseAt.Add(-16*time.Minute)); err != nil {
		t.Fatalf("materialise: %v", err)
	}
	if _, err := f.scheduler.RunOnce(ctx, f.doseAt.Add(-14*time.Minute)); err != nil {
		t.Fatalf("deliver: %v", err)
	}

	// Still pending, so nothing is lost — but leased, so nothing hammers the
	// broken sender either.
	status, _, availableAt := f.state(t)
	if status != "pending" {
		t.Errorf("delivery_status = %q, want it left pending", status)
	}
	if !availableAt.After(f.doseAt.Add(-14 * time.Minute)) {
		t.Errorf("available_at = %s, want the lease to hold it back", availableAt)
	}
}

func TestOldNotificationsArePurged(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.account("daughter", "daughter@example.com")

	seniorID := h.createCircle(t, "daughter", "Amma", "Asia/Kolkata")
	daughter := h.signIn(t, "daughter")

	h.seed(t, daughter, seniorID, "MEDICATION_REMINDER", time.Now().Add(-40*24*time.Hour))
	recent := h.seed(t, daughter, seniorID, "MEDICATION_REMINDER", time.Now().Add(-time.Hour))

	repository := notifications.NewRepository(h.pool)
	purged, err := repository.PurgeBefore(context.Background(), time.Now().Add(-30*24*time.Hour))
	if err != nil {
		t.Fatalf("PurgeBefore: %v", err)
	}
	if purged != 1 {
		t.Errorf("purged %d, want the one beyond retention", purged)
	}

	items := inboxItems(t, h.inbox(t, "daughter", ""))
	if len(items) != 1 || items[0]["id"] != recent.String() {
		t.Errorf("inbox = %v, want only the recent notification", items)
	}
}
