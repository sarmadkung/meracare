package notifications

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/genxcare/api/internal/care"
)

// Senior is the little a reminder needs to know about the person being cared
// for: who they are, and which clock their care runs on.
type Senior struct {
	ID          uuid.UUID
	DisplayName string
	Timezone    string
}

// Membership is one user's current, active access to one senior.
//
// Permissions come from the stored relationship, never from the request. A
// caregiver reminded about medication is a caregiver who could open the
// medication screen; the reminder grants nothing (plans/phase8.md §5).
type Membership struct {
	Senior      Senior
	Permissions care.PermissionSet
}

// Due is one thing falling due, reduced to what scheduling needs.
//
// Deliberately not a task, a dose, or an appointment. This package must not
// learn what a dosage is: everything it could do with that knowledge would be
// something plans/phase8.md §§16, 17, and 47 forbid putting into a
// notification.
type Due struct {
	EntityID uuid.UUID
	At       time.Time
	// AssigneeID is the circle member responsible, when one has been named.
	AssigneeID *uuid.UUID
}

// ScheduleSource is a domain that can say what is coming up for a senior.
//
// One narrow interface per domain, implemented by adapters at the composition
// root, so this package depends on "things fall due at times" rather than on
// tasks, medications, and appointments. It is also what keeps the recurrence
// engine singular: the sources return the schedule the domains already expand,
// and nothing here re-expands a rule (plans/phase8.md §§1, 21).
type ScheduleSource interface {
	// Upcoming returns what is still outstanding for the senior in [from, to).
	// Anything already dealt with — a dose taken, a task completed, an
	// appointment cancelled — must not be returned: a reminder for it would be
	// noise at best and wrong at worst (plans/phase8.md §22).
	Upcoming(ctx context.Context, seniorID uuid.UUID, from, to time.Time) ([]Due, error)
}

// CircleSource lists the seniors a user currently has access to.
type CircleSource interface {
	// Memberships returns only active relationships. A revoked caregiver has
	// none, which is what stops their reminders (plans/phase8.md §23).
	Memberships(ctx context.Context, userID uuid.UUID) ([]Membership, error)
}

// permissionFor is the permission a reminder type requires.
//
// The existing view permissions, not new ones. Being reminded about a dose is a
// weaker act than reading the medication list, so no separate
// "notifications.*" vocabulary is needed and none is invented
// (docs/02-permissions-and-authorization.md, plans/phase8.md §5).
func permissionFor(t ReminderType) care.Permission {
	switch t {
	case ReminderTaskReminder:
		return care.PermissionTasksView
	case ReminderMedicationReminder:
		return care.PermissionMedicationsView
	default:
		return care.PermissionAppointmentsView
	}
}

// planFor builds one user's reminder plan across every senior they can reach.
//
// Nothing is read from a notifications table, because there is none. The plan
// is a projection of current domain state through current authorization and
// current preferences, computed fresh on every request. That is what makes the
// hard cases disappear rather than need handling: a stopped medicine has no
// upcoming doses, so it has no reminders; a revoked caregiver has no
// memberships, so they have no reminders; a rescheduled appointment produces a
// different reminder because the time is part of the identity
// (plans/phase8.md §§22, 23, 31).
func planFor(
	ctx context.Context,
	sources map[ReminderType]ScheduleSource,
	memberships []Membership,
	userID uuid.UUID,
	preferences Preferences,
	from, to time.Time,
) ([]Reminder, error) {
	reminders := make([]Reminder, 0)

	for _, membership := range memberships {
		for _, reminderType := range ReminderTypes {
			if !preferences.wants(reminderType) {
				continue
			}
			if !membership.Permissions.Has(permissionFor(reminderType)) {
				continue
			}

			source, ok := sources[reminderType]
			if !ok {
				continue
			}

			// Widened by the lead time at the far end: something due just after
			// the window closes still has its reminder inside the window.
			lead := reminderType.leadTime()
			due, err := source.Upcoming(ctx, membership.Senior.ID, from, to.Add(lead))
			if err != nil {
				return nil, err
			}

			for _, item := range due {
				if !addressedTo(item, userID) {
					continue
				}

				reminder := newReminder(userID, reminderType, membership.Senior, item.EntityID, item.At)

				// A reminder whose moment has already passed cannot be
				// scheduled, and firing it late would tell somebody to do
				// something at the wrong time. The care itself is still on the
				// screens either way.
				if reminder.FireAt.Before(from) || !reminder.FireAt.Before(to) {
					continue
				}

				reminders = append(reminders, reminder)
			}
		}
	}

	// Soonest first, then by id so a tie is ordered the same way every time —
	// which matters because the list is truncated below, and an unstable
	// ordering would swap reminders in and out of the plan on every refresh,
	// rescheduling them for no reason.
	slices.SortFunc(reminders, func(a, b Reminder) int {
		if c := a.FireAt.Compare(b.FireAt); c != 0 {
			return c
		}
		return cmp.Compare(a.ID.String(), b.ID.String())
	})

	if len(reminders) > maxReminders {
		reminders = reminders[:maxReminders]
	}

	return reminders, nil
}

// addressedTo reports whether this user is the one to remind.
//
// When somebody has been named as responsible, the reminder is theirs alone: a
// circle of six should not all be told to take Amma to the clinic when one
// daughter is driving. When nobody has been named, the work belongs to whoever
// can do it, so everyone permitted to see it is reminded (plans/phase8.md §4).
func addressedTo(item Due, userID uuid.UUID) bool {
	return item.AssigneeID == nil || *item.AssigneeID == userID
}
