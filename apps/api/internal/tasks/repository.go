package tasks

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/genxcare/api/internal/database"
)

// ErrNotFound is returned when no task matches the lookup.
var ErrNotFound = errors.New("task not found")

// Repository reads and writes care tasks.
type Repository struct {
	db database.Querier
}

// NewRepository builds a Repository over the shared pool.
func NewRepository(pool *database.Pool) *Repository {
	return &Repository{db: pool}
}

// due_time is read as text rather than a driver time type: it is a wall-clock
// time, and "09:00" is the whole of what it means.
const templateColumns = `id, senior_id, created_by_user_id, title, description, assigned_user_id,
	recurrence_rule, to_char(due_time, 'HH24:MI') AS due_time, active, created_at, updated_at`

const instanceColumns = `id, template_id, senior_id, created_by_user_id, title, description,
	assigned_user_id, scheduled_for, status, completed_at, completed_by, skipped_at, skipped_by,
	notes, created_at, updated_at`

// The same list qualified for the one query that joins. Written out rather than
// derived from the constant above: a string-rewriting helper would be one more
// thing to be wrong, and the compiler cannot check what it produces.
const instanceColumnsAliased = `i.id, i.template_id, i.senior_id, i.created_by_user_id, i.title,
	i.description, i.assigned_user_id, i.scheduled_for, i.status, i.completed_at, i.completed_by,
	i.skipped_at, i.skipped_by, i.notes, i.created_at, i.updated_at`

// CreateTemplateParams describes a new recurring task. The caller has already
// authorized the creator and validated the assignee.
type CreateTemplateParams struct {
	SeniorID        uuid.UUID
	CreatedByUserID uuid.UUID
	Title           string
	Description     string
	AssignedUserID  *uuid.UUID
	Recurrence      Recurrence
	DueTime         TimeOfDay
}

// CreateTemplate inserts a recurring task definition.
func (r *Repository) CreateTemplate(ctx context.Context, params CreateTemplateParams) (Template, error) {
	template, err := scanTemplate(r.db.QueryRow(ctx, `
		INSERT INTO care_task_templates (
			senior_id, created_by_user_id, title, description, assigned_user_id,
			recurrence_rule, due_time
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7::time)
		RETURNING `+templateColumns,
		params.SeniorID,
		params.CreatedByUserID,
		params.Title,
		params.Description,
		params.AssignedUserID,
		params.Recurrence.String(),
		params.DueTime.String(),
	))
	if err != nil {
		return Template{}, fmt.Errorf("create task template: %w", err)
	}
	return template, nil
}

