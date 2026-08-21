package notifications

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/meracare/api/internal/paging"
)

// ErrBadCursor is returned when a page cursor cannot be read. The same sentinel
// every other paged history in the API uses, so the handler answers 400 the
// same way (plans/phase11.md §41).
var ErrBadCursor = paging.ErrBadCursor

const notificationColumns = `id, recipient_user_id, senior_id, notification_type,
	title, body, entity_type, entity_id, scheduled_for, delivery_status, attempts,
	available_at, delivered_at, last_error, read_at, created_at, updated_at`

// AllPreferences loads every stored preference row, keyed by user.
//
// One query rather than one per member: a scheduler pass consults the
// preferences of everybody with an active membership, and asking the database
// separately for each would make the sweep's cost a multiple of the circle
// count for data that fits in memory many times over. Users with no row are
// absent and take the defaults (plans/phase11.md §71).
func (r *Repository) AllPreferences(ctx context.Context) (map[uuid.UUID]Preferences, error) {
	rows, err := r.db.Query(ctx, `SELECT `+preferenceColumns+` FROM notification_preferences`)
	if err != nil {
		return nil, fmt.Errorf("list notification preferences: %w", err)
	}
	defer rows.Close()

	found := make(map[uuid.UUID]Preferences)
	for rows.Next() {
		preferences, err := scanPreferences(rows)
		if err != nil {
			return nil, fmt.Errorf("read notification preferences: %w", err)
		}
		found[preferences.UserID] = preferences
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read notification preferences: %w", err)
	}
	return found, nil
}

// Insert writes notifications that do not already exist, and reports how many
// were new.
//
// ON CONFLICT DO NOTHING on the dedupe key is the entire deduplication
// mechanism, and it is the database's rather than the scheduler's on purpose: a
// check-then-insert in application code is correct until two processes run it
// at the same moment, which is exactly the situation a second API instance
// creates (plans/phase11.md §§20, 37).
func (r *Repository) Insert(ctx context.Context, items []pending) (int, error) {
	created := 0

	for _, item := range items {
		tag, err := r.db.Exec(ctx, `
			INSERT INTO notifications (
				recipient_user_id, senior_id, notification_type, title, body,
				entity_type, entity_id, scheduled_for, dedupe_key, available_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $8)
			ON CONFLICT (dedupe_key) DO NOTHING`,
			item.RecipientUserID,
			item.SeniorID,
			string(item.Type),
			item.Title,
			item.Body,
			string(item.EntityType),
			item.EntityID,
			item.ScheduledFor,
			item.DedupeKey,
		)
		if err != nil {
			return created, fmt.Errorf("insert notification: %w", err)
		}
		created += int(tag.RowsAffected())
	}
	return created, nil
}

// Page is one page of somebody's inbox, newest first.
type Page struct {
	Items []Notification
	// NextCursor is empty when the inbox is exhausted.
	NextCursor string
	// Unread is the total count of unread notifications, not the count on this
	// page. It is what the badge shows, and it is returned with the page so the
	// badge cannot drift from the list it labels (plans/phase11.md §61).
	Unread int
}

