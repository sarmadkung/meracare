package server

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/meracare/api/internal/appointments"
	"github.com/meracare/api/internal/careevents"
	"github.com/meracare/api/internal/invitations"
	"github.com/meracare/api/internal/medications"
	"github.com/meracare/api/internal/notifications"
	"github.com/meracare/api/internal/seniors"
	"github.com/meracare/api/internal/tasks"
	"github.com/meracare/api/internal/users"
)

// userLookup adapts the users repository to the narrow interface the invitation
// flow needs.
//
// The adapter lives here, at the composition root, so internal/invitations
// depends on a two-method interface rather than on the whole users package.
type userLookup struct {
	repo *users.Repository
}

func (l userLookup) GetByID(ctx context.Context, id uuid.UUID) (invitations.UserSummary, error) {
	user, err := l.repo.GetByID(ctx, id)
	if err != nil {
		return invitations.UserSummary{}, err
	}
	return invitations.UserSummary{
		ID:          user.ID,
		DisplayName: user.DisplayName,
		Email:       user.Email,
	}, nil
}

func (l userLookup) FindIDByEmail(ctx context.Context, email string) (uuid.UUID, error) {
	return l.repo.FindIDByEmail(ctx, email)
}

// The four adapters below let internal/notifications ask "what is coming up?"
// without importing tasks, medications, appointments, or seniors.
//
// The alternative — a notifications package that knows about doses — is how a
// reminder system starts making care decisions. Keeping the translation here,
// at the composition root, means the notification code cannot reach a dosage
// even by accident, which is most of what plans/phase8.md §§1, 16, and 17 ask
// for (docs/05-api-and-backend-spec.md).

// circleSource adapts the seniors repository to the reminder plan's view of a
// care circle.
type circleSource struct {
	repo *seniors.Repository
}

// Memberships returns only the caller's active relationships.
//
// This is where a revoked caregiver stops being reminded: they have no active
// membership, so no senior of theirs contributes reminders, and the next plan
// their device fetches is empty of that senior (plans/phase8.md §23).
func (c circleSource) Memberships(
	ctx context.Context,
	userID uuid.UUID,
) ([]notifications.Membership, error) {
	found, err := c.repo.ListForUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	memberships := make([]notifications.Membership, 0, len(found))
	for _, membership := range found {
		if !membership.Relationship.IsActive() {
			continue
		}

		memberships = append(memberships, notifications.Membership{
			Senior: notifications.Senior{
				ID:          membership.Senior.ID,
				DisplayName: membership.Senior.DisplayName,
				Timezone:    membership.Senior.Timezone,
			},
			Permissions: membership.Relationship.Permissions,
		})
	}
	return memberships, nil
}

// taskSource adapts the task service to the reminder plan.
type taskSource struct {
	service *tasks.Service
}

// Upcoming returns the senior's outstanding task occurrences in the window.
//
// Going through the service rather than the repository is deliberate: the
// service materialises occurrences for the window before reading them, so a
// recurring task that nobody has opened the app to look at still produces
// reminders. That also keeps the recurrence engine singular — the plan consumes
// the expansion tasks already does, and never expands a rule itself
// (plans/phase8.md §21).
func (t taskSource) Upcoming(
	ctx context.Context,
	seniorID uuid.UUID,
	from, to time.Time,
) ([]notifications.Due, error) {
	instances, err := t.service.List(ctx, tasks.ListInput{
		SeniorID: seniorID,
		Scope:    tasks.ScopeWindow,
		From:     from,
		To:       to,
	}, from)
	if err != nil {
		return nil, err
	}

	due := make([]notifications.Due, 0, len(instances))
	for _, instance := range instances {
		// Anything already dealt with needs no reminder. Overdue is derived
		// rather than stored, so a still-pending occurrence is exactly what is
		// outstanding (plans/phase8.md §22).
		if instance.Status != tasks.StatusPending {
			continue
		}

		due = append(due, notifications.Due{
			EntityID:   instance.ID,
			At:         instance.ScheduledFor,
			AssigneeID: instance.AssignedUserID,
		})
	}
	return due, nil
}

// medicationSource adapts the medication service to the reminder plan.
type medicationSource struct {
	service *medications.Service
}

// Upcoming returns the senior's outstanding doses in the window.
//
// Doses carry no assignee: a medicine is the senior's, and anybody permitted to
// record it may be the one who helps. So every circle member with
// medications.view is reminded, which is the behaviour a family sharing care
// actually needs (plans/phase8.md §4).
func (m medicationSource) Upcoming(
	ctx context.Context,
	seniorID uuid.UUID,
	from, to time.Time,
) ([]notifications.Due, error) {
	instances, err := m.service.ListDoses(ctx, medications.ListDosesInput{
		SeniorID: seniorID,
		Scope:    medications.ScopeWindow,
		From:     from,
		To:       to,
	}, from)
	if err != nil {
		return nil, err
	}

	due := make([]notifications.Due, 0, len(instances))
	for _, instance := range instances {
		if instance.Status != medications.StatusPending {
			continue
		}

		due = append(due, notifications.Due{EntityID: instance.ID, At: instance.ScheduledFor})
	}
	return due, nil
}

// appointmentSource adapts the appointment service to the reminder plan.
type appointmentSource struct {
	service *appointments.Service
}

