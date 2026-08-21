package notifications

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/meracare/api/internal/care"
)

// How far ahead and how far back one materialisation pass looks.
const (
	// materialiseHorizon is how far into the future a pass writes notifications.
	//
	// Deliberately short. A row written now is a decision taken now, and a
	// decision taken a week early is a decision taken with a week-old view of
	// the care, the permissions, and the preferences. An hour is long enough
	// that the sweep does not have to run at exactly the right second, and
	// short enough that a cancelled appointment almost never has a notification
	// already waiting for it (plans/phase11.md §§21, 71).
	materialiseHorizon = time.Hour

	// overdueGrace is how long after its time a task is left alone.
	//
	// Overdue is derived from the clock the moment the time passes
	// (tasks.Instance.EffectiveStatus), and telling somebody so at the same
	// instant would make the overdue alert a duplicate of the reminder they got
	// fifteen minutes earlier. Half an hour is long enough to have done the
	// thing and recorded it (plans/phase11.md §17).
	overdueGrace = 30 * time.Minute

	// lookback is how far back a pass reconsiders.
	//
	// Everything materialised is idempotent, so re-examining the recent past
	// costs an insert that does nothing and buys the ability to survive a
	// restart, a slow tick, or a deploy without silently losing the
	// notifications that fell in the gap (plans/phase11.md §20).
	lookback = 15 * time.Minute

	// maxLeadTime is the longest any notification runs ahead of its subject. It
	// widens the window queried from the domains so a 24-hour appointment
	// reminder can be found the day before.
	maxLeadTime = 24 * time.Hour
)

// leadTimes is how long before its subject each notification type fires.
//
// Appointments get two, which is the one place Phase 11 adds an offset rather
// than reusing Phase 8's: a day's notice is what lets somebody arrange a lift
// or take the morning off, and an hour's notice is what gets them out of the
// door. Neither alone does both (plans/phase11.md §15).
//
// The 15-minute reminder offsets are Phase 8's, unchanged, so a dose reminds at
// the same moment whether the device scheduled it or the server pushed it.
func leadTimes(t Type) []time.Duration {
	switch t {
	case TypeAppointmentReminder:
		return []time.Duration{24 * time.Hour, time.Hour}
	case TypeMedicationReminder, TypeTaskReminder:
		return []time.Duration{15 * time.Minute}
	default:
		return nil
	}
}

// permissionForType is the existing permission a notification type requires.
//
// No new "notifications.*" vocabulary: being told a dose is due is a weaker act
// than reading the medication list, so the view permission that already governs
// the latter governs this too (docs/02-permissions-and-authorization.md).
func permissionForType(t Type) care.Permission {
	switch t {
	case TypeTaskReminder, TypeTaskOverdue:
		return care.PermissionTasksView
	case TypeMedicationReminder:
		return care.PermissionMedicationsView
	case TypeAppointmentReminder:
		return care.PermissionAppointmentsView
	default:
		return care.PermissionActivityView
	}
}

// entityForType is what each notification type points at.
func entityForType(t Type) EntityType {
	switch t {
	case TypeTaskReminder, TypeTaskOverdue:
		return EntityTaskInstance
	case TypeMedicationReminder:
		return EntityMedicationDose
	case TypeAppointmentReminder:
		return EntityAppointment
	default:
		return EntityCareEvent
	}
}

// UserMembership is one person's active access to one senior.
type UserMembership struct {
	UserID      uuid.UUID
	Senior      Senior
	Permissions care.PermissionSet
}

// Roster lists every active membership in the system.
//
// The scheduler works outwards from memberships rather than from users because
// a user with no active relationship has nothing to be notified about, and a
// revoked caregiver disappears from the roster the moment they are revoked —
// which is the whole of "a revoked caregiver stops being notified"
// (plans/phase11.md §31).
type Roster interface {
	ActiveMemberships(ctx context.Context) ([]UserMembership, error)
}

// OverdueSource reports care that should have happened and has not.
type OverdueSource interface {
	// Overdue returns the senior's still-outstanding occurrences that were due
	// in [from, to). The domain decides what outstanding means; this package
	// must not acquire a second definition of overdue (plans/phase11.md §17).
	Overdue(ctx context.Context, seniorID uuid.UUID, from, to time.Time) ([]Due, error)
}

// ActivityKind is the small set of things worth telling somebody else about.
//
// Far smaller than the care-event vocabulary, and that is the point: a care
// event is a record, a notification is an interruption, and most records are
// not worth interrupting anybody for. Creating a task is not news; completing
// one is (plans/phase11.md §§19, 45, 53).
type ActivityKind string

