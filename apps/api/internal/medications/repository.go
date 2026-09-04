package medications

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/genxcare/api/internal/database"
	"github.com/genxcare/api/internal/paging"
	"github.com/genxcare/api/internal/recurrence"
)

// ErrNotFound is returned when no medication, schedule or dose matches.
var ErrNotFound = errors.New("medication not found")

// Repository reads and writes medications, their schedules, and their doses.
type Repository struct {
	db database.Querier
}

// NewRepository builds a Repository over the shared pool.
func NewRepository(pool *database.Pool) *Repository {
	return &Repository{db: pool}
}

const medicationColumns = `id, senior_id, created_by_user_id, name, dosage, form,
	instructions, notes, active, created_at, updated_at`

// scheduled_time is read as text rather than a driver time type: it is a
// wall-clock time, and "08:00" is the whole of what it means.
const scheduleColumns = `id, medication_id, senior_id, recurrence_rule,
	to_char(scheduled_time, 'HH24:MI') AS scheduled_time, active, created_at, updated_at`

// The same list qualified for the queries that join to medications. Written out
// rather than derived from the constant above: a string-rewriting helper would
// be one more thing to be wrong, and the compiler cannot check what it produces.
const scheduleColumnsAliased = `s.id, s.medication_id, s.senior_id, s.recurrence_rule,
	to_char(s.scheduled_time, 'HH24:MI') AS scheduled_time, s.active, s.created_at, s.updated_at`

const instanceColumns = `id, schedule_id, medication_id, senior_id, created_by_user_id,
	name, dosage, scheduled_for, status, taken_at, taken_by, skipped_at, skipped_by,
	notes, created_at, updated_at`

// --- Medications -----------------------------------------------------------

// CreateMedicationParams describes a new medication. The caller has already
// authorized the creator.
type CreateMedicationParams struct {
	SeniorID        uuid.UUID
	CreatedByUserID uuid.UUID
	Name            string
	Dosage          string
	Form            Form
	Instructions    string
	Notes           string
}

// CreateMedication inserts a medication.
func (r *Repository) CreateMedication(
	ctx context.Context,
	params CreateMedicationParams,
) (Medication, error) {
	medication, err := scanMedication(r.db.QueryRow(ctx, `
		INSERT INTO medications (
			senior_id, created_by_user_id, name, dosage, form, instructions, notes
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+medicationColumns,
		params.SeniorID,
		params.CreatedByUserID,
		params.Name,
		params.Dosage,
		optionalForm(params.Form),
		params.Instructions,
		params.Notes,
	))
	if err != nil {
		return Medication{}, fmt.Errorf("create medication: %w", err)
	}
	return medication, nil
}

// GetMedication loads one medication.
func (r *Repository) GetMedication(ctx context.Context, id uuid.UUID) (Medication, error) {
	medication, err := scanMedication(r.db.QueryRow(ctx,
		`SELECT `+medicationColumns+` FROM medications WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Medication{}, ErrNotFound
	}
	if err != nil {
		return Medication{}, fmt.Errorf("get medication: %w", err)
	}
	return medication, nil
}

// ListMedications returns a senior's medications, active ones first.
//
// Stopped medicines are included so the detail screen and the history remain
// reachable; the client separates them (plans/phase5.md §13).
func (r *Repository) ListMedications(
	ctx context.Context,
	seniorID uuid.UUID,
) ([]Medication, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+medicationColumns+`
		 FROM medications
		 WHERE senior_id = $1
		 ORDER BY active DESC, lower(name), created_at`, seniorID)
	if err != nil {
		return nil, fmt.Errorf("list medications: %w", err)
	}
	defer rows.Close()

	found := make([]Medication, 0)
	for rows.Next() {
		medication, err := scanMedication(rows)
		if err != nil {
			return nil, fmt.Errorf("scan medication: %w", err)
		}
		found = append(found, medication)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read medications: %w", err)
	}
	return found, nil
}

// UpdateMedicationParams carries the fields an edit may change. A nil field is
// left as it is.
type UpdateMedicationParams struct {
	Name         *string
	Dosage       *string
	Instructions *string
	Notes        *string
	Active       *bool

	// Form changes the form; ClearForm removes one.
	Form      *Form
	ClearForm bool
}