// GetTemplate loads one template.
func (r *Repository) GetTemplate(ctx context.Context, id uuid.UUID) (Template, error) {
	template, err := scanTemplate(r.db.QueryRow(ctx,
		`SELECT `+templateColumns+` FROM care_task_templates WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Template{}, ErrNotFound
	}
	if err != nil {
		return Template{}, fmt.Errorf("get task template: %w", err)
	}
	return template, nil
}

// ListActiveTemplates returns a senior's live recurring tasks.
func (r *Repository) ListActiveTemplates(ctx context.Context, seniorID uuid.UUID) ([]Template, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+templateColumns+`
		 FROM care_task_templates
		 WHERE senior_id = $1 AND active
		 ORDER BY due_time, created_at`, seniorID)
	if err != nil {
		return nil, fmt.Errorf("list task templates: %w", err)
	}
	defer rows.Close()

	templates := make([]Template, 0)
	for rows.Next() {
		template, err := scanTemplate(rows)
		if err != nil {
			return nil, fmt.Errorf("scan task template: %w", err)
		}
		templates = append(templates, template)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read task templates: %w", err)
	}
	return templates, nil
}

// UpdateTemplateParams carries the fields an edit may change. A nil field is
// left as it is.
type UpdateTemplateParams struct {
	Title       *string
	Description *string
	Recurrence  *Recurrence
	DueTime     *TimeOfDay
	Active      *bool

	// AssignedUserID changes the assignee; ClearAssignee removes one.
	AssignedUserID *uuid.UUID
	ClearAssignee  bool
}

// UpdateTemplate applies an edit.
//
// COALESCE keeps every untouched column at its current value, so a partial
// edit cannot blank a field the client did not mention.
func (r *Repository) UpdateTemplate(
	ctx context.Context,
	id uuid.UUID,
	params UpdateTemplateParams,
) (Template, error) {
	var recurrence *string
	if params.Recurrence != nil {
		rule := params.Recurrence.String()
		recurrence = &rule
	}

	var dueTime *string
	if params.DueTime != nil {
		at := params.DueTime.String()
		dueTime = &at
	}

	template, err := scanTemplate(r.db.QueryRow(ctx, `
		UPDATE care_task_templates
		SET title           = COALESCE($2, title),
		    description     = COALESCE($3, description),
		    recurrence_rule = COALESCE($4, recurrence_rule),
		    due_time        = COALESCE($5::time, due_time),
		    active          = COALESCE($6, active),
		    assigned_user_id = CASE
		        WHEN $7 THEN NULL
		        ELSE COALESCE($8, assigned_user_id)
		    END
		WHERE id = $1
		RETURNING `+templateColumns,
		id,
		params.Title,
		params.Description,
		recurrence,
		dueTime,
		params.Active,
		params.ClearAssignee,
		params.AssignedUserID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Template{}, ErrNotFound
	}
	if err != nil {
		return Template{}, fmt.Errorf("update task template: %w", err)
	}
	return template, nil
}

// CreateInstanceParams describes a one-time task.
type CreateInstanceParams struct {
	SeniorID        uuid.UUID
	CreatedByUserID uuid.UUID
	Title           string
	Description     string
	AssignedUserID  *uuid.UUID
	ScheduledFor    time.Time
}

// CreateInstance inserts a one-time task: an occurrence with no template.
func (r *Repository) CreateInstance(ctx context.Context, params CreateInstanceParams) (Instance, error) {
	instance, err := scanInstance(r.db.QueryRow(ctx, `
		INSERT INTO care_task_instances (
			senior_id, created_by_user_id, title, description, assigned_user_id, scheduled_for
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+instanceColumns,
		params.SeniorID,
		params.CreatedByUserID,
		params.Title,
		params.Description,
		params.AssignedUserID,
		params.ScheduledFor,
	))
	if err != nil {
		return Instance{}, fmt.Errorf("create task: %w", err)
	}
	return instance, nil
}

// Materialise writes the occurrences a template produces in a window.
//
// ON CONFLICT DO NOTHING against the one-per-occurrence unique constraint is
// what makes this safe to run repeatedly and concurrently: two readers asking
// for the same week produce the same rows, and the second insert is discarded
// rather than duplicating somebody's care.
func (r *Repository) Materialise(
	ctx context.Context,
	template Template,
	occurrences []time.Time,
) error {
	if len(occurrences) == 0 {
		return nil
	}

	_, err := r.db.Exec(ctx, `
		INSERT INTO care_task_instances (
			template_id, senior_id, created_by_user_id, title, description,
			assigned_user_id, scheduled_for
		)
		SELECT $1, $2, $3, $4, $5, $6, occurrence
		FROM unnest($7::timestamptz[]) AS occurrence
		ON CONFLICT (template_id, scheduled_for) DO NOTHING`,
		template.ID,
		template.SeniorID,
		template.CreatedByUserID,
		template.Title,
		template.Description,
		template.AssignedUserID,
		occurrences,
	)
	if err != nil {
		return fmt.Errorf("materialise task occurrences: %w", err)
	}
	return nil
}

// DiscardFuturePending removes a template's not-yet-due, untouched occurrences.
//
// Called when a template is edited, so the new rule takes effect going forward.
// The WHERE clause is what keeps history intact: anything already completed,
// skipped, or cancelled is left exactly as it was, and so is anything already
// due — a task somebody may be looking at right now does not vanish from under
// them (plans/phase4.md §18).
func (r *Repository) DiscardFuturePending(
	ctx context.Context,
	templateID uuid.UUID,
	after time.Time,
) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM care_task_instances
		WHERE template_id = $1 AND status = 'pending' AND scheduled_for > $2`,
		templateID, after)
	if err != nil {
		return 0, fmt.Errorf("discard future occurrences: %w", err)
	}
	return tag.RowsAffected(), nil
}

// GetInstance loads one occurrence.
func (r *Repository) GetInstance(ctx context.Context, id uuid.UUID) (Instance, error) {
	instance, err := scanInstance(r.db.QueryRow(ctx,
		`SELECT `+instanceColumns+` FROM care_task_instances WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Instance{}, ErrNotFound
	}
	if err != nil {
		return Instance{}, fmt.Errorf("get task: %w", err)
	}
	return instance, nil
}

