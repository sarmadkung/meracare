// Package notifications decides what MeraCare is allowed to remind each user
// about, and when.
//
// It is infrastructure, not care. Tasks, medication, and appointments remain
// the only source of truth for what is due; this package reads their schedules
// and answers a narrower question — "which of those should this particular
// person's phone tell them about, and at what moment?" It creates no care data
// of its own and can be deleted without losing any (plans/phase8.md §1).
package notifications

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
)

// ReminderType is the kind of thing a reminder is about.
//
// These are the three categories docs/08-notifications-and-background.md
// assigns to scheduled local reminders, and the three plans/phase8.md §2
// requires. There is deliberately no fourth: an event that needs telling
// somebody about right now is a push notification, which is a different
// mechanism and a later phase.
type ReminderType string

const (
	// ReminderTaskReminder is a care task falling due.
	ReminderTaskReminder ReminderType = "TASK_REMINDER"
	// ReminderMedicationReminder is a dose falling due.
	ReminderMedicationReminder ReminderType = "MEDICATION_REMINDER"
	// ReminderAppointmentReminder is an appointment starting soon.
	ReminderAppointmentReminder ReminderType = "APPOINTMENT_REMINDER"
)

// ReminderTypes lists every reminder type. The mobile client's vocabulary in
// packages/contracts/src/notification.ts must match this exactly.
var ReminderTypes = []ReminderType{
	ReminderTaskReminder,
	ReminderMedicationReminder,
	ReminderAppointmentReminder,
}

// Valid reports whether the type is recognised.
func (t ReminderType) Valid() bool { return slices.Contains(ReminderTypes, t) }

// leadTime is how long before the thing itself a reminder fires.
//
// Fixed, and not configurable. Nothing in the documentation defines reminder
// offsets, and plans/phase8.md §12 forbids inventing options beyond the
// documented MVP — so these are exactly the three worked examples the brief
// itself gives: a dose at 08:00 reminds at 07:45 (§12), a task due 09:00
// reminds at 08:45 (§13), an appointment at 14:00 reminds at 13:00 (§14).
//
// The asymmetry is real rather than an oversight. Fifteen minutes is enough
// warning to walk to the kitchen; an appointment needs enough warning to leave
// the house.
func (t ReminderType) leadTime() time.Duration {
	switch t {
	case ReminderAppointmentReminder:
		return time.Hour
	default:
		return 15 * time.Minute
	}
}

// EntityType is what a reminder points at, so the app can open the right
// screen. The names match the domains' own vocabulary.
type EntityType string

const (
	// EntityTaskInstance is one occurrence of a care task.
	EntityTaskInstance EntityType = "task_instance"
	// EntityMedicationDose is one scheduled dose.
	EntityMedicationDose EntityType = "medication_dose"
	// EntityAppointment is one appointment.
	EntityAppointment EntityType = "appointment"
)

// entityFor is the subject each reminder type is about. One reminder type has
// exactly one entity type, so the client never has to handle a combination
// that cannot occur.
func (t ReminderType) entityFor() EntityType {
	switch t {
	case ReminderTaskReminder:
		return EntityTaskInstance
	case ReminderMedicationReminder:
		return EntityMedicationDose
	default:
		return EntityAppointment
	}
}

// Platform is the operating system a registered device runs.
type Platform string

const (
	// PlatformIOS is an iPhone or iPad.
	PlatformIOS Platform = "ios"
	// PlatformAndroid is an Android phone or tablet.
	PlatformAndroid Platform = "android"
	// PlatformWeb is the browser build. It registers like any other device;
	// whether a browser can be pushed to is the push phase's problem.
	PlatformWeb Platform = "web"
)

// Platforms lists every recognised platform. The database CHECK on
// notification_devices.platform must match.
var Platforms = []Platform{PlatformIOS, PlatformAndroid, PlatformWeb}

// Valid reports whether the platform is recognised.
func (p Platform) Valid() bool { return slices.Contains(Platforms, p) }

// Preferences are one user's notification settings.
//
// Held per user rather than per senior: the person receiving the notification
// is the person who decides whether they want it (plans/phase8.md §§3, 4). Two
// caregivers in the same circle can and routinely will differ.
type Preferences struct {
	UserID uuid.UUID

	TaskReminders        bool
	MedicationReminders  bool
	AppointmentReminders bool
	// OverdueTaskAlerts and CareActivity are Phase 11's two additions. 0008
	// refused to store switches for categories nothing could deliver; these two
	// now have a delivery path, so they exist (plans/phase11.md §9).
	OverdueTaskAlerts bool
	CareActivity      bool

	CreatedAt time.Time
	UpdatedAt time.Time
}

