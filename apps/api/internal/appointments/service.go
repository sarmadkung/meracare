package appointments

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/genxcare/api/internal/care"
	"github.com/genxcare/api/internal/careevents"
	"github.com/genxcare/api/internal/relationships"
	"github.com/genxcare/api/internal/seniors"
)

// SeniorLookup loads the senior an appointment belongs to, for their timezone.
// internal/seniors.Repository satisfies it.
type SeniorLookup interface {
	GetByID(ctx context.Context, id uuid.UUID) (seniors.Senior, error)
}

// MemberLookup resolves a care-circle membership, so an appointment cannot be
// assigned to somebody outside the circle.
// internal/relationships.Repository satisfies it.
type MemberLookup interface {
	FindByUserAndSenior(ctx context.Context, userID, seniorID uuid.UUID) (relationships.Relationship, error)
}

// Service coordinates appointments.
type Service struct {
	appointments *Repository
	seniors      SeniorLookup
	members      MemberLookup
	// events records what happened, in the same transaction as the change
	// itself (plans/phase7.md §26).
	events *careevents.Recorder
}

// NewService builds the service.
func NewService(
	appointments *Repository,
	seniorLookup SeniorLookup,
	members MemberLookup,
	events *careevents.Recorder,
) *Service {
	return &Service{
		appointments: appointments,
		seniors:      seniorLookup,
		members:      members,
		events:       events,
	}
}

// ErrInvalidAssignee is returned when an appointment is assigned to somebody
// who is not an active member of the senior's circle.
var ErrInvalidAssignee = errors.New("assignee is not an active member of this care circle")

// ErrBadScope is returned when the requested view is not one the API offers. It
// is a sentinel so the handler can answer 400 without inspecting the message.
var ErrBadScope = errors.New("appointments: unknown view")

// upcomingHorizon is how far ahead the upcoming list looks.
//
// Long enough that next year's annual review appears the day it is booked. It
// is not really the bound that matters — maxUpcoming is — but a bounded range
// keeps the query an index scan over a known slice of the calendar rather than
// over everything a circle has ever booked.
const upcomingHorizon = 365 * 24 * time.Hour

// maxUpcoming bounds the upcoming list. Somebody with more than this many
// appointments ahead of them is not reading a list, and the history page is
// where a longer view belongs (plans/phase6.md §§5, 32).
const maxUpcoming = 100

// defaultHistoryPage and maxHistoryPage bound one page of history, so a client
// cannot ask for every appointment ever booked in one request
// (plans/phase6.md §§6, 32).
const (
	defaultHistoryPage = 30
	maxHistoryPage     = 100
)

// --- Reading ---------------------------------------------------------------

// Scope names one of the appointment views the app asks for.
type Scope string

const (
	// ScopeToday is the senior's own calendar day, from midnight to midnight
	// where they live.
	ScopeToday Scope = "today"
	// ScopeUpcoming is everything still to come, soonest first.
	ScopeUpcoming Scope = "upcoming"
	// ScopePast is everything already started, newest first and paged.
	ScopePast Scope = "past"
)

// Get loads one appointment. The caller's access to its senior is checked by
// the handler, which cannot know the senior until this has run.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (Appointment, error) {
	return s.appointments.Get(ctx, id)
}

// List returns a senior's appointments for one of the unpaged views.
//
// The senior's own timezone decides where their day begins: "today" for a
// mother in Karachi is her day, not her daughter's in London
// (plans/phase6.md §4).
func (s *Service) List(
	ctx context.Context,
	seniorID uuid.UUID,
	scope Scope,
	now time.Time,
) ([]Appointment, error) {
	switch scope {
	case ScopeUpcoming:
		return s.appointments.ListWindow(ctx, seniorID, now, now.Add(upcomingHorizon), maxUpcoming)

	case ScopeToday:
		senior, err := s.seniors.GetByID(ctx, seniorID)
		if err != nil {
			return nil, err
		}

		local := now.In(senior.Location())
		year, month, day := local.Date()
		start := time.Date(year, month, day, 0, 0, 0, 0, senior.Location())

		return s.appointments.ListWindow(ctx, seniorID, start, start.AddDate(0, 0, 1), maxUpcoming)

	default:
		return nil, fmt.Errorf("%w: %q", ErrBadScope, scope)
	}
}