// UpdateMedication applies an edit.
//
// COALESCE keeps every untouched column at its current value, so a partial edit
// cannot blank a field the client did not mention.
func (r *Repository) UpdateMedication(
	ctx context.Context,
	id uuid.UUID,
	params UpdateMedicationParams,
) (Medication, error) {
	var form *string
	if params.Form != nil {
		value := string(*params.Form)
		form = &value
	}

	medication, err := scanMedication(r.db.QueryRow(ctx, `
		UPDATE medications
		SET name         = COALESCE($2, name),
		    dosage       = COALESCE($3, dosage),
		    instructions = COALESCE($4, instructions),
		    notes        = COALESCE($5, notes),
		    active       = COALESCE($6, active),
		    form         = CASE
		        WHEN $7 THEN NULL
		        ELSE COALESCE($8, form)
		    END
		WHERE id = $1
		RETURNING `+medicationColumns,
		id,
		params.Name,
		params.Dosage,
		params.Instructions,
		params.Notes,
		params.Active,
		params.ClearForm,
		form,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Medication{}, ErrNotFound
	}
	if err != nil {
		return Medication{}, fmt.Errorf("update medication: %w", err)
	}
	return medication, nil
}

// --- Schedules -------------------------------------------------------------

// CreateScheduleParams describes a new time of day for a medication.
type CreateScheduleParams struct {
	MedicationID  uuid.UUID
	SeniorID      uuid.UUID
	Recurrence    recurrence.Rule
	ScheduledTime recurrence.TimeOfDay
}

// ErrDuplicateSchedule is returned when the same medicine already has a live
// schedule at that time under the same rule.
var ErrDuplicateSchedule = errors.New("that time is already scheduled for this medication")

// duplicateScheduleIndex names the partial unique index that enforces it, so a
// violation of some other constraint is not reported as a duplicate time.
const duplicateScheduleIndex = "medication_schedules_no_duplicate_time_idx"

// CreateSchedule inserts a schedule.
func (r *Repository) CreateSchedule(
	ctx context.Context,
	params CreateScheduleParams,
) (Schedule, error) {
	schedule, err := scanSchedule(r.db.QueryRow(ctx, `
		INSERT INTO medication_schedules (
			medication_id, senior_id, recurrence_rule, scheduled_time
		)
		VALUES ($1, $2, $3, $4::time)
		RETURNING `+scheduleColumns,
		params.MedicationID,
		params.SeniorID,
		params.Recurrence.String(),
		params.ScheduledTime.String(),
	))
	if database.IsUniqueViolation(err, duplicateScheduleIndex) {
		return Schedule{}, ErrDuplicateSchedule
	}
	if err != nil {
		return Schedule{}, fmt.Errorf("create medication schedule: %w", err)
	}
	return schedule, nil
}

// GetSchedule loads one schedule.
func (r *Repository) GetSchedule(ctx context.Context, id uuid.UUID) (Schedule, error) {
	schedule, err := scanSchedule(r.db.QueryRow(ctx,
		`SELECT `+scheduleColumns+` FROM medication_schedules WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Schedule{}, ErrNotFound
	}
	if err != nil {
		return Schedule{}, fmt.Errorf("get medication schedule: %w", err)
	}
	return schedule, nil
}

// ListSchedules returns every schedule for one medication, live ones first.
func (r *Repository) ListSchedules(
	ctx context.Context,
	medicationID uuid.UUID,
) ([]Schedule, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+scheduleColumns+`
		 FROM medication_schedules
		 WHERE medication_id = $1
		 ORDER BY active DESC, scheduled_time`, medicationID)
	if err != nil {
		return nil, fmt.Errorf("list medication schedules: %w", err)
	}
	defer rows.Close()

	return collectSchedules(rows)
}

// Scheduled is a live schedule together with the medicine it belongs to.
//
// Doses carry the name and dosage as they read when the dose was generated, so
// generating them needs both halves at once.
type Scheduled struct {
	Schedule Schedule
	Name     string
	Dosage   string
	// CreatedByUserID is the medicine's author, carried onto each dose so the
	// row has an origin even though no person pressed anything to create it.
	CreatedByUserID uuid.UUID
}

// ListDueSchedulesForSenior returns every live schedule of every live
// medication for one senior.
//
// Both halves must be active: stopping a medicine must stop its doses without
// anybody having to deactivate each of its times as well.
func (r *Repository) ListDueSchedulesForSenior(
	ctx context.Context,
	seniorID uuid.UUID,
) ([]Scheduled, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+scheduleColumnsAliased+`, m.name, m.dosage, m.created_by_user_id
		 FROM medication_schedules s
		 JOIN medications m ON m.id = s.medication_id
		 WHERE s.senior_id = $1 AND s.active AND m.active
		 ORDER BY s.scheduled_time`, seniorID)
	if err != nil {
		return nil, fmt.Errorf("list due schedules: %w", err)
	}
	defer rows.Close()

	return collectScheduled(rows)
}

// ListDueSchedulesForMedication is the same, narrowed to one medication.
func (r *Repository) ListDueSchedulesForMedication(
	ctx context.Context,
	medicationID uuid.UUID,
) ([]Scheduled, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+scheduleColumnsAliased+`, m.name, m.dosage, m.created_by_user_id
		 FROM medication_schedules s
		 JOIN medications m ON m.id = s.medication_id
		 WHERE s.medication_id = $1 AND s.active AND m.active
		 ORDER BY s.scheduled_time`, medicationID)
	if err != nil {
		return nil, fmt.Errorf("list due schedules for medication: %w", err)
	}
	defer rows.Close()

	return collectScheduled(rows)
}

// UpdateScheduleParams carries an edit to one schedule.
type UpdateScheduleParams struct {
	Recurrence    *recurrence.Rule
	ScheduledTime *recurrence.TimeOfDay
	Active        *bool
}

// UpdateSchedule applies an edit.
func (r *Repository) UpdateSchedule(
	ctx context.Context,
	id uuid.UUID,
	params UpdateScheduleParams,
) (Schedule, error) {
	var rule *string
	if params.Recurrence != nil {
		value := params.Recurrence.String()
		rule = &value
	}

	var at *string
	if params.ScheduledTime != nil {
		value := params.ScheduledTime.String()
		at = &value
	}

	schedule, err := scanSchedule(r.db.QueryRow(ctx, `
		UPDATE medication_schedules
		SET recurrence_rule = COALESCE($2, recurrence_rule),
		    scheduled_time  = COALESCE($3::time, scheduled_time),
		    active          = COALESCE($4, active)
		WHERE id = $1
		RETURNING `+scheduleColumns,
		id,
		rule,
		at,
		params.Active,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Schedule{}, ErrNotFound
	}
	if database.IsUniqueViolation(err, duplicateScheduleIndex) {
		return Schedule{}, ErrDuplicateSchedule
	}
	if err != nil {
		return Schedule{}, fmt.Errorf("update medication schedule: %w", err)
	}
	return schedule, nil
}

// --- Doses -----------------------------------------------------------------

// CreateInstanceParams describes a one-off dose: one that no rule produced.
type CreateInstanceParams struct {
	MedicationID    uuid.UUID
	SeniorID        uuid.UUID
	CreatedByUserID uuid.UUID
	Name            string
	Dosage          string
	ScheduledFor    time.Time
}

// CreateInstance inserts a single dose with no schedule behind it.
func (r *Repository) CreateInstance(
	ctx context.Context,
	params CreateInstanceParams,
) (Instance, error) {
	instance, err := scanInstance(r.db.QueryRow(ctx, `
		INSERT INTO medication_instances (
			medication_id, senior_id, created_by_user_id, name, dosage, scheduled_for
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+instanceColumns,
		params.MedicationID,
		params.SeniorID,
		params.CreatedByUserID,
		params.Name,
		params.Dosage,
		params.ScheduledFor,
	))
	if err != nil {
		return Instance{}, fmt.Errorf("create dose: %w", err)
	}
	return instance, nil
}

// Materialise writes the doses a schedule produces in a window.
//
// ON CONFLICT DO NOTHING against the one-per-dose unique constraint is what
// makes this safe to run repeatedly and concurrently: two readers asking for
// the same day produce the same rows, and the second insert is discarded rather
// than telling somebody to take a second tablet.
func (r *Repository) Materialise(
	ctx context.Context,
	scheduled Scheduled,
	createdByUserID uuid.UUID,
	occurrences []time.Time,
) error {
	if len(occurrences) == 0 {
		return nil
	}

	_, err := r.db.Exec(ctx, `
		INSERT INTO medication_instances (
			schedule_id, medication_id, senior_id, created_by_user_id,
			name, dosage, scheduled_for
		)
		SELECT $1, $2, $3, $4, $5, $6, occurrence
		FROM unnest($7::timestamptz[]) AS occurrence
		ON CONFLICT (schedule_id, scheduled_for) DO NOTHING`,
		scheduled.Schedule.ID,
		scheduled.Schedule.MedicationID,
		scheduled.Schedule.SeniorID,
		createdByUserID,
		scheduled.Name,
		scheduled.Dosage,
		occurrences,
	)
	if err != nil {
		return fmt.Errorf("materialise doses: %w", err)
	}
	return nil
}

// DiscardFuturePending removes a schedule's not-yet-due, untouched doses.
//
// Called when a schedule is edited or stopped, so the new arrangement takes
// effect going forward. The WHERE clause is what keeps history intact: anything
// already taken or skipped is left exactly as it was, and so is anything
// already due — a dose somebody may be looking at right now does not vanish
// from under them (plans/phase5.md §12).
func (r *Repository) DiscardFuturePending(
	ctx context.Context,
	scheduleID uuid.UUID,
	after time.Time,
) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM medication_instances
		WHERE schedule_id = $1 AND status = 'pending' AND scheduled_for > $2`,
		scheduleID, after)
	if err != nil {
		return 0, fmt.Errorf("discard future doses: %w", err)
	}
	return tag.RowsAffected(), nil
}

// DiscardFuturePendingForMedication does the same for every schedule of one
// medication, which is what stopping a medicine means.
func (r *Repository) DiscardFuturePendingForMedication(
	ctx context.Context,
	medicationID uuid.UUID,
	after time.Time,
) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM medication_instances
		WHERE medication_id = $1 AND status = 'pending' AND scheduled_for > $2`,
		medicationID, after)
	if err != nil {
		return 0, fmt.Errorf("discard future doses for medication: %w", err)
	}
	return tag.RowsAffected(), nil
}

