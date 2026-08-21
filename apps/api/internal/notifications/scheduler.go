package notifications

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Scheduler is the one background process MeraCare runs.
//
// It does three things on a fixed tick — decide which notifications should
// exist, deliver the ones that are due, and forget the ones that are old — and
// nothing else. There is no goroutine per notification, no timer per user, and
// nothing that has to stay alive between ticks: a pass reads the world, acts,
// and returns, so a process that dies mid-pass loses nothing that the next pass
// does not recompute (plans/phase11.md §36).
//
// Its lifecycle is explicit. Start begins the loop, Stop waits for the pass in
// flight to finish, and the API's shutdown blocks on it — so the process does
// not exit with a delivery half-recorded.
type Scheduler struct {
	repository *Repository
	sender     PushSender
	roster     Roster
	reminders  map[Type]ScheduleSource
	overdue    OverdueSource
	activity   ActivitySource

	options SchedulerOptions
	logger  *slog.Logger

	// now is the clock, injected so tests can drive a pass to an exact instant
	// rather than sleeping (plans/phase11.md §63).
	now func() time.Time

	mu      sync.Mutex
	stop    context.CancelFunc
	stopped chan struct{}
}

// SchedulerOptions tunes the loop. Every field has a working default.
type SchedulerOptions struct {
	// Interval is how often a pass runs.
	//
	// A minute. Notifications are scheduled to the minute, materialisation
	// looks an hour ahead, and delivery re-reads the recent past, so a tick that
	// is late by a few minutes costs nothing — but a tick much longer than this
	// would delay the overdue alerts, which are the only type whose whole point
	// is timeliness.
	Interval time.Duration
	// BatchSize is how many notifications one pass claims for delivery.
	BatchSize int
	// Lease is how long a claimed notification is held before another worker may
	// take it. Longer than any plausible push request, so a slow provider does
	// not cause a double send; short enough that a crashed worker's rows are
	// picked up promptly.
	Lease time.Duration
	// MaxAttempts is how many times a delivery is tried before it is abandoned.
	//
	// Three. A push that has failed three times over twenty minutes is not
	// failing for a reason another attempt will fix, and the notification is
	// still in the inbox either way (plans/phase11.md §38).
	MaxAttempts int
	// Retention is how long a notification is kept.
	Retention time.Duration
	// PurgeEvery bounds how often retention is enforced; the delete is cheap but
	// pointless to run every minute.
	PurgeEvery time.Duration
}

func (o SchedulerOptions) withDefaults() SchedulerOptions {
	if o.Interval <= 0 {
		o.Interval = time.Minute
	}
	if o.BatchSize <= 0 {
		o.BatchSize = 100
	}
	if o.Lease <= 0 {
		o.Lease = 2 * time.Minute
	}
	if o.MaxAttempts <= 0 {
		o.MaxAttempts = 3
	}
	if o.Retention <= 0 {
		o.Retention = 30 * 24 * time.Hour
	}
	if o.PurgeEvery <= 0 {
		o.PurgeEvery = time.Hour
	}
	return o
}

// SchedulerDependencies are the collaborators a scheduler needs.
type SchedulerDependencies struct {
	Repository *Repository
	Sender     PushSender
	Roster     Roster
	// Reminders is keyed by notification type, so a domain with no reminder path
	// is an absent entry rather than a nil check at every use.
	Reminders map[Type]ScheduleSource
	Overdue   OverdueSource
	Activity  ActivitySource
	Logger    *slog.Logger
	// Now is the clock. Nil means time.Now.
	Now func() time.Time
}

// NewScheduler builds a scheduler. It does not start it.
func NewScheduler(deps SchedulerDependencies, options SchedulerOptions) *Scheduler {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	clock := deps.Now
	if clock == nil {
		clock = time.Now
	}
	sender := deps.Sender
	if sender == nil {
		sender = DisabledSender{}
	}

	return &Scheduler{
		repository: deps.Repository,
		sender:     sender,
		roster:     deps.Roster,
		reminders:  deps.Reminders,
		overdue:    deps.Overdue,
		activity:   deps.Activity,
		options:    options.withDefaults(),
		logger:     logger,
		now:        clock,
	}
}

// ErrAlreadyRunning is returned by Start when the scheduler is already going.
var ErrAlreadyRunning = errors.New("notifications: scheduler is already running")