// ListWindow returns a senior's occurrences due within [from, to).
func (r *Repository) ListWindow(
	ctx context.Context,
	seniorID uuid.UUID,
	from, to time.Time,
) ([]Instance, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+instanceColumns+`
		 FROM care_task_instances
		 WHERE senior_id = $1 AND scheduled_for >= $2 AND scheduled_for < $3
		 ORDER BY scheduled_for, created_at`,
		seniorID, from, to)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	return collectInstances(rows)
}

// ListOverdue returns a senior's occurrences that are past due and still
// pending, oldest first.
func (r *Repository) ListOverdue(
	ctx context.Context,
	seniorID uuid.UUID,
	now time.Time,
	limit int,
) ([]Instance, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+instanceColumns+`
		 FROM care_task_instances
		 WHERE senior_id = $1 AND status = 'pending' AND scheduled_for < $2
		 ORDER BY scheduled_for
		 LIMIT $3`,
		seniorID, now, limit)
	if err != nil {
		return nil, fmt.Errorf("list overdue tasks: %w", err)
	}
	defer rows.Close()

	return collectInstances(rows)
}

// ListAssignedToUser returns the pending tasks assigned to one user, across
// every circle they belong to.
//
// The join is the authorization: a task only appears if the user still holds an
// active relationship carrying tasks.view for that senior. Revoking somebody's
// membership therefore empties their task list immediately, without any
// reassignment sweep having to run first.
func (r *Repository) ListAssignedToUser(
	ctx context.Context,
	userID uuid.UUID,
	from, to time.Time,
) ([]Instance, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+instanceColumnsAliased+`
		 FROM care_task_instances i
		 JOIN care_relationships cr
		   ON cr.senior_id = i.senior_id
		  AND cr.user_id = $1
		  AND cr.status = 'active'
		  AND 'tasks.view' = ANY (cr.permissions)
		 WHERE i.assigned_user_id = $1
		   AND i.status = 'pending'
		   AND i.scheduled_for >= $2
		   AND i.scheduled_for < $3
		 ORDER BY i.scheduled_for`,
		userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("list assigned tasks: %w", err)
	}
	defer rows.Close()

	return collectInstances(rows)
}

// ActParams records who acted on a task and what they said about it.
type ActParams struct {
	InstanceID uuid.UUID
	Action     Action
	ActorID    uuid.UUID
	Notes      *string
}

