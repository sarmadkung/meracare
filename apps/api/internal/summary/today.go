package summary

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/genxcare/api/internal/appointments"
	"github.com/genxcare/api/internal/auth"
	"github.com/genxcare/api/internal/care"
	"github.com/genxcare/api/internal/medications"
	"github.com/genxcare/api/internal/seniors"
	"github.com/genxcare/api/internal/tasks"
)

// AppointmentLister returns a senior's appointments for a view.
type AppointmentLister interface {
	List(
		ctx context.Context,
		seniorID uuid.UUID,
		scope appointments.Scope,
		now time.Time,
	) ([]appointments.Appointment, error)
}

// ItemKind names which domain a row on Today came from.
type ItemKind string

const (
	// KindTask is a care task occurrence.
	KindTask ItemKind = "task"
	// KindDose is a scheduled medication dose.
	KindDose ItemKind = "dose"
	// KindAppointment is a visit.
	KindAppointment ItemKind = "appointment"
)

// Item is one thing happening today, in one senior's care.
type Item struct {
	Kind     ItemKind
	ID       uuid.UUID
	SeniorID uuid.UUID
	// SeniorName is carried on every row because this list spans circles:
	// "Metformin at 2pm" says nothing without knowing whose 2pm it is.
	SeniorName string
	// Timezone is the senior's, so the reader sees the time that person will
	// experience rather than the one on the reader's own phone.
	Timezone string

	Title string
	// Detail is the dosage, the provider, or empty.
	Detail       string
	ScheduledFor time.Time
	Status       string
	// AssignedToMe is true only for work explicitly given to this reader.
	AssignedToMe bool
}

// TodayService gathers everything due today across the caller's circles.
type TodayService struct {
	seniors      SeniorLister
	tasks        TaskLister
	doses        DoseLister
	appointments AppointmentLister
}

// NewTodayService builds the service.
func NewTodayService(
	seniorLister SeniorLister,
	taskLister TaskLister,
	doseLister DoseLister,
	appointmentLister AppointmentLister,
) *TodayService {
	return &TodayService{
		seniors:      seniorLister,
		tasks:        taskLister,
		doses:        doseLister,
		appointments: appointmentLister,
	}
}

// ForUser returns today's care across every circle the caller belongs to,
// soonest first.
//
// Deliberately not filtered to what is assigned to the reader. `/v1/tasks`
// already answers that, and it is the right answer for a professional working
// a round — but a family member caring alone assigns nothing to themselves, so
// a Today built only from assignments is permanently empty for the most common
// user. Assignment is reported per row instead, and the client leads with it.
//
// Sorted by time across circles rather than grouped by person: a caregiver
// works through their day in order, not one client at a time.
func (s *TodayService) ForUser(
	ctx context.Context,
	principal auth.Principal,
	now time.Time,
) ([]Item, error) {
	memberships, err := s.seniors.List(ctx, principal)
	if err != nil {
		return nil, err
	}

	var items []Item
	for _, membership := range memberships {
		items = append(items, s.forSenior(ctx, membership, principal.UserID, now)...)
	}

	sort.SliceStable(items, func(a, b int) bool {
		return items[a].ScheduledFor.Before(items[b].ScheduledFor)
	})

	return items, nil
}

func (s *TodayService) forSenior(
	ctx context.Context,
	membership seniors.Membership,
	userID uuid.UUID,
	now time.Time,
) []Item {
	senior := membership.Senior
	var items []Item

	base := func(kind ItemKind, id uuid.UUID) Item {
		return Item{
			Kind:       kind,
			ID:         id,
			SeniorID:   senior.ID,
			SeniorName: senior.DisplayName,
			Timezone:   senior.Timezone,
		}
	}

	if membership.Relationship.Can(care.PermissionTasksView) {
		instances, err := s.tasks.List(
			ctx, tasks.ListInput{SeniorID: senior.ID, Scope: tasks.ScopeToday}, now,
		)
		if err == nil {
			for _, instance := range instances {
				item := base(KindTask, instance.ID)
				item.Title = instance.Title
				item.ScheduledFor = instance.ScheduledFor
				item.Status = string(instance.EffectiveStatus(now))
				item.AssignedToMe = instance.AssignedUserID != nil && *instance.AssignedUserID == userID
				items = append(items, item)
			}
		}
	}

	if membership.Relationship.Can(care.PermissionMedicationsView) {
		instances, err := s.doses.ListDoses(
			ctx, medications.ListDosesInput{SeniorID: senior.ID, Scope: medications.ScopeToday}, now,
		)
		if err == nil {
			for _, instance := range instances {
				item := base(KindDose, instance.ID)
				item.Title = instance.Name
				item.Detail = instance.Dosage
				item.ScheduledFor = instance.ScheduledFor
				item.Status = string(instance.EffectiveStatus(now))
				items = append(items, item)
			}
		}
	}

	if membership.Relationship.Can(care.PermissionAppointmentsView) {
		visits, err := s.appointments.List(ctx, senior.ID, appointments.ScopeToday, now)
		if err == nil {
			for _, visit := range visits {
				item := base(KindAppointment, visit.ID)
				item.Title = visit.Title
				item.Detail = visit.ProviderName
				item.ScheduledFor = visit.ScheduledAt
				item.Status = string(visit.Status)
				item.AssignedToMe = visit.AssignedUserID != nil && *visit.AssignedUserID == userID
				items = append(items, item)
			}
		}
	}

	return items
}