// List returns one page of a user's notifications, newest first.
//
// Scoped by recipient in the statement itself. There is no code path that takes
// a user id from anywhere but the verified token, so "a user cannot read
// somebody else's notifications" is a property of the shape of the query rather
// than a check that could be forgotten (plans/phase11.md §8).
//
// Notifications scheduled for the future are excluded. The scheduler
// materialises up to an hour ahead so delivery does not depend on a sweep
// landing on the right second, and an inbox that showed tomorrow's reminder
// today would be showing a decision, not a notification.
func (r *Repository) List(
	ctx context.Context,
	userID uuid.UUID,
	cursor string,
	limit int,
	now time.Time,
) (Page, error) {
	at, atID, err := paging.DecodeCursor(cursor)
	if err != nil {
		return Page{}, err
	}

	// One more than asked for, so the presence of a next page is known without
	// a second count query.
	rows, err := r.db.Query(ctx, `
		SELECT `+notificationColumns+`
		  FROM notifications
		 WHERE recipient_user_id = $1
		   AND scheduled_for <= $2
		   AND ($3::timestamptz IS NULL OR (scheduled_for, id) < ($3, $4))
		 ORDER BY scheduled_for DESC, id DESC
		 LIMIT $5`,
		userID, now, at, atID, limit+1)
	if err != nil {
		return Page{}, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()

	found := make([]Notification, 0, limit+1)
	for rows.Next() {
		notification, err := scanNotification(rows)
		if err != nil {
			return Page{}, fmt.Errorf("read notifications: %w", err)
		}
		found = append(found, notification)
	}
	if err := rows.Err(); err != nil {
		return Page{}, fmt.Errorf("read notifications: %w", err)
	}

	unread, err := r.UnreadCount(ctx, userID, now)
	if err != nil {
		return Page{}, err
	}

	page := Page{Items: found, Unread: unread}
	if len(found) > limit {
		last := found[limit-1]
		page.Items = found[:limit]
		page.NextCursor = paging.EncodeCursor(last.ScheduledFor, last.ID)
	}
	return page, nil
}

// UnreadCount returns how many of the user's arrived notifications are unread.
func (r *Repository) UnreadCount(ctx context.Context, userID uuid.UUID, now time.Time) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, `
		SELECT count(*) FROM notifications
		 WHERE recipient_user_id = $1 AND read_at IS NULL AND scheduled_for <= $2`,
		userID, now).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count unread notifications: %w", err)
	}
	return count, nil
}

// MarkRead marks one of the caller's notifications as read.
//
// Idempotent: an already-read notification keeps its original read time rather
// than being restamped, so "when did they see it?" stays answerable after a
// list screen re-marks something on a second visit.
func (r *Repository) MarkRead(
	ctx context.Context,
	userID, notificationID uuid.UUID,
) (Notification, error) {
	notification, err := scanNotification(r.db.QueryRow(ctx, `
		UPDATE notifications
		   SET read_at = COALESCE(read_at, now()), updated_at = now()
		 WHERE id = $1 AND recipient_user_id = $2
		RETURNING `+notificationColumns,
		notificationID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		// Somebody else's notification and a notification that does not exist
		// get the same answer, so an id cannot be probed (plans/phase11.md §8).
		return Notification{}, ErrUnknownNotification
	}
	if err != nil {
		return Notification{}, fmt.Errorf("mark notification read: %w", err)
	}
	return notification, nil
}

// MarkAllRead marks every arrived notification of the caller's as read, and
// reports how many changed.
//
// One statement, so it is atomic without a transaction and cannot half-apply
// while a page is being read (plans/phase11.md §29).
func (r *Repository) MarkAllRead(ctx context.Context, userID uuid.UUID, now time.Time) (int, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE notifications
		   SET read_at = now(), updated_at = now()
		 WHERE recipient_user_id = $1 AND read_at IS NULL AND scheduled_for <= $2`,
		userID, now)
	if err != nil {
		return 0, fmt.Errorf("mark all notifications read: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// ClaimDue takes ownership of up to limit notifications that are ready to send.
//
// The claim and the read are one statement. `FOR UPDATE SKIP LOCKED` is what
// lets a second worker take different rows instead of waiting for the first,
// and pushing `available_at` forward by the lease is what stops it taking the
// same rows a moment later — so two API instances sweeping simultaneously
// deliver each notification exactly once, with no in-memory lock and nothing
// that has to know how many instances exist (plans/phase11.md §37).
//
// A worker that dies mid-flight loses its lease rather than its work: the rows
// stay pending, and the next sweep after the lease expires picks them up.
func (r *Repository) ClaimDue(
	ctx context.Context,
	now time.Time,
	lease time.Duration,
	limit int,
) ([]Notification, error) {
	rows, err := r.db.Query(ctx, `
		UPDATE notifications
		   SET attempts = attempts + 1, available_at = $2, updated_at = now()
		 WHERE id IN (
			SELECT id FROM notifications
			 WHERE delivery_status = 'pending'
			   AND scheduled_for <= $1
			   AND available_at <= $1
			 ORDER BY scheduled_for
			 LIMIT $3
			 FOR UPDATE SKIP LOCKED
		 )
		RETURNING `+notificationColumns,
		now, now.Add(lease), limit)
	if err != nil {
		return nil, fmt.Errorf("claim notifications: %w", err)
	}
	defer rows.Close()

	claimed := make([]Notification, 0, limit)
	for rows.Next() {
		notification, err := scanNotification(rows)
		if err != nil {
			return nil, fmt.Errorf("read claimed notifications: %w", err)
		}
		claimed = append(claimed, notification)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read claimed notifications: %w", err)
	}
	return claimed, nil
}

// Settle records the outcome of a delivery attempt.
//
// A terminal status clears the backoff; `pending` with a later availableAt is
// how a retry is expressed. The error text is the provider's own message, never
// a token (plans/phase11.md §24).
func (r *Repository) Settle(
	ctx context.Context,
	notificationID uuid.UUID,
	status DeliveryStatus,
	availableAt time.Time,
	failure string,
) error {
	_, err := r.db.Exec(ctx, `
		UPDATE notifications
		   SET delivery_status = $2,
		       available_at    = $3,
		       delivered_at    = CASE WHEN $2 = 'sent' THEN now() ELSE delivered_at END,
		       last_error      = $4,
		       updated_at      = now()
		 WHERE id = $1`,
		notificationID, string(status), availableAt, failure)
	if err != nil {
		return fmt.Errorf("settle notification: %w", err)
	}
	return nil
}

// ActiveDevicesFor returns the reachable devices of each of the given users.
//
// Only devices with a token: a registration with none is an install that has
// not been given notification permission, which is a device we know about and
// cannot reach (plans/phase8.md §9).
func (r *Repository) ActiveDevicesFor(
	ctx context.Context,
	userIDs []uuid.UUID,
) (map[uuid.UUID][]Device, error) {
	found := make(map[uuid.UUID][]Device)
	if len(userIDs) == 0 {
		return found, nil
	}

	rows, err := r.db.Query(ctx, `
		SELECT `+deviceColumns+`
		  FROM notification_devices
		 WHERE user_id = ANY($1) AND active AND push_token IS NOT NULL`,
		userIDs)
	if err != nil {
		return nil, fmt.Errorf("list push devices: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		device, err := scanDevice(rows)
		if err != nil {
			return nil, fmt.Errorf("read push devices: %w", err)
		}
		found[device.UserID] = append(found[device.UserID], device)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read push devices: %w", err)
	}
	return found, nil
}

// DeactivateToken retires a token the push provider has rejected.
//
// Matched on the token rather than on a device id because that is all the
// provider tells us. The row survives with its token cleared, so the same
// install signing in again is still an update rather than a duplicate
// (plans/phase11.md §39).
func (r *Repository) DeactivateToken(ctx context.Context, token string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE notification_devices
		   SET active = false, push_token = NULL, updated_at = now()
		 WHERE push_token = $1`, token)
	if err != nil {
		return fmt.Errorf("deactivate push token: %w", err)
	}
	return nil
}

