package notifications

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/meracare/api/internal/database"
)

// Repository reads and writes notification preferences and device
// registrations.
//
// It stores no reminders. See the header of 0008_notifications.sql for why the
// plan is computed rather than kept.
type Repository struct {
	db database.Querier
}

// NewRepository builds a Repository over the shared pool.
func NewRepository(pool *database.Pool) *Repository {
	return &Repository{db: pool}
}

const preferenceColumns = `user_id, task_reminders, medication_reminders,
	appointment_reminders, overdue_task_alerts, care_activity, created_at, updated_at`

// GetPreferences returns the user's settings, or the defaults if they have
// never changed any.
//
// A missing row is not an error and does not become one: writing a row of
// defaults the first time somebody signs in would put a notification write on
// the authentication path, and the answer would be identical
// (plans/phase8.md §41, "defaults are correct").
func (r *Repository) GetPreferences(ctx context.Context, userID uuid.UUID) (Preferences, error) {
	preferences, err := scanPreferences(r.db.QueryRow(ctx,
		`SELECT `+preferenceColumns+` FROM notification_preferences WHERE user_id = $1`, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return DefaultPreferences(userID), nil
	}
	if err != nil {
		return Preferences{}, fmt.Errorf("get notification preferences: %w", err)
	}
	return preferences, nil
}

// PreferenceUpdate is a change to a user's settings. A nil field is left as it
// is, so a client can turn one category off without having to send — and
// possibly stale-overwrite — the others.
type PreferenceUpdate struct {
	TaskReminders        *bool
	MedicationReminders  *bool
	AppointmentReminders *bool
	OverdueTaskAlerts    *bool
	CareActivity         *bool
}

// SavePreferences applies an update, creating the row if this is the first one.
//
// The upsert reads the defaults for anything the client did not send, so a
// first-time update of one category does not silently switch the other two off.
func (r *Repository) SavePreferences(
	ctx context.Context,
	userID uuid.UUID,
	update PreferenceUpdate,
) (Preferences, error) {
	defaults := DefaultPreferences(userID)

	preferences, err := scanPreferences(r.db.QueryRow(ctx, `
		INSERT INTO notification_preferences
			(user_id, task_reminders, medication_reminders, appointment_reminders,
			 overdue_task_alerts, care_activity)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id) DO UPDATE SET
			task_reminders        = COALESCE($7, notification_preferences.task_reminders),
			medication_reminders  = COALESCE($8, notification_preferences.medication_reminders),
			appointment_reminders = COALESCE($9, notification_preferences.appointment_reminders),
			overdue_task_alerts   = COALESCE($10, notification_preferences.overdue_task_alerts),
			care_activity         = COALESCE($11, notification_preferences.care_activity),
			updated_at            = now()
		RETURNING `+preferenceColumns,
		userID,
		orDefault(update.TaskReminders, defaults.TaskReminders),
		orDefault(update.MedicationReminders, defaults.MedicationReminders),
		orDefault(update.AppointmentReminders, defaults.AppointmentReminders),
		orDefault(update.OverdueTaskAlerts, defaults.OverdueTaskAlerts),
		orDefault(update.CareActivity, defaults.CareActivity),
		update.TaskReminders,
		update.MedicationReminders,
		update.AppointmentReminders,
		update.OverdueTaskAlerts,
		update.CareActivity,
	))
	if err != nil {
		return Preferences{}, fmt.Errorf("save notification preferences: %w", err)
	}
	return preferences, nil
}

// orDefault resolves an optional flag for the insert half of the upsert.
func orDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

const deviceColumns = `id, user_id, device_id, platform, push_token, app_version,
	active, last_seen_at, created_at, updated_at`

// RegisterParams describes one device announcing itself.
type RegisterParams struct {
	UserID   uuid.UUID
	DeviceID string
	Platform Platform
	// PushToken is empty when the app has no token yet — the user has not been
	// asked for permission, or refused. Registration still happens.
	PushToken  string
	AppVersion string
}

// Register records or refreshes one installation.
//
// An upsert on (user_id, device_id) rather than an insert, so an app that
// registers on every launch — which it should, since tokens rotate — keeps one
// row for the life of the install. Re-registering also reactivates a device
// that had signed out, which is precisely what signing back in means
// (plans/phase8.md §§25, 27).
func (r *Repository) Register(ctx context.Context, params RegisterParams) (Device, error) {
	device, err := scanDevice(r.db.QueryRow(ctx, `
		INSERT INTO notification_devices
			(user_id, device_id, platform, push_token, app_version)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5)
		ON CONFLICT (user_id, device_id) DO UPDATE SET
			platform     = EXCLUDED.platform,
			-- Keep the known token when this registration carries none, so a
			-- launch before the permission prompt does not erase a working one.
			push_token   = COALESCE(EXCLUDED.push_token, notification_devices.push_token),
			app_version  = EXCLUDED.app_version,
			active       = true,
			last_seen_at = now(),
			updated_at   = now()
		RETURNING `+deviceColumns,
		params.UserID,
		strings.TrimSpace(params.DeviceID),
		params.Platform,
		strings.TrimSpace(params.PushToken),
		strings.TrimSpace(params.AppVersion),
	))
	if err != nil {
		return Device{}, fmt.Errorf("register notification device: %w", err)
	}
	return device, nil
}

// Deactivate marks one of the user's devices as no longer reachable.
//
// The row is updated rather than deleted: the same installation will register
// again when somebody signs back in, and keeping the row is what makes that an
// update instead of a new registration (plans/phase8.md §9).
//
// Scoped by user_id in the statement itself, so a device id belonging to
// somebody else matches nothing and is reported as unknown — the same answer an
// invented id gets (plans/phase8.md §40).
func (r *Repository) Deactivate(ctx context.Context, userID uuid.UUID, deviceID string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE notification_devices
		   SET active = false, push_token = NULL, updated_at = now()
		 WHERE user_id = $1 AND device_id = $2`,
		userID, strings.TrimSpace(deviceID))
	if err != nil {
		return fmt.Errorf("deactivate notification device: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrUnknownDevice
	}
	return nil
}

// row is the part of pgx.Row and pgx.Rows the scanners need.
type row interface {
	Scan(dest ...any) error
}

func scanPreferences(source row) (Preferences, error) {
	var preferences Preferences
	err := source.Scan(
		&preferences.UserID,
		&preferences.TaskReminders,
		&preferences.MedicationReminders,
		&preferences.AppointmentReminders,
		&preferences.OverdueTaskAlerts,
		&preferences.CareActivity,
		&preferences.CreatedAt,
		&preferences.UpdatedAt,
	)
	return preferences, err
}

func scanDevice(source row) (Device, error) {
	var (
		device    Device
		pushToken *string
		lastSeen  time.Time
	)

	err := source.Scan(
		&device.ID,
		&device.UserID,
		&device.DeviceID,
		&device.Platform,
		&pushToken,
		&device.AppVersion,
		&device.Active,
		&lastSeen,
		&device.CreatedAt,
		&device.UpdatedAt,
	)
	if err != nil {
		return Device{}, err
	}

	if pushToken != nil {
		device.PushToken = *pushToken
	}
	device.LastSeenAt = lastSeen
	return device, nil
}