// Act applies a completion, skip, or cancellation.
//
// The update requires the task to still be pending, so concurrent actions from
// two devices resolve in the database: exactly one updates a row, and the other
// is told what actually happened rather than overwriting it
// (plans/phase4.md §28).
//
// ErrNotFound here means "not pending any more", which the service
// distinguishes from "no such task" by re-reading the row.
func (r *Repository) Act(ctx context.Context, params ActParams) (Instance, error) {
	status := resultOf[params.Action]

	instance, err := scanInstance(r.db.QueryRow(ctx, `
		UPDATE care_task_instances
		SET status       = $2,
		    completed_at = CASE WHEN $2 = 'completed' THEN now() ELSE completed_at END,
		    completed_by = CASE WHEN $2 = 'completed' THEN $3 ELSE completed_by END,
		    skipped_at   = CASE WHEN $2 = 'skipped'   THEN now() ELSE skipped_at END,
		    skipped_by   = CASE WHEN $2 = 'skipped'   THEN $3 ELSE skipped_by END,
		    notes        = COALESCE($4, notes)
		WHERE id = $1 AND status = 'pending'
		RETURNING `+instanceColumns,
		params.InstanceID,
		string(status),
		params.ActorID,
		params.Notes,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Instance{}, ErrNotFound
	}
	if err != nil {
		return Instance{}, fmt.Errorf("act on task: %w", err)
	}
	return instance, nil
}

// UpdateInstanceParams carries an edit to a single occurrence.
type UpdateInstanceParams struct {
	Title          *string
	Description    *string
	ScheduledFor   *time.Time
	AssignedUserID *uuid.UUID
	ClearAssignee  bool
}

// UpdateInstance edits one occurrence, leaving its template alone.
func (r *Repository) UpdateInstance(
	ctx context.Context,
	id uuid.UUID,
	params UpdateInstanceParams,
) (Instance, error) {
	instance, err := scanInstance(r.db.QueryRow(ctx, `
		UPDATE care_task_instances
		SET title         = COALESCE($2, title),
		    description   = COALESCE($3, description),
		    scheduled_for = COALESCE($4, scheduled_for),
		    assigned_user_id = CASE
		        WHEN $5 THEN NULL
		        ELSE COALESCE($6, assigned_user_id)
		    END
		WHERE id = $1
		RETURNING `+instanceColumns,
		id,
		params.Title,
		params.Description,
		params.ScheduledFor,
		params.ClearAssignee,
		params.AssignedUserID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Instance{}, ErrNotFound
	}
	if err != nil {
		return Instance{}, fmt.Errorf("update task: %w", err)
	}
	return instance, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanTemplate(row rowScanner) (Template, error) {
	var (
		template   Template
		rule       string
		dueTime    string
		assignedTo *uuid.UUID
	)

	err := row.Scan(
		&template.ID,
		&template.SeniorID,
		&template.CreatedByUserID,
		&template.Title,
		&template.Description,
		&assignedTo,
		&rule,
		&dueTime,
		&template.Active,
		&template.CreatedAt,
		&template.UpdatedAt,
	)
	if err != nil {
		return Template{}, err
	}

	// A rule or time that will not parse means the row was written by
	// something other than this code. Failing here is better than scheduling
	// care from a value nobody can interpret.
	recurrence, err := ParseRecurrence(rule)
	if err != nil {
		return Template{}, fmt.Errorf("template %s: %w", template.ID, err)
	}
	at, err := ParseTimeOfDay(dueTime)
	if err != nil {
		return Template{}, fmt.Errorf("template %s: %w", template.ID, err)
	}

	template.AssignedUserID = assignedTo
	template.Recurrence = recurrence
	template.DueTime = at
	return template, nil
}

func scanInstance(row rowScanner) (Instance, error) {
	var (
		instance Instance
		status   string
	)

	err := row.Scan(
		&instance.ID,
		&instance.TemplateID,
		&instance.SeniorID,
		&instance.CreatedByUserID,
		&instance.Title,
		&instance.Description,
		&instance.AssignedUserID,
		&instance.ScheduledFor,
		&status,
		&instance.CompletedAt,
		&instance.CompletedBy,
		&instance.SkippedAt,
		&instance.SkippedBy,
		&instance.Notes,
		&instance.CreatedAt,
		&instance.UpdatedAt,
	)
	if err != nil {
		return Instance{}, err
	}

	instance.Status = Status(status)
	return instance, nil
}

func collectInstances(rows pgx.Rows) ([]Instance, error) {
	found := make([]Instance, 0)
	for rows.Next() {
		instance, err := scanInstance(rows)
		if err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		found = append(found, instance)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read tasks: %w", err)
	}
	return found, nil
}

// WithTx returns a repository bound to tx, so a task change and the care event
// describing it are written through the same connection and commit together
// (plans/phase7.md §26).
func (r *Repository) WithTx(tx pgx.Tx) *Repository {
	return &Repository{db: tx}
}
