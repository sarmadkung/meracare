package tasks

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

// SeniorLookup loads the senior a task belongs to, for their timezone.
// internal/seniors.Repository satisfies it.
type SeniorLookup interface {
	GetByID(ctx context.Context, id uuid.UUID) (seniors.Senior, error)
}

// MemberLookup resolves a care-circle membership.
// internal/relationships.Repository satisfies it.
type MemberLookup interface {
	FindByUserAndSenior(ctx context.Context, userID, seniorID uuid.UUID) (relationships.Relationship, error)
}

// Service coordinates task templates, their occurrences, and the care circle
// rules that govern both.
type Service struct {
	tasks   *Repository
	seniors SeniorLookup
	members MemberLookup
	// events records what happened, in the same transaction as the change
	// itself (plans/phase7.md §26).
	events *careevents.Recorder
}

// NewService builds the service.
func NewService(
	tasks *Repository,
	seniorLookup SeniorLookup,
	members MemberLookup,
	events *careevents.Recorder,
) *Service {
	return &Service{tasks: tasks, seniors: seniorLookup, members: members, events: events}
}

// ErrInvalidAssignee is returned when a task is assigned to somebody who is not
// an active member of that senior's care circle.
var ErrInvalidAssignee = errors.New("assignee is not an active member of this care circle")

// ErrBadWindow is returned when the requested date range is unusable. It is a
// sentinel so the handler can answer 400 without inspecting the message.
var ErrBadWindow = errors.New("tasks: unusable date range")

// horizon is how far ahead occurrences are written when a recurring task is
// created or edited.
//
// Long enough that a circle can see the coming weeks, short enough that a daily
// task does not write years of rows nobody has looked at. Reading beyond it
// materialises on demand, so this is a convenience, not a limit.
const horizon = 60 * 24 * time.Hour

// maxWindowDays bounds an explicit date range, so one request cannot ask the
// server to expand every recurrence for a decade.
const maxWindowDays = 92

// maxOverdue bounds the overdue list. A circle that is hundreds of tasks behind
// needs a conversation, not a longer list.
const maxOverdue = 200

// CreateInput is a request to create a care task.
//
// Exactly one of ScheduledFor (a one-time task) or Recurrence with DueTime (a
// recurring one) is set; the handler has already established which.
type CreateInput struct {
	SeniorID        uuid.UUID
	CreatedByUserID uuid.UUID

	Title       string
	Description string

	AssignedUserID *uuid.UUID

	ScheduledFor *time.Time

	Recurrence *Recurrence
	DueTime    *TimeOfDay
}

// Created is the result of creating a task: a one-time occurrence, or a
// template together with the occurrences it has produced so far.
type Created struct {
	Template  *Template
	Instances []Instance
}

// Create makes a one-time or recurring care task.
func (s *Service) Create(ctx context.Context, input CreateInput, now time.Time) (Created, error) {
	if err := s.checkAssignee(ctx, input.SeniorID, input.AssignedUserID); err != nil {
		return Created{}, err
	}

	if input.Recurrence == nil {
		var instance Instance
		err := s.events.InTransaction(ctx, func(tx pgx.Tx, events *careevents.Repository) error {
			created, err := s.tasks.WithTx(tx).CreateInstance(ctx, CreateInstanceParams{
				SeniorID:        input.SeniorID,
				CreatedByUserID: input.CreatedByUserID,
				Title:           input.Title,
				Description:     input.Description,
				AssignedUserID:  input.AssignedUserID,
				ScheduledFor:    *input.ScheduledFor,
			})
			if err != nil {
				return err
			}
			instance = created

			return recordTaskCreated(
				ctx, events, created.SeniorID, created.ID, created.Title, input.CreatedByUserID)
		})
		if err != nil {
			return Created{}, err
		}
		return Created{Instances: []Instance{instance}}, nil
	}

	// A recurring task is created once and produces occurrences afterwards, so
	// the event is about the template: the circle decided on a routine, which
	// is the thing that happened. The occurrences it generates are not each a
	// separate event — nobody did anything when tomorrow's row was written, and
	// a timeline full of them would bury the actions people actually took
	// (plans/phase7.md §2, "do not create events for every database update").
	var template Template
	err := s.events.InTransaction(ctx, func(tx pgx.Tx, events *careevents.Repository) error {
		created, err := s.tasks.WithTx(tx).CreateTemplate(ctx, CreateTemplateParams{
			SeniorID:        input.SeniorID,
			CreatedByUserID: input.CreatedByUserID,
			Title:           input.Title,
			Description:     input.Description,
			AssignedUserID:  input.AssignedUserID,
			Recurrence:      *input.Recurrence,
			DueTime:         *input.DueTime,
		})
		if err != nil {
			return err
		}
		template = created

		return recordTaskCreated(
			ctx, events, created.SeniorID, created.ID, created.Title, input.CreatedByUserID)
	})
	if err != nil {
		return Created{}, err
	}

	// Write the first stretch of occurrences straight away, so the task appears
	// on today's list without waiting for somebody to browse a date range.
	senior, err := s.seniors.GetByID(ctx, input.SeniorID)
	if err != nil {
		return Created{}, err
	}
	if err := s.materialise(ctx, senior, []Template{template}, now, now.Add(horizon)); err != nil {
		return Created{}, err
	}

	instances, err := s.tasks.ListWindow(ctx, input.SeniorID, now, now.Add(horizon))
	if err != nil {
		return Created{}, err
	}

	mine := make([]Instance, 0, 8)
	for _, instance := range instances {
		if instance.TemplateID != nil && *instance.TemplateID == template.ID {
			mine = append(mine, instance)
		}
	}

	return Created{Template: &template, Instances: mine}, nil
}