// DefaultPreferences is what a user has before they have ever opened the
// settings screen. Every category is on: reminders are the reason the app
// exists, and silence by default would look like a broken app rather than a
// respectful one.
func DefaultPreferences(userID uuid.UUID) Preferences {
	return Preferences{
		UserID:               userID,
		TaskReminders:        true,
		MedicationReminders:  true,
		AppointmentReminders: true,
		OverdueTaskAlerts:    true,
		CareActivity:         true,
	}
}

// wants reports whether these preferences allow the given reminder type.
func (p Preferences) wants(t ReminderType) bool {
	return p.wantsType(t.notificationType())
}

// wantsType reports whether these preferences allow the given notification
// type.
//
// The default is false, not true. A category nobody has decided about is a
// category MeraCare has not been given permission to use, and an unrecognised
// type reaching here at all means the vocabulary has grown without the
// preferences following it.
func (p Preferences) wantsType(t Type) bool {
	switch t {
	case TypeTaskReminder:
		return p.TaskReminders
	case TypeMedicationReminder:
		return p.MedicationReminders
	case TypeAppointmentReminder:
		return p.AppointmentReminders
	case TypeTaskOverdue:
		return p.OverdueTaskAlerts
	case TypeCareActivity:
		return p.CareActivity
	default:
		return false
	}
}

// Device is one installation of the app that may be pushed to.
type Device struct {
	ID     uuid.UUID
	UserID uuid.UUID

	// DeviceID is the app's own stable identifier for this installation.
	DeviceID string
	Platform Platform

	// PushToken is a credential for reaching this phone. It is loaded so the
	// push phase can use it, and is never put into any response. Nothing in
	// this package logs it (plans/phase8.md §8).
	PushToken string

	AppVersion string
	Active     bool

	LastSeenAt time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// ErrUnknownDevice is returned when a device id does not belong to the caller.
//
// It carries no information about whether the device exists for somebody else:
// the handler turns it into the same 404 an unknown id gets, so registrations
// cannot be probed (plans/phase8.md §§8, 40).
var ErrUnknownDevice = errors.New("notifications: no such device for this user")

// reminderNamespace seeds the deterministic reminder identifier below. A fixed
// random UUID, generated once; its only requirement is that it never changes,
// because changing it would rename every reminder already scheduled on every
// device and each would be scheduled a second time.
var reminderNamespace = uuid.MustParse("b7f6b7a2-3a5f-4f0e-9a1d-2c8f0a6d4e11")

// Reminder is one notification a device should schedule locally.
//
// It carries no wording. The title and body are composed on the device from
// packages/contracts/src/notification-labels.ts, the same way every other
// user-visible sentence in MeraCare is — one place where the phrasing lives,
// and no way for the server to accidentally put a medicine's name into
// something that appears on a lock screen (plans/phase8.md §§17, 47).
type Reminder struct {
	// ID is derived, not stored, and is the whole idempotency mechanism.
	ID uuid.UUID

	Type ReminderType

	SeniorID   uuid.UUID
	SeniorName string
	// SeniorTimezone is the IANA name the FireAt instant should be read in, so
	// the device shows "08:00" to a caregiver in another country too
	// (plans/phase8.md §32).
	SeniorTimezone string

	EntityType EntityType
	EntityID   uuid.UUID

	// DueAt is when the care itself is due.
	DueAt time.Time
	// FireAt is when the notification should appear: DueAt minus the type's
	// lead time.
	FireAt time.Time
}

// newReminder builds a reminder and derives its identifier.
//
// The identifier is a UUIDv5 over the recipient, the type, the subject, and the
// firing instant. That makes it a pure function of the plan: recomputing the
// same plan yields byte-identical ids, so a device that re-runs the whole
// reconciliation after every launch, every refresh, and every retry schedules
// each reminder exactly once (plans/phase8.md §§25, 26).
//
// FireAt is part of the identity on purpose. Moving a dose from 08:00 to 09:00
// must produce a different reminder, so the old one is cancelled and the new
// one scheduled, rather than a stale 07:45 alert surviving because its subject
// is unchanged (plans/phase8.md §22).
func newReminder(userID uuid.UUID, t ReminderType, senior Senior, entityID uuid.UUID, dueAt time.Time) Reminder {
	fireAt := dueAt.Add(-t.leadTime())

	seed := fmt.Sprintf("%s|%s|%s|%s", userID, t, entityID, fireAt.UTC().Format(time.RFC3339))

	return Reminder{
		ID:             uuid.NewSHA1(reminderNamespace, []byte(seed)),
		Type:           t,
		SeniorID:       senior.ID,
		SeniorName:     senior.DisplayName,
		SeniorTimezone: senior.Timezone,
		EntityType:     t.entityFor(),
		EntityID:       entityID,
		DueAt:          dueAt,
		FireAt:         fireAt,
	}
}