// Start begins the loop. It returns immediately.
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stop != nil {
		return ErrAlreadyRunning
	}

	loopCtx, cancel := context.WithCancel(ctx)
	s.stop = cancel
	s.stopped = make(chan struct{})

	go s.loop(loopCtx, s.stopped)

	s.logger.Info("notification scheduler started",
		slog.Duration("interval", s.options.Interval))
	return nil
}

// Stop ends the loop and waits for the pass in flight to finish, or for ctx to
// be done.
//
// Waiting is the point. A pass that is interrupted between claiming a
// notification and recording what happened to it leaves the row leased, and the
// notification is then delayed by the lease rather than lost — acceptable on a
// crash, but not something to do on every deploy.
func (s *Scheduler) Stop(ctx context.Context) error {
	s.mu.Lock()
	stop, stopped := s.stop, s.stopped
	s.stop, s.stopped = nil, nil
	s.mu.Unlock()

	if stop == nil {
		return nil
	}
	stop()

	select {
	case <-stopped:
		s.logger.Info("notification scheduler stopped")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Scheduler) loop(ctx context.Context, stopped chan struct{}) {
	defer close(stopped)

	ticker := time.NewTicker(s.options.Interval)
	defer ticker.Stop()

	lastPurge := time.Time{}

	for {
		// The pass runs first, before the first tick, so a restart delivers
		// whatever fell due while the process was down instead of waiting a
		// minute to notice.
		pass, err := s.RunOnce(ctx, s.now())
		if err != nil && !errors.Is(err, context.Canceled) {
			s.logger.Error("notification pass failed", slog.Any("error", err))
		} else if pass.worthLogging() {
			s.logger.Info("notification pass",
				slog.Int("created", pass.Created),
				slog.Int("delivered", pass.Delivered),
				slog.Int("skipped", pass.Skipped),
				slog.Int("failed", pass.Failed),
				slog.Int("retrying", pass.Retrying),
			)
		}

		if now := s.now(); now.Sub(lastPurge) >= s.options.PurgeEvery {
			lastPurge = now
			if purged, err := s.repository.PurgeBefore(ctx, now.Add(-s.options.Retention)); err != nil {
				if !errors.Is(err, context.Canceled) {
					s.logger.Error("notification purge failed", slog.Any("error", err))
				}
			} else if purged > 0 {
				s.logger.Info("notifications purged", slog.Int("count", purged))
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Pass is what one run of the scheduler did.
type Pass struct {
	Created   int
	Delivered int
	Skipped   int
	Failed    int
	Retrying  int
}

func (p Pass) worthLogging() bool {
	return p.Created+p.Delivered+p.Skipped+p.Failed+p.Retrying > 0
}

// RunOnce performs a single pass: decide, then deliver.
//
// Exported so tests can drive it at an exact instant, and so an operator can
// reason about the loop as "this, on a timer" rather than as a background
// process with a life of its own.
func (s *Scheduler) RunOnce(ctx context.Context, now time.Time) (Pass, error) {
	created, err := s.materialise(ctx, now)
	if err != nil {
		return Pass{}, err
	}

	pass, err := s.deliver(ctx, now)
	pass.Created = created
	return pass, err
}

// materialise decides which notifications should exist and writes the ones that
// do not yet.
func (s *Scheduler) materialise(ctx context.Context, now time.Time) (int, error) {
	if s.roster == nil {
		return 0, nil
	}

	memberships, err := s.roster.ActiveMemberships(ctx)
	if err != nil {
		return 0, err
	}
	if len(memberships) == 0 {
		return 0, nil
	}

	preferences, err := s.repository.AllPreferences(ctx)
	if err != nil {
		return 0, err
	}

	decided, err := materialise(ctx, materialiseInput{
		Memberships: memberships,
		Preferences: preferences,
		Reminders:   s.reminders,
		Overdue:     s.overdue,
		Activity:    s.activity,
		Now:         now,
	})
	if err != nil {
		return 0, err
	}

	return s.repository.Insert(ctx, decided)
}

// deliver claims what is due and pushes it.
func (s *Scheduler) deliver(ctx context.Context, now time.Time) (Pass, error) {
	var pass Pass

	claimed, err := s.repository.ClaimDue(ctx, now, s.options.Lease, s.options.BatchSize)
	if err != nil {
		return pass, err
	}
	if len(claimed) == 0 {
		return pass, nil
	}

	recipients := make([]uuid.UUID, 0, len(claimed))
	seen := make(map[uuid.UUID]struct{}, len(claimed))
	for _, notification := range claimed {
		if _, ok := seen[notification.RecipientUserID]; ok {
			continue
		}
		seen[notification.RecipientUserID] = struct{}{}
		recipients = append(recipients, notification.RecipientUserID)
	}

	devices, err := s.repository.ActiveDevicesFor(ctx, recipients)
	if err != nil {
		return pass, err
	}

	for _, notification := range claimed {
		outcome, err := s.deliverOne(ctx, notification, devices[notification.RecipientUserID], now)
		if err != nil {
			return pass, err
		}

		switch outcome {
		case DeliverySent:
			pass.Delivered++
		case DeliverySkipped:
			pass.Skipped++
		case DeliveryFailed:
			pass.Failed++
		default:
			pass.Retrying++
		}
	}
	return pass, nil
}

// deliverOne pushes one notification to every device its recipient has, and
// records the outcome.
//
// One accepted device is success. A person with a phone and a tablet has been
// told, and marking the notification failed because the tablet's token had
// expired would send it again to the phone that already showed it.
func (s *Scheduler) deliverOne(
	ctx context.Context,
	notification Notification,
	devices []Device,
	now time.Time,
) (DeliveryStatus, error) {
	if len(devices) == 0 {
		// Nowhere to send it. The notification is in the inbox, which is where
		// it is read today; retrying would only ask the same question again
		// (plans/phase11.md §43).
		return DeliverySkipped, s.repository.Settle(
			ctx, notification.ID, DeliverySkipped, now, "no reachable device")
	}

	messages := make([]PushMessage, 0, len(devices))
	for _, device := range devices {
		messages = append(messages, PushMessage{
			Token: device.PushToken,
			Title: notification.Title,
			Body:  notification.Body,
			Data:  payloadFor(notification),
		})
	}

	outcomes, err := s.sender.Send(ctx, messages)
	if err != nil {
		// The sender itself broke, which is a bug rather than a delivery
		// failure. Leave the row leased so the next pass retries it.
		return DeliveryPending, nil
	}

	delivered, retryable := false, false
	failure := ""

	for _, outcome := range outcomes {
		switch {
		case outcome.Delivered:
			delivered = true
		case outcome.TokenInvalid:
			// The install is gone. Retire the token rather than asking again
			// every minute for the next month (plans/phase11.md §39).
			if err := s.repository.DeactivateToken(ctx, outcome.Token); err != nil {
				return DeliveryPending, err
			}
			failure = orFirst(failure, outcome.Error)
		case outcome.Retryable:
			retryable = true
			failure = orFirst(failure, outcome.Error)
		default:
			failure = orFirst(failure, outcome.Error)
		}
	}

	switch {
	case delivered:
		return DeliverySent, s.repository.Settle(ctx, notification.ID, DeliverySent, now, "")

	case retryable && notification.Attempts < s.options.MaxAttempts:
		next := now.Add(backoff(notification.Attempts))
		return DeliveryPending, s.repository.Settle(
			ctx, notification.ID, DeliveryPending, next, failure)

	case retryable:
		return DeliveryFailed, s.repository.Settle(
			ctx, notification.ID, DeliveryFailed, now, failure)

	default:
		// Every device refused permanently — an invalid token, or a provider
		// rejection. Another attempt would be refused the same way.
		return DeliveryFailed, s.repository.Settle(
			ctx, notification.ID, DeliveryFailed, now, failure)
	}
}

// backoff spaces out retries. Deliberately conservative: a push that failed is
// not an emergency, and a tight retry loop against a struggling provider makes
// it worse (plans/phase11.md §38).
func backoff(attempts int) time.Duration {
	switch attempts {
	case 0, 1:
		return time.Minute
	case 2:
		return 5 * time.Minute
	default:
		return 15 * time.Minute
	}
}

// payloadFor is the data a push carries: identifiers only.
//
// Enough to open the right screen, and nothing that could be shown without
// asking the server first. A push payload is not a credential and is never
// treated as one (plans/phase11.md §58).
func payloadFor(notification Notification) map[string]string {
	return map[string]string{
		"notificationId": notification.ID.String(),
		"type":           string(notification.Type),
		"seniorId":       notification.SeniorID.String(),
		"entityType":     string(notification.EntityType),
		"entityId":       notification.EntityID.String(),
	}
}

func orFirst(existing, candidate string) string {
	if existing != "" || candidate == "" {
		return existing
	}
	return candidate
}