// checkAssignee refuses an assignee who cannot act for this senior.
//
// The membership must exist and be active, so a revoked caregiver cannot be
// handed new work and a stranger's ID cannot be attached to a circle's tasks
// (plans/phase4.md §17).
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

// Scope names one of the task views the app asks for.
type Scope string

const (
	// ScopeToday is the senior's own calendar day.
	ScopeToday Scope = "today"
	// ScopeUpcoming is the week after today.
	ScopeUpcoming Scope = "upcoming"
	// ScopeOverdue is everything past due and still waiting, however old.
	ScopeOverdue Scope = "overdue"
	// ScopeWindow is an explicit date range.
	ScopeWindow Scope = "window"
)

// upcomingDays is how far "upcoming" looks ahead.
const upcomingDays = 7

// ListInput asks for a senior's tasks.
type ListInput struct {
	SeniorID uuid.UUID
	Scope    Scope
	// From and To bound ScopeWindow, as instants.
	From time.Time
	To   time.Time
}

// List returns a senior's task occurrences for the requested view.
//
// Occurrences are written for the window before it is read. That is a write on
// a read path, and it is deliberate: the alternative is a scheduler that must
// be running for the data to be correct, and a care schedule that silently
// stops when a background job dies is worse than one that costs an insert to
// look at. The unique constraint on (template_id, scheduled_for) makes the
// write idempotent and safe under concurrency (plans/phase4.md §8).
func (s *Service) List(ctx context.Context, input ListInput, now time.Time) ([]Instance, error) {
	senior, err := s.seniors.GetByID(ctx, input.SeniorID)
	if err != nil {
		return nil, err
	}

	if input.Scope == ScopeOverdue {
		// Nothing to materialise: an occurrence cannot become overdue unless it
		// already exists.
		return s.tasks.ListOverdue(ctx, input.SeniorID, now, maxOverdue)
	}

	from, to, err := s.window(input, senior.Location(), now)
	if err != nil {
		return nil, err
	}

	templates, err := s.tasks.ListActiveTemplates(ctx, input.SeniorID)
	if err != nil {
		return nil, err
	}
	if err := s.materialise(ctx, senior, templates, from, to); err != nil {
		return nil, err
	}

	return s.tasks.ListWindow(ctx, input.SeniorID, from, to)
}

// window turns a scope into an instant range in the senior's own day.
func (s *Service) window(input ListInput, location *time.Location, now time.Time) (from, to time.Time, err error) {
	local := now.In(location)
	year, month, day := local.Date()
	startOfToday := time.Date(year, month, day, 0, 0, 0, 0, location)

	switch input.Scope {
	case ScopeToday:
		return startOfToday, startOfToday.AddDate(0, 0, 1), nil

	case ScopeUpcoming:
		start := startOfToday.AddDate(0, 0, 1)
		return start, start.AddDate(0, 0, upcomingDays), nil

	case ScopeWindow:
		if !input.To.After(input.From) {
			return time.Time{}, time.Time{}, fmt.Errorf("%w: it must end after it starts", ErrBadWindow)
		}
		if input.To.Sub(input.From) > maxWindowDays*24*time.Hour {
			return time.Time{}, time.Time{}, fmt.Errorf("%w: longer than %d days", ErrBadWindow, maxWindowDays)
		}
		return input.From, input.To, nil

	default:
		return time.Time{}, time.Time{}, fmt.Errorf("%w: unknown scope %q", ErrBadWindow, input.Scope)
	}
}