// GetInstance loads one dose.
func (r *Repository) GetInstance(ctx context.Context, id uuid.UUID) (Instance, error) {
	instance, err := scanInstance(r.db.QueryRow(ctx,
		`SELECT `+instanceColumns+` FROM medication_instances WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Instance{}, ErrNotFound
	}
	if err != nil {
		return Instance{}, fmt.Errorf("get dose: %w", err)
	}
	return instance, nil
}

// ListWindow returns a senior's doses due within [from, to).
func (r *Repository) ListWindow(
	ctx context.Context,
	seniorID uuid.UUID,
	from, to time.Time,
) ([]Instance, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+instanceColumns+`
		 FROM medication_instances
		 WHERE senior_id = $1 AND scheduled_for >= $2 AND scheduled_for < $3
		 ORDER BY scheduled_for, lower(name), id`,
		seniorID, from, to)
	if err != nil {
		return nil, fmt.Errorf("list doses: %w", err)
	}
	defer rows.Close()

	return collectInstances(rows)
}

// ListMedicationWindow returns one medication's doses due within [from, to).
func (r *Repository) ListMedicationWindow(
	ctx context.Context,
	medicationID uuid.UUID,
	from, to time.Time,
) ([]Instance, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+instanceColumns+`
		 FROM medication_instances
		 WHERE medication_id = $1 AND scheduled_for >= $2 AND scheduled_for < $3
		 ORDER BY scheduled_for, id`,
		medicationID, from, to)
	if err != nil {
		return nil, fmt.Errorf("list medication doses: %w", err)
	}
	defer rows.Close()

	return collectInstances(rows)
}

// NextPendingDose returns the earliest dose from `from` that nobody has acted
// on, or nil when there is none.
func (r *Repository) NextPendingDose(
	ctx context.Context,
	medicationID uuid.UUID,
	from time.Time,
) (*Instance, error) {
	instance, err := scanInstance(r.db.QueryRow(ctx,
		`SELECT `+instanceColumns+`
		 FROM medication_instances
		 WHERE medication_id = $1 AND status = 'pending' AND scheduled_for >= $2
		 ORDER BY scheduled_for
		 LIMIT 1`,
		medicationID, from))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("next dose: %w", err)
	}
	return &instance, nil
}

// ListMissed returns a senior's doses whose window has passed and which nobody
// acted on, oldest first.
func (r *Repository) ListMissed(
	ctx context.Context,
	seniorID uuid.UUID,
	before time.Time,
	limit int,
) ([]Instance, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+instanceColumns+`
		 FROM medication_instances
		 WHERE senior_id = $1 AND status = 'pending' AND scheduled_for < $2
		 ORDER BY scheduled_for
		 LIMIT $3`,
		seniorID, before, limit)
	if err != nil {
		return nil, fmt.Errorf("list missed doses: %w", err)
	}
	defer rows.Close()

	return collectInstances(rows)
}

// HistoryPage is one page of a medication's doses, newest first.
type HistoryPage struct {
	Items []Instance
	// NextCursor is empty when the history is exhausted.
	NextCursor string
}

// ListHistory returns one page of a medication's doses, newest first.
//
// Keyset rather than offset pagination: a long-running medication accumulates
// thousands of doses, and OFFSET makes the server count past every one of them
// to reach the page nobody has read yet (docs/05, "Avoid offset pagination").
func (r *Repository) ListHistory(
	ctx context.Context,
	medicationID uuid.UUID,
	cursor string,
	limit int,
) (HistoryPage, error) {
	before, beforeID, err := decodeCursor(cursor)
	if err != nil {
		return HistoryPage{}, err
	}

	// One more than asked for, so the presence of a next page is known without
	// a second count query.
	rows, err := r.db.Query(ctx,
		`SELECT `+instanceColumns+`
		 FROM medication_instances
		 WHERE medication_id = $1
		   AND ($2::timestamptz IS NULL OR (scheduled_for, id) < ($2, $3))
		 ORDER BY scheduled_for DESC, id DESC
		 LIMIT $4`,
		medicationID, before, beforeID, limit+1)
	if err != nil {
		return HistoryPage{}, fmt.Errorf("list medication history: %w", err)
	}
	defer rows.Close()

	found, err := collectInstances(rows)
	if err != nil {
		return HistoryPage{}, err
	}

	if len(found) <= limit {
		return HistoryPage{Items: found}, nil
	}

	last := found[limit-1]
	return HistoryPage{
		Items:      found[:limit],
		NextCursor: encodeCursor(last.ScheduledFor, last.ID),
	}, nil
}

// ActParams records who acted on a dose and what they said about it.
type ActParams struct {
	InstanceID uuid.UUID
	Action     Action
	ActorID    uuid.UUID
	Notes      *string
}

// Act records a dose as taken or skipped.
//
// The update requires the dose to still be pending, so concurrent actions from
// two devices resolve in the database: exactly one updates a row, and the other
// is told what actually happened rather than overwriting it
// (plans/phase5.md §22).
//
// ErrNotFound here means "not pending any more", which the service
// distinguishes from "no such dose" by re-reading the row.
func (r *Repository) Act(ctx context.Context, params ActParams) (Instance, error) {
	status := resultOf[params.Action]

	instance, err := scanInstance(r.db.QueryRow(ctx, `
		UPDATE medication_instances
		SET status     = $2,
		    taken_at   = CASE WHEN $2 = 'taken'   THEN now() ELSE taken_at END,
		    taken_by   = CASE WHEN $2 = 'taken'   THEN $3 ELSE taken_by END,
		    skipped_at = CASE WHEN $2 = 'skipped' THEN now() ELSE skipped_at END,
		    skipped_by = CASE WHEN $2 = 'skipped' THEN $3 ELSE skipped_by END,
		    notes      = COALESCE($4, notes)
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
		return Instance{}, fmt.Errorf("record dose: %w", err)
	}
	return instance, nil
}

// --- Cursors ---------------------------------------------------------------

// The keyset cursor lives in internal/paging: appointments page their history
// the same way, and one encoder is easier to keep right than two.

// ErrBadCursor is returned when a page cursor cannot be read. It is a sentinel
// so the handler can answer 400 without inspecting the message.
var ErrBadCursor = paging.ErrBadCursor

var (
	encodeCursor = paging.EncodeCursor
	decodeCursor = paging.DecodeCursor
)

// --- Scanning --------------------------------------------------------------

type rowScanner interface {
	Scan(dest ...any) error
}

func scanMedication(row rowScanner) (Medication, error) {
	var (
		medication Medication
		form       *string
	)

	err := row.Scan(
		&medication.ID,
		&medication.SeniorID,
		&medication.CreatedByUserID,
		&medication.Name,
		&medication.Dosage,
		&form,
		&medication.Instructions,
		&medication.Notes,
		&medication.Active,
		&medication.CreatedAt,
		&medication.UpdatedAt,
	)
	if err != nil {
		return Medication{}, err
	}

	if form != nil {
		medication.Form = Form(*form)
	}
	return medication, nil
}

func scanSchedule(row rowScanner) (Schedule, error) {
	var (
		schedule Schedule
		rule     string
		at       string
	)

	err := row.Scan(
		&schedule.ID,
		&schedule.MedicationID,
		&schedule.SeniorID,
		&rule,
		&at,
		&schedule.Active,
		&schedule.CreatedAt,
		&schedule.UpdatedAt,
	)
	if err != nil {
		return Schedule{}, err
	}

	// A rule or time that will not parse means the row was written by something
	// other than this code. Failing here is better than scheduling medicine
	// from a value nobody can interpret.
	parsedRule, err := recurrence.Parse(rule)
	if err != nil {
		return Schedule{}, fmt.Errorf("medication schedule %s: %w", schedule.ID, err)
	}
	parsedTime, err := recurrence.ParseTimeOfDay(at)
	if err != nil {
		return Schedule{}, fmt.Errorf("medication schedule %s: %w", schedule.ID, err)
	}

	schedule.Recurrence = parsedRule
	schedule.ScheduledTime = parsedTime
	return schedule, nil
}

func scanScheduled(row rowScanner) (Scheduled, error) {
	var (
		schedule Schedule
		rule     string
		at       string
		name     string
		dosage   string
		author   uuid.UUID
	)

	err := row.Scan(
		&schedule.ID,
		&schedule.MedicationID,
		&schedule.SeniorID,
		&rule,
		&at,
		&schedule.Active,
		&schedule.CreatedAt,
		&schedule.UpdatedAt,
		&name,
		&dosage,
		&author,
	)
	if err != nil {
		return Scheduled{}, err
	}

	parsedRule, err := recurrence.Parse(rule)
	if err != nil {
		return Scheduled{}, fmt.Errorf("medication schedule %s: %w", schedule.ID, err)
	}
	parsedTime, err := recurrence.ParseTimeOfDay(at)
	if err != nil {
		return Scheduled{}, fmt.Errorf("medication schedule %s: %w", schedule.ID, err)
	}

	schedule.Recurrence = parsedRule
	schedule.ScheduledTime = parsedTime
	return Scheduled{
		Schedule:        schedule,
		Name:            name,
		Dosage:          dosage,
		CreatedByUserID: author,
	}, nil
}

func scanInstance(row rowScanner) (Instance, error) {
	var (
		instance Instance
		status   string
	)

	err := row.Scan(
		&instance.ID,
		&instance.ScheduleID,
		&instance.MedicationID,
		&instance.SeniorID,
		&instance.CreatedByUserID,
		&instance.Name,
		&instance.Dosage,
		&instance.ScheduledFor,
		&status,
		&instance.TakenAt,
		&instance.TakenBy,
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

func collectSchedules(rows pgx.Rows) ([]Schedule, error) {
	found := make([]Schedule, 0)
	for rows.Next() {
		schedule, err := scanSchedule(rows)
		if err != nil {
			return nil, fmt.Errorf("scan medication schedule: %w", err)
		}
		found = append(found, schedule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read medication schedules: %w", err)
	}
	return found, nil
}

func collectScheduled(rows pgx.Rows) ([]Scheduled, error) {
	found := make([]Scheduled, 0)
	for rows.Next() {
		scheduled, err := scanScheduled(rows)
		if err != nil {
			return nil, fmt.Errorf("scan due schedule: %w", err)
		}
		found = append(found, scheduled)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read due schedules: %w", err)
	}
	return found, nil
}

func collectInstances(rows pgx.Rows) ([]Instance, error) {
	found := make([]Instance, 0)
	for rows.Next() {
		instance, err := scanInstance(rows)
		if err != nil {
			return nil, fmt.Errorf("scan dose: %w", err)
		}
		found = append(found, instance)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read doses: %w", err)
	}
	return found, nil
}

// optionalForm renders an unset form as SQL NULL rather than the empty string,
// which the CHECK constraint would refuse.
func optionalForm(form Form) *string {
	if form == "" {
		return nil
	}
	value := string(form)
	return &value
}

// WithTx returns a repository bound to tx, so a medication change and the care event
// describing it are written through the same connection and commit together
// (plans/phase7.md §26).
func (r *Repository) WithTx(tx pgx.Tx) *Repository {
	return &Repository{db: tx}
}
