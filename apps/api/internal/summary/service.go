// Package summary answers one question for a whole care circle at once: does
// anyone need me right now.
//
// It lives outside the domain packages because it composes them. `tasks` and
// `medications` both import `seniors`, so `seniors` cannot import them back —
// and a caller that had to ask each domain separately, per senior, would make
// one request per row from a phone.
package summary

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/meracare/api/internal/auth"
	"github.com/meracare/api/internal/care"
	"github.com/meracare/api/internal/medications"
	"github.com/meracare/api/internal/seniors"
	"github.com/meracare/api/internal/tasks"
)

// SeniorLister returns the circles the caller belongs to.
type SeniorLister interface {
	List(ctx context.Context, principal auth.Principal) ([]seniors.Membership, error)
}

// TaskLister returns a senior's task occurrences for a view.
type TaskLister interface {
	List(ctx context.Context, input tasks.ListInput, now time.Time) ([]tasks.Instance, error)
}

// DoseLister returns a senior's medication doses for a view.
type DoseLister interface {
	ListDoses(
		ctx context.Context,
		input medications.ListDosesInput,
		now time.Time,
	) ([]medications.Instance, error)
}

// Counts is how much of something is finished out of how much there is.
type Counts struct {
	Done  int
	Total int
}

// Summary is one senior's day, as far as this caller is allowed to see it.
//
// Medications and Tasks are nil rather than zero when the caller lacks the
// permission to view them. The distinction matters: zero doses today is a fact
// about the senior, whereas no permission is a fact about the reader, and
// flattening them would tell a restricted caregiver that a senior takes no
// medication when they may take several.
type Summary struct {
	SeniorID       uuid.UUID
	Medications    *Counts
	Tasks          *Counts
	NeedsAttention int
}

// Service composes today's counts across the caller's circles.
type Service struct {
	seniors SeniorLister
	tasks   TaskLister
	doses   DoseLister
}

// NewService builds the service.
func NewService(seniorLister SeniorLister, taskLister TaskLister, doseLister DoseLister) *Service {
	return &Service{seniors: seniorLister, tasks: taskLister, doses: doseLister}
}

// ForUser returns today's counts for every senior the caller can reach.
//
// Each senior costs two domain calls, and both of those materialise the day's
// occurrences before counting them — the same write-on-read the list endpoints
// perform. Counting rows directly would be cheaper and wrong: an occurrence
// that nobody has looked at yet does not exist to be counted.
//
// A senior whose data cannot be read is reported with zero counts rather than
// dropped. A card that vanishes reads as a loading failure, and the rest of the
// circle is still worth showing.
func (s *Service) ForUser(
	ctx context.Context,
	principal auth.Principal,
	now time.Time,
) ([]Summary, error) {
	memberships, err := s.seniors.List(ctx, principal)
	if err != nil {
		return nil, err
	}

	summaries := make([]Summary, 0, len(memberships))
	for _, membership := range memberships {
		summaries = append(summaries, s.forSenior(ctx, membership, now))
	}

	return summaries, nil
}

func (s *Service) forSenior(
	ctx context.Context,
	membership seniors.Membership,
	now time.Time,
) Summary {
	result := Summary{SeniorID: membership.Senior.ID}

	if membership.Relationship.Can(care.PermissionTasksView) {
		done, total, late := s.countTasks(ctx, membership.Senior.ID, now)
		result.Tasks = &Counts{Done: done, Total: total}
		result.NeedsAttention += late
	}

	if membership.Relationship.Can(care.PermissionMedicationsView) {
		done, total, late := s.countDoses(ctx, membership.Senior.ID, now)
		result.Medications = &Counts{Done: done, Total: total}
		result.NeedsAttention += late
	}

	return result
}

// countTasks returns today's finished, total, and overdue task counts.
//
// A read failure yields zeroes rather than an error: one senior's tasks being
// briefly unreadable should not blank the whole circle.
func (s *Service) countTasks(
	ctx context.Context,
	seniorID uuid.UUID,
	now time.Time,
) (done, total, late int) {
	instances, err := s.tasks.List(ctx, tasks.ListInput{SeniorID: seniorID, Scope: tasks.ScopeToday}, now)
	if err != nil {
		return 0, 0, 0
	}

	for _, instance := range instances {
		total++
		if instance.Status.Settled() {
			done++
		}
		if instance.Overdue(now) {
			late++
		}
	}

	return done, total, late
}

// countDoses returns today's taken, total, and missed dose counts.
func (s *Service) countDoses(
	ctx context.Context,
	seniorID uuid.UUID,
	now time.Time,
) (done, total, late int) {
	instances, err := s.doses.ListDoses(
		ctx,
		medications.ListDosesInput{SeniorID: seniorID, Scope: medications.ScopeToday},
		now,
	)
	if err != nil {
		return 0, 0, 0
	}

	for _, instance := range instances {
		total++
		if instance.Status == medications.StatusTaken {
			done++
		}
		if instance.Missed(now) {
			late++
		}
	}

	return done, total, late
}