// Upcoming returns the senior's still-scheduled appointments in the window.
//
// A cancelled appointment is filtered out here, and that is the whole of
// "cancelling an appointment cancels its reminder": the plan stops containing
// it, and the device's next reconciliation removes it. Nothing has to remember
// to clean anything up (plans/phase8.md §22).
func (a appointmentSource) Upcoming(
	ctx context.Context,
	seniorID uuid.UUID,
	from, to time.Time,
) ([]notifications.Due, error) {
	booked, err := a.service.Window(ctx, seniorID, from, to)
	if err != nil {
		return nil, err
	}

	due := make([]notifications.Due, 0, len(booked))
	for _, appointment := range booked {
		if appointment.Status != appointments.StatusScheduled {
			continue
		}

		due = append(due, notifications.Due{
			EntityID:   appointment.ID,
			At:         appointment.ScheduledAt,
			AssigneeID: appointment.AssignedUserID,
		})
	}
	return due, nil
}

// The three adapters below are Phase 11's: the roster the scheduler sweeps, the
// overdue tasks it alerts about, and the care events it turns into activity
// notifications. Like the four above, they live here so internal/notifications
// keeps depending on "things fall due" and "things happened" rather than on
// tasks, care events, or relationships (plans/phase11.md §4).

// roster adapts the seniors repository to the scheduler's view of who exists.
type roster struct {
	repo *seniors.Repository
}

// ActiveMemberships returns every active relationship, with its permissions.
//
// Permissions come from the stored relationship, never from anything the
// notification carries — which is why a revoked caregiver stops being
// materialised for at the same instant they lose access, rather than when their
// device next checks in (plans/phase11.md §31).
func (r roster) ActiveMemberships(ctx context.Context) ([]notifications.UserMembership, error) {
	found, err := r.repo.ListAllActive(ctx)
	if err != nil {
		return nil, err
	}

	memberships := make([]notifications.UserMembership, 0, len(found))
	for _, membership := range found {
		if !membership.Relationship.IsActive() {
			continue
		}

		memberships = append(memberships, notifications.UserMembership{
			UserID: membership.Relationship.UserID,
			Senior: notifications.Senior{
				ID:          membership.Senior.ID,
				DisplayName: membership.Senior.DisplayName,
				Timezone:    membership.Senior.Timezone,
			},
			Permissions: membership.Relationship.Permissions,
		})
	}
	return memberships, nil
}

// overdueSource adapts the task service to the overdue alert.
type overdueSource struct {
	service *tasks.Service
}

// Overdue returns the senior's task occurrences that were due in the window and
// are still outstanding.
//
// It asks the tasks domain what is still pending rather than computing overdue
// itself. tasks.Instance.EffectiveStatus is the single definition of overdue in
// MeraCare and stays that way: a still-stored-pending occurrence whose time has
// passed is exactly what that method calls overdue, and a second definition
// here is how two parts of an app start disagreeing about whether Amma took her
// tablets (plans/phase11.md §17).
func (o overdueSource) Overdue(
	ctx context.Context,
	seniorID uuid.UUID,
	from, to time.Time,
) ([]notifications.Due, error) {
	instances, err := o.service.List(ctx, tasks.ListInput{
		SeniorID: seniorID,
		Scope:    tasks.ScopeWindow,
		From:     from,
		To:       to,
	}, to)
	if err != nil {
		return nil, err
	}

	due := make([]notifications.Due, 0, len(instances))
	for _, instance := range instances {
		// Anything completed, skipped, or cancelled needs no alert. Only a
		// still-pending occurrence in a window that has already passed is
		// overdue, which is the domain's own rule read back.
		if instance.Status != tasks.StatusPending {
			continue
		}

		due = append(due, notifications.Due{
			EntityID:   instance.ID,
			At:         instance.ScheduledFor,
			AssigneeID: instance.AssignedUserID,
		})
	}
	return due, nil
}

// activitySource adapts the care-event timeline to activity notifications.
type activitySource struct {
	repo *careevents.Repository
}

// notifiableActivity maps the few care events worth interrupting somebody for
// onto the notification vocabulary.
//
// Four of the fifteen event types. A care event is a record and a notification
// is an interruption, and most records are not worth one: creating a task,
// inviting a member, or cancelling an appointment all belong in the timeline
// and none of them needs to make a phone buzz (plans/phase11.md §§19, 45).
var notifiableActivity = map[careevents.Type]notifications.ActivityKind{
	careevents.TypeMedicationTaken:      notifications.ActivityMedicationRecorded,
	careevents.TypeMedicationSkipped:    notifications.ActivityMedicationRecorded,
	careevents.TypeTaskCompleted:        notifications.ActivityTaskCompleted,
	careevents.TypeAppointmentCompleted: notifications.ActivityAppointmentCompleted,
	careevents.TypeMemberJoined:         notifications.ActivityMemberJoined,
}

// RecentActivity returns the notification-worthy events in the window.
func (a activitySource) RecentActivity(
	ctx context.Context,
	from, to time.Time,
) ([]notifications.Activity, error) {
	types := make([]careevents.Type, 0, len(notifiableActivity))
	for eventType := range notifiableActivity {
		types = append(types, eventType)
	}

	events, err := a.repo.ListRecent(ctx, types, from, to)
	if err != nil {
		return nil, err
	}

	activities := make([]notifications.Activity, 0, len(events))
	for _, event := range events {
		kind, ok := notifiableActivity[event.Type]
		if !ok {
			continue
		}

		activities = append(activities, notifications.Activity{
			EventID:     event.ID,
			SeniorID:    event.SeniorID,
			Kind:        kind,
			ActorUserID: event.ActorUserID,
			ActorName:   event.ActorName,
			OccurredAt:  event.OccurredAt,
		})
	}
	return activities, nil
}