// Window returns the senior's appointments starting in [from, to).
//
// An explicit range rather than one of the named scopes, for the reminder plan:
// it asks about a week from an arbitrary instant, which is neither "today" nor
// "upcoming". Every status is returned; deciding which of them still deserve a
// reminder belongs to the caller, not here (plans/phase8.md §14).
func (s *Service) Window(
	ctx context.Context,
	seniorID uuid.UUID,
	from, to time.Time,
) ([]Appointment, error) {
	return s.appointments.ListWindow(ctx, seniorID, from, to, maxUpcoming)
}

// History returns one page of a senior's past appointments, newest first.
//
// "Past" means the appointment's time has come, whatever became of it: a
// cancelled visit and a completed one both belong in the record of what
// happened (plans/phase6.md §6).
func (s *Service) History(
	ctx context.Context,
	seniorID uuid.UUID,
	cursor string,
	limit int,
	now time.Time,
) (Page, error) {
	if limit <= 0 || limit > maxHistoryPage {
		limit = defaultHistoryPage
	}
	return s.appointments.ListBefore(ctx, seniorID, now, cursor, limit)
}

// --- Writing ---------------------------------------------------------------

// CreateInput is a request to book an appointment.
type CreateInput struct {
	SeniorID uuid.UUID
	// CreatedByUserID is the authenticated caller. It is never read from the
	// request body: a client that could name the creator could attribute a
	// booking to somebody who never made it (plans/phase6.md §13).
	CreatedByUserID uuid.UUID

	Title        string
	Kind         Kind
	ProviderName string
	Location     string
	Notes        string

	AssignedUserID *uuid.UUID

	ScheduledAt time.Time
	EndsAt      *time.Time
}

// Create books an appointment.
func (s *Service) Create(ctx context.Context, input CreateInput) (Appointment, error) {
	if err := s.checkAssignee(ctx, input.SeniorID, input.AssignedUserID); err != nil {
		return Appointment{}, err
	}

	var appointment Appointment
	err := s.events.InTransaction(ctx, func(tx pgx.Tx, events *careevents.Repository) error {
		created, err := s.appointments.WithTx(tx).Create(ctx, CreateParams{
			SeniorID:        input.SeniorID,
			CreatedByUserID: input.CreatedByUserID,
			Title:           input.Title,
			Kind:            input.Kind,
			ProviderName:    input.ProviderName,
			Location:        input.Location,
			Notes:           input.Notes,
			AssignedUserID:  input.AssignedUserID,
			ScheduledAt:     input.ScheduledAt,
			EndsAt:          input.EndsAt,
		})
		if err != nil {
			return err
		}
		appointment = created

		return recordAppointmentEvent(
			ctx, events, created, careevents.TypeAppointmentCreated, input.CreatedByUserID)
	})
	if err != nil {
		return Appointment{}, err
	}
	return appointment, nil
}

// appointmentEventTypeFor maps settling an appointment to the event it produces.
var appointmentEventTypeFor = map[Action]careevents.Type{
	ActionComplete: careevents.TypeAppointmentCompleted,
	ActionCancel:   careevents.TypeAppointmentCancelled,
}

func recordAppointmentEvent(
	ctx context.Context,
	events *careevents.Repository,
	appointment Appointment,
	eventType careevents.Type,
	actorID uuid.UUID,
) error {
	_, err := events.Record(ctx, careevents.RecordParams{
		SeniorID:    appointment.SeniorID,
		ActorUserID: &actorID,
		Type:        eventType,
		EntityType:  careevents.EntityAppointment,
		EntityID:    appointment.ID,
		// The title as it reads now. An appointment cannot be edited once
		// settled, so for the two settling events this is also the title it
		// will keep (plans/phase6.md §8).
		Metadata: careevents.Metadata{careevents.MetaAppointmentName: appointment.Title},
	})
	return err
}

// UpdateInput edits an appointment.
type UpdateInput struct {
	AppointmentID uuid.UUID
	SeniorID      uuid.UUID

	Title        *string
	ProviderName *string
	Location     *string
	Notes        *string
	ScheduledAt  *time.Time

	Kind      *Kind
	ClearKind bool

	EndsAt      *time.Time
	ClearEndsAt bool

	AssignedUserID *uuid.UUID
	ClearAssignee  bool
}