// materialise writes each template's occurrences for the window.
//
// Generation starts no earlier than the template's own creation: asking for
// last month must not invent a month of tasks that nobody was ever expected to
// do, and then report them all overdue.
func (s *Service) materialise(
	ctx context.Context,
	senior seniors.Senior,
	templates []Template,
	from, to time.Time,
) error {
	location := senior.Location()

	for _, template := range templates {
		if !template.Active {
			continue
		}

		start := from
		if template.CreatedAt.After(start) {
			start = template.CreatedAt
		}
		if !to.After(start) {
			continue
		}

		occurrences := template.Recurrence.Occurrences(template.DueTime, location, start, to)
		if err := s.tasks.Materialise(ctx, template, occurrences); err != nil {
			return err
		}
	}
	return nil
}

// ListAssigned returns the caller's own pending tasks across every circle they
// belong to, from now until the end of the window.
func (s *Service) ListAssigned(ctx context.Context, userID uuid.UUID, now time.Time, days int) ([]Instance, error) {
	if days <= 0 || days > maxWindowDays {
		days = upcomingDays
	}
	// Reaches back as well as forward, so work that is already late does not
	// drop off the one screen most likely to catch it.
	return s.tasks.ListAssignedToUser(ctx, userID, now.AddDate(0, 0, -maxWindowDays), now.AddDate(0, 0, days))
}

// Get loads one occurrence. The caller's access to its senior is checked by the
// handler, which cannot know the senior until this has run.
func (s *Service) Get(ctx context.Context, instanceID uuid.UUID) (Instance, error) {
	return s.tasks.GetInstance(ctx, instanceID)
}

// GetTemplate loads one template.
func (s *Service) GetTemplate(ctx context.Context, templateID uuid.UUID) (Template, error) {
	return s.tasks.GetTemplate(ctx, templateID)
}

// ListTemplates returns a senior's live recurring tasks.
func (s *Service) ListTemplates(ctx context.Context, seniorID uuid.UUID) ([]Template, error) {
	return s.tasks.ListActiveTemplates(ctx, seniorID)
}

// ActInput records an action on one occurrence.
type ActInput struct {
	InstanceID uuid.UUID
	Action     Action
	// ActorID is the authenticated caller. It is never read from the request
	// body: a client that could name the actor could attribute somebody else's
	// care to them (plans/phase4.md §10).
	ActorID uuid.UUID
	Notes   *string
}

// ActResult reports what happened, including whether the action had already
// been recorded.
type ActResult struct {
	Instance Instance
	// Repeat is true when the task was already in the requested state, which
	// makes a retried mutation a success rather than a conflict.
	Repeat bool
}

// Act completes, skips, or cancels one occurrence.
//
// The conditional UPDATE in the repository is the real guard. When it matches
// no row the task is no longer pending, and this re-reads it to decide between
// two very different answers: the same action arriving twice, which succeeds
// quietly, and a different outcome already recorded, which is refused so the
// server's version stands (plans/phase4.md §§27–28).
func (s *Service) Act(ctx context.Context, input ActInput) (ActResult, error) {
	var result ActResult

	err := s.events.InTransaction(ctx, func(tx pgx.Tx, events *careevents.Repository) error {
		tasks := s.tasks.WithTx(tx)

		instance, err := tasks.Act(ctx, ActParams{
			InstanceID: input.InstanceID,
			Action:     input.Action,
			ActorID:    input.ActorID,
			Notes:      input.Notes,
		})
		if err == nil {
			result = ActResult{Instance: instance}
			return recordTaskAction(ctx, events, instance, input.Action, input.ActorID)
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}

		current, err := tasks.GetInstance(ctx, input.InstanceID)
		if err != nil {
			return err
		}

		_, repeat, err := Transition(current.Status, input.Action)
		if err != nil {
			return err
		}

		// A repeat writes no second event. The state machine already decided
		// that nothing happened this time, and that decision is exactly the
		// idempotency the timeline needs: a completion replayed by the offline
		// queue must not appear twice in the record (plans/phase7.md §20).
		result = ActResult{Instance: current, Repeat: repeat}
		return nil
	})
	if err != nil {
		return ActResult{}, err
	}
	return result, nil
}