// PurgeBefore deletes notifications older than the cutoff and reports how many
// went.
//
// Retention rather than an archive: a notification is a copy of something the
// care record already holds — the dose, the task, the care event all remain —
// so keeping the copy forever grows a table without preserving anything
// (plans/phase11.md §32).
func (r *Repository) PurgeBefore(ctx context.Context, cutoff time.Time) (int, error) {
	tag, err := r.db.Exec(ctx, `DELETE FROM notifications WHERE scheduled_for < $1`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("purge notifications: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func scanNotification(source row) (Notification, error) {
	var (
		notification   Notification
		notifType      string
		entityType     string
		deliveryStatus string
	)

	err := source.Scan(
		&notification.ID,
		&notification.RecipientUserID,
		&notification.SeniorID,
		&notifType,
		&notification.Title,
		&notification.Body,
		&entityType,
		&notification.EntityID,
		&notification.ScheduledFor,
		&deliveryStatus,
		&notification.Attempts,
		&notification.AvailableAt,
		&notification.DeliveredAt,
		&notification.LastError,
		&notification.ReadAt,
		&notification.CreatedAt,
		&notification.UpdatedAt,
	)
	if err != nil {
		return Notification{}, err
	}

	notification.Type = Type(notifType)
	notification.EntityType = EntityType(entityType)
	notification.DeliveryStatus = DeliveryStatus(deliveryStatus)
	return notification, nil
}