// Update edits an appointment that has not been settled.
//
// A completed or cancelled appointment is refused rather than quietly changed.
// It is no longer a plan somebody can revise; it is the record of what happened,
// and rewriting it would destroy the history this phase exists to keep
// (plans/phase6.md §8). The conditional UPDATE in the repository does the
// refusing, and this re-reads the row to say which of the two reasons applies.
func (s *Service) Update(ctx context.Context, input UpdateInput) (Appointment, error) {
	if err := s.checkAssignee(ctx, input.SeniorID, input.AssignedUserID); err != nil {
		return Appointment{}, err
	}

	appointment, err := s.appointments.Update(ctx, input.AppointmentID, UpdateParams{
		Title:          input.Title,
		ProviderName:   input.ProviderName,
		Location:       input.Location,
		Notes:          input.Notes,
		ScheduledAt:    input.ScheduledAt,
		Kind:           input.Kind,
		ClearKind:      input.ClearKind,
		EndsAt:         input.EndsAt,
		ClearEndsAt:    input.ClearEndsAt,
		AssignedUserID: input.AssignedUserID,
		ClearAssignee:  input.ClearAssignee,
	})
	if err == nil {
		return appointment, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Appointment{}, err
	}

	// The row either does not exist or is no longer scheduled. Only one of those
	// is worth a distinct answer; the other stays a 404.
	current, err := s.appointments.Get(ctx, input.AppointmentID)
	if err != nil {
		return Appointment{}, err
	}
	if current.Status.Settled() {
		return Appointment{}, ErrSettled
	}
	return Appointment{}, ErrNotFound
}

// ActInput settles an appointment.
type ActInput struct {
	AppointmentID uuid.UUID
	Action        Action
	// ActorID is the authenticated caller, never a value from the request body
	// (plans/phase6.md §10).
	ActorID uuid.UUID
}

// ActResult reports what happened, including whether the action had already
// been recorded.
type ActResult struct {
	Appointment Appointment
	// Repeat is true when the appointment was already in the requested state,
	// which makes a retried request a success rather than a conflict.
	Repeat bool
}

// Act records an appointment as completed or cancelled.
//
// The conditional UPDATE in the repository is the real guard. When it matches
// no row the appointment is no longer scheduled, and this re-reads it to decide
// between two very different answers: the same action arriving twice, which
// succeeds quietly, and a different outcome already recorded, which is refused
// so the server's version stands (plans/phase6.md §§24–25).
func (s *Service) Act(ctx context.Context, input ActInput) (ActResult, error) {
	var result ActResult

	err := s.events.InTransaction(ctx, func(tx pgx.Tx, events *careevents.Repository) error {
		appointments := s.appointments.WithTx(tx)

		appointment, err := appointments.Act(ctx, ActParams{
			AppointmentID: input.AppointmentID,
			Action:        input.Action,
			ActorID:       input.ActorID,
		})
		if err == nil {
			result = ActResult{Appointment: appointment}

			eventType, ok := appointmentEventTypeFor[input.Action]
			if !ok {
				return fmt.Errorf("no care event defined for appointment action %q", input.Action)
			}
			return recordAppointmentEvent(ctx, events, appointment, eventType, input.ActorID)
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}

		current, err := appointments.Get(ctx, input.AppointmentID)
		if err != nil {
			return err
		}

		_, repeat, err := Transition(current.Status, input.Action)
		if err != nil {
			return err
		}

		// A repeat writes no second event (plans/phase7.md §20).
		result = ActResult{Appointment: current, Repeat: repeat}
		return nil
	})
	if err != nil {
		return ActResult{}, err
	}
	return result, nil
}

// checkAssignee refuses an assignee who cannot act for this senior.
//
// Assigning an appointment to somebody outside the circle would name a person
// who cannot see it, and would leak that the account exists. The same rule
// guards task assignment (plans/phase4.md §9).
func (s *Service) checkAssignee(ctx context.Context, seniorID uuid.UUID, assignee *uuid.UUID) error {
	if assignee == nil {
		return nil
	}

	relationship, err := s.members.FindByUserAndSenior(ctx, *assignee, seniorID)
	if err != nil {
		if errors.Is(err, relationships.ErrNotFound) {
			return ErrInvalidAssignee
		}
		return fmt.Errorf("check assignee: %w", err)
	}
	if relationship.Status != care.StatusActive {
		return ErrInvalidAssignee
	}
	return nil
}