func recordTaskCreated(
	ctx context.Context,
	events *careevents.Repository,
	seniorID, entityID uuid.UUID,
	title string,
	actorID uuid.UUID,
) error {
	_, err := events.Record(ctx, careevents.RecordParams{
		SeniorID:    seniorID,
		ActorUserID: &actorID,
		Type:        careevents.TypeTaskCreated,
		EntityType:  careevents.EntityTask,
		EntityID:    entityID,
		Metadata:    careevents.Metadata{careevents.MetaTaskTitle: title},
	})
	return err
}

// eventTypeFor maps a task action to the event it produces.
var eventTypeFor = map[Action]careevents.Type{
	ActionComplete: careevents.TypeTaskCompleted,
	ActionSkip:     careevents.TypeTaskSkipped,
}

func recordTaskAction(
	ctx context.Context,
	events *careevents.Repository,
	instance Instance,
	action Action,
	actorID uuid.UUID,
) error {
	eventType, ok := eventTypeFor[action]
	if !ok {
		return fmt.Errorf("no care event defined for task action %q", action)
	}

	_, err := events.Record(ctx, careevents.RecordParams{
		SeniorID:    instance.SeniorID,
		ActorUserID: &actorID,
		Type:        eventType,
		EntityType:  careevents.EntityTask,
		EntityID:    instance.ID,
		// The title as it reads now, copied rather than referenced: renaming
		// the task next month must not rewrite what this entry says happened
		// (plans/phase7.md §5).
		Metadata: careevents.Metadata{careevents.MetaTaskTitle: instance.Title},
	})
	return err
}

// UpdateInstanceInput edits a single occurrence.
type UpdateInstanceInput struct {
	InstanceID     uuid.UUID
	SeniorID       uuid.UUID
	Title          *string
	Description    *string
	ScheduledFor   *time.Time
	AssignedUserID *uuid.UUID
	ClearAssignee  bool
}

// UpdateInstance edits one occurrence, leaving the template and every other
// occurrence untouched — changing today's time must not move tomorrow's.
func (s *Service) UpdateInstance(ctx context.Context, input UpdateInstanceInput) (Instance, error) {
	if err := s.checkAssignee(ctx, input.SeniorID, input.AssignedUserID); err != nil {
		return Instance{}, err
	}

	return s.tasks.UpdateInstance(ctx, input.InstanceID, UpdateInstanceParams{
		Title:          input.Title,
		Description:    input.Description,
		ScheduledFor:   input.ScheduledFor,
		AssignedUserID: input.AssignedUserID,
		ClearAssignee:  input.ClearAssignee,
	})
}

// UpdateTemplateInput edits a recurring task.
type UpdateTemplateInput struct {
	TemplateID     uuid.UUID
	SeniorID       uuid.UUID
	Title          *string
	Description    *string
	Recurrence     *Recurrence
	DueTime        *TimeOfDay
	Active         *bool
	AssignedUserID *uuid.UUID
	ClearAssignee  bool
}

// UpdateTemplate edits a recurring task and regenerates what has not happened
// yet.
//
// Occurrences that are already due, or that anybody has completed, skipped, or
// cancelled, are left exactly as they were. Only future pending rows are
// discarded and rebuilt from the new rule, so changing "every day" to "weekdays"
// clears next Saturday without touching last Saturday (plans/phase4.md §18).
func (s *Service) UpdateTemplate(ctx context.Context, input UpdateTemplateInput, now time.Time) (Template, error) {
	if err := s.checkAssignee(ctx, input.SeniorID, input.AssignedUserID); err != nil {
		return Template{}, err
	}

	template, err := s.tasks.UpdateTemplate(ctx, input.TemplateID, UpdateTemplateParams{
		Title:          input.Title,
		Description:    input.Description,
		Recurrence:     input.Recurrence,
		DueTime:        input.DueTime,
		Active:         input.Active,
		AssignedUserID: input.AssignedUserID,
		ClearAssignee:  input.ClearAssignee,
	})
	if err != nil {
		return Template{}, err
	}

	if _, err := s.tasks.DiscardFuturePending(ctx, template.ID, now); err != nil {
		return Template{}, err
	}

	// A deactivated template produces nothing further; its history remains.
	if !template.Active {
		return template, nil
	}

	senior, err := s.seniors.GetByID(ctx, template.SeniorID)
	if err != nil {
		return Template{}, err
	}
	if err := s.materialise(ctx, senior, []Template{template}, now, now.Add(horizon)); err != nil {
		return Template{}, err
	}

	return template, nil
}
