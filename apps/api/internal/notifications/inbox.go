package notifications

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
)

// Type is what a delivered notification is about.
//
// A superset of ReminderType: the three reminder categories a device can
// schedule for itself, plus the two that only a server can know about — a task
// that has gone past its time, and something a *different* person did. Both of
// those are facts about a moment that has already passed, which is precisely
// what a device sitting in a pocket cannot compute (plans/phase11.md §5).
//
// The CHECK constraint on notifications.notification_type mirrors this list,
// and so does NOTIFICATION_TYPES in packages/contracts/src/notification.ts.
type Type string

const (
	// TypeMedicationReminder is a dose falling due.
	TypeMedicationReminder Type = "MEDICATION_REMINDER"
	// TypeAppointmentReminder is an appointment starting soon.
	TypeAppointmentReminder Type = "APPOINTMENT_REMINDER"
	// TypeTaskReminder is a care task falling due.
	TypeTaskReminder Type = "TASK_REMINDER"
	// TypeTaskOverdue is a care task whose time has passed with nothing recorded.
	TypeTaskOverdue Type = "TASK_OVERDUE"
	// TypeCareActivity is something somebody else did.
	TypeCareActivity Type = "CARE_ACTIVITY"
)

// Types lists every notification type. There is deliberately none for a missed
// dose: missed is derived from the clock and never stored, and a notification
// type for it would mean inventing the sweep that writes it down — which
// plans/phase4.md §8, plans/phase5.md §8, and plans/phase11.md §18 all refuse
// (see careevents.NotYetEmitted, which names the event this would need).
var Types = []Type{
	TypeMedicationReminder,
	TypeAppointmentReminder,
	TypeTaskReminder,
	TypeTaskOverdue,
	TypeCareActivity,
}

// Valid reports whether the type is recognised.
func (t Type) Valid() bool { return slices.Contains(Types, t) }

// notificationType is the delivered-notification name for a reminder category.
// The two vocabularies share their three overlapping names on purpose: a
// medication reminder is one thing whether the device scheduled it or the
// server pushed it, and a client that had to map between two spellings would
// eventually map one of them wrong.
func (t ReminderType) notificationType() Type { return Type(t) }

// EntityCareEvent is what a care-activity notification points at. The three
// reminder entities are declared in notification.go.
const EntityCareEvent EntityType = "care_event"

// EntityTypes lists every entity a notification can point at. The CHECK
// constraint on notifications.entity_type mirrors it.
var EntityTypes = []EntityType{
	EntityTaskInstance,
	EntityMedicationDose,
	EntityAppointment,
	EntityCareEvent,
}

// Valid reports whether the entity type is recognised.
func (e EntityType) Valid() bool { return slices.Contains(EntityTypes, e) }

// DeliveryStatus is how far a notification got towards somebody's phone.
type DeliveryStatus string

const (
	// DeliveryPending has not been attempted, or is waiting out a backoff.
	DeliveryPending DeliveryStatus = "pending"
	// DeliverySent reached the push provider.
	DeliverySent DeliveryStatus = "sent"
	// DeliveryFailed exhausted its attempts.
	DeliveryFailed DeliveryStatus = "failed"
	// DeliverySkipped had nowhere to go: the recipient has no reachable device,
	// or push is not configured at all. Not a failure, and not worth retrying —
	// the notification is still in the inbox, which is where it is read from
	// today anyway (plans/phase11.md §43).
	DeliverySkipped DeliveryStatus = "skipped"
)

// Notification is one thing MeraCare has decided to tell one person.
//
// It is a record of a decision, not a projection of care. Once written it stops
// tracking the thing it describes: an appointment that moves does not rewrite
// yesterday's reminder, in the same way a care event does not change when the
// task it mentions is renamed (plans/phase7.md §5, plans/phase11.md §6).
type Notification struct {
	ID uuid.UUID

	RecipientUserID uuid.UUID
	SeniorID        uuid.UUID

	Type Type

	// Title and Body are the words as they were sent. See wording.go for what
	// they are allowed to contain.
	Title string
	Body  string

	EntityType EntityType
	EntityID   uuid.UUID

	// ScheduledFor is the moment the notification is for. It orders the inbox
	// and gates delivery; a notification materialised ahead of time is invisible
	// until it arrives.
	ScheduledFor time.Time

	DeliveryStatus DeliveryStatus
	Attempts       int
	AvailableAt    time.Time
	DeliveredAt    *time.Time
	LastError      string

	ReadAt *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Read reports whether the recipient has seen this notification.
func (n Notification) Read() bool { return n.ReadAt != nil }

// ErrUnknownNotification is returned when a notification id does not belong to
// the caller.
//
// One error for "does not exist" and "is not yours", because the handler must
// answer both with the same 404. A distinguishable response would let anyone
// with a notification id learn whether it is real, which is a small oracle over
// other people's care (plans/phase11.md §§8, 31).
var ErrUnknownNotification = errors.New("notifications: no such notification for this user")

// notificationNamespace seeds the deterministic dedupe key below. A fixed
// random UUID, generated once; changing it would make every notification
// already sent look new and send it again.
var notificationNamespace = uuid.MustParse("2a9d5c31-7e64-4a0b-b8f2-1d6c9e4a7b03")

// dedupeKey identifies one notification-worthy occurrence for one person.
//
// A pure function of the decision: the same recipient, the same type, the same
// subject, the same moment. That is what lets the scheduler re-examine the same
// hour every minute and insert nothing the second time — the uniqueness is the
// database's, so it holds across processes as well as across ticks
// (plans/phase11.md §§20, 37).
//
// ScheduledFor is part of the identity on purpose. An appointment moved from
// 10:00 to 14:00 is a different notification, not an amendment to one already
// sent.
func dedupeKey(recipientID uuid.UUID, t Type, entityID uuid.UUID, scheduledFor time.Time) string {
	seed := fmt.Sprintf("%s|%s|%s|%s",
		recipientID, t, entityID, scheduledFor.UTC().Format(time.RFC3339))
	return uuid.NewSHA1(notificationNamespace, []byte(seed)).String()
}