const (
	// ActivityMedicationRecorded is a dose taken or skipped by somebody.
	ActivityMedicationRecorded ActivityKind = "medication_recorded"
	// ActivityTaskCompleted is a care task done.
	ActivityTaskCompleted ActivityKind = "task_completed"
	// ActivityAppointmentCompleted is an appointment attended.
	ActivityAppointmentCompleted ActivityKind = "appointment_completed"
	// ActivityMemberJoined is somebody accepting an invitation into the circle.
	ActivityMemberJoined ActivityKind = "member_joined"
)

// Activity is one care event that may be worth a notification.
type Activity struct {
	// EventID is the care event's own id. It is what the notification points at
	// and what makes it unique, so the same event never notifies twice.
	EventID  uuid.UUID
	SeniorID uuid.UUID
	Kind     ActivityKind
	// ActorUserID is who did it. They are never notified about their own action
	// — an app that tells you what you just did is an app nobody trusts to only
	// tell them useful things.
	ActorUserID *uuid.UUID
	ActorName   string
	OccurredAt  time.Time
}

// ActivitySource reports recent care events across every senior.
type ActivitySource interface {
	// RecentActivity returns notification-worthy events in [from, to).
	RecentActivity(ctx context.Context, from, to time.Time) ([]Activity, error)
}

// pending is one notification a pass has decided should exist.
type pending struct {
	RecipientUserID uuid.UUID
	SeniorID        uuid.UUID
	Type            Type
	Title           string
	Body            string
	EntityType      EntityType
	EntityID        uuid.UUID
	ScheduledFor    time.Time
	DedupeKey       string
}

// newPending builds a notification and derives its dedupe key.
func newPending(
	recipientID uuid.UUID,
	t Type,
	senior Senior,
	entityID uuid.UUID,
	scheduledFor time.Time,
	body string,
) pending {
	// Truncated to the second so the key is stable: a domain that returns a
	// time with microseconds on one pass and without them on the next would
	// otherwise produce two "different" notifications for one occurrence.
	scheduledFor = scheduledFor.UTC().Truncate(time.Second)

	return pending{
		RecipientUserID: recipientID,
		SeniorID:        senior.ID,
		Type:            t,
		Title:           Title(t),
		Body:            body,
		EntityType:      entityForType(t),
		EntityID:        entityID,
		ScheduledFor:    scheduledFor,
		DedupeKey:       dedupeKey(recipientID, t, entityID, scheduledFor),
	}
}

// materialiseInput is everything one pass needs. Grouped into a struct because
// the pass is a pure function of it, which is what makes it testable without a
// database or a clock.
type materialiseInput struct {
	Memberships []UserMembership
	Preferences map[uuid.UUID]Preferences
	Reminders   map[Type]ScheduleSource
	Overdue     OverdueSource
	Activity    ActivitySource
	Now         time.Time
}

// materialise decides every notification that should exist right now.
//
// It reads the domains and returns rows; it writes nothing and sends nothing.
// Keeping it that way is what makes "the same pass run twice changes nothing"
// something you can read off the code rather than something you have to trust
// the database to rescue (plans/phase11.md §21).
func materialise(ctx context.Context, in materialiseInput) ([]pending, error) {
	found := make([]pending, 0)

	reminders, err := materialiseReminders(ctx, in)
	if err != nil {
		return nil, err
	}
	found = append(found, reminders...)

	overdue, err := materialiseOverdue(ctx, in)
	if err != nil {
		return nil, err
	}
	found = append(found, overdue...)

	activity, err := materialiseActivity(ctx, in)
	if err != nil {
		return nil, err
	}
	return append(found, activity...), nil
}

// preferencesFor returns a user's settings, or the defaults they have never
// changed. Absent is not an error, exactly as in the repository: writing a row
// of defaults on first sign-in would put a notification write on the
// authentication path for an answer that is already known.
func (in materialiseInput) preferencesFor(userID uuid.UUID) Preferences {
	if preferences, ok := in.Preferences[userID]; ok {
		return preferences
	}
	return DefaultPreferences(userID)
}

// allows reports whether this member should be told about this type at all:
// they want it, and they are permitted to see the thing it is about.
func (in materialiseInput) allows(membership UserMembership, t Type) bool {
	return in.preferencesFor(membership.UserID).wantsType(t) &&
		membership.Permissions.Has(permissionForType(t))
}

// materialiseReminders covers the three "something is about to happen" types.
func materialiseReminders(ctx context.Context, in materialiseInput) ([]pending, error) {
	found := make([]pending, 0)
	windowEnd := in.Now.Add(materialiseHorizon)

	for _, membership := range in.Memberships {
		for _, reminderType := range ReminderTypes {
			t := reminderType.notificationType()
			if !in.allows(membership, t) {
				continue
			}

			source, ok := in.Reminders[t]
			if !ok {
				continue
			}

			// Widened by the longest lead so a 24-hour appointment reminder
			// finds tomorrow's appointment.
			due, err := source.Upcoming(ctx, membership.Senior.ID, in.Now, windowEnd.Add(maxLeadTime))
			if err != nil {
				return nil, err
			}

			for _, item := range due {
				if !addressedTo(item, membership.UserID) {
					continue
				}

				for _, lead := range leadTimes(t) {
					fireAt := item.At.Add(-lead)

					// Outside this pass's window. Something further out will be
					// materialised by a later pass, and something already past
					// is not worth waking anybody for.
					if fireAt.Before(in.Now) || !fireAt.Before(windowEnd) {
						continue
					}

					subject := subjectOf(membership.Senior)
					found = append(found, newPending(
						membership.UserID, t, membership.Senior, item.EntityID, fireAt,
						Body(t, subject, item.At, fireAt),
					))
				}
			}
		}
	}
	return found, nil
}

// materialiseOverdue covers tasks whose time has passed with nothing recorded.
//
// Written looking backwards rather than forwards, and that is the design rather
// than an accident. A task is overdue only if nobody dealt with it, which is
// not knowable in advance — so instead of scheduling an alert and hoping to
// cancel it, the pass asks "what is overdue *now*?" and writes only what
// actually is. A task completed before the sweep never produces an alert at
// all, because it is not in the answer (plans/phase11.md §§17, 71).
func materialiseOverdue(ctx context.Context, in materialiseInput) ([]pending, error) {
	found := make([]pending, 0)
	if in.Overdue == nil {
		return found, nil
	}

	// Due times whose grace has expired within this pass's reach.
	to := in.Now.Add(-overdueGrace)
	from := to.Add(-lookback)

	for _, membership := range in.Memberships {
		if !in.allows(membership, TypeTaskOverdue) {
			continue
		}

		due, err := in.Overdue.Overdue(ctx, membership.Senior.ID, from, to)
		if err != nil {
			return nil, err
		}

		for _, item := range due {
			if !addressedTo(item, membership.UserID) {
				continue
			}

			alertAt := item.At.Add(overdueGrace)
			subject := subjectOf(membership.Senior)
			found = append(found, newPending(
				membership.UserID, TypeTaskOverdue, membership.Senior, item.EntityID, alertAt,
				Body(TypeTaskOverdue, subject, item.At, alertAt),
			))
		}
	}
	return found, nil
}

// materialiseActivity covers "somebody else did something".
//
// The events are read after they are committed, which is what guarantees a
// notification is never sent for an action that rolled back — the same
// guarantee plans/phase11.md §52 asks for from a transactional write, obtained
// without threading a notification write through tasks, medications,
// appointments, and members. An uncommitted care event is invisible here, so
// there is nothing to undo (§53).
func materialiseActivity(ctx context.Context, in materialiseInput) ([]pending, error) {
	found := make([]pending, 0)
	if in.Activity == nil {
		return found, nil
	}

	activities, err := in.Activity.RecentActivity(ctx, in.Now.Add(-lookback), in.Now)
	if err != nil {
		return nil, err
	}
	if len(activities) == 0 {
		return found, nil
	}

	// Grouped by senior so each event is matched against that senior's circle
	// rather than against every membership in the system.
	circles := make(map[uuid.UUID][]UserMembership, len(in.Memberships))
	for _, membership := range in.Memberships {
		circles[membership.Senior.ID] = append(circles[membership.Senior.ID], membership)
	}

	for _, activity := range activities {
		for _, membership := range circles[activity.SeniorID] {
			// Never tell somebody what they themselves just did.
			if activity.ActorUserID != nil && *activity.ActorUserID == membership.UserID {
				continue
			}
			if !in.allows(membership, TypeCareActivity) {
				continue
			}

			found = append(found, newPending(
				membership.UserID, TypeCareActivity, membership.Senior,
				activity.EventID, activity.OccurredAt,
				ActivityBody(activity.Kind, activity.ActorName, subjectOf(membership.Senior)),
			))
		}
	}
	return found, nil
}

func subjectOf(senior Senior) Subject {
	return Subject{Name: senior.DisplayName, Timezone: senior.Timezone}
}
