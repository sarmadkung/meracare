-- 0005_medications — what a person takes, when they take it, and whether they did.
--
-- Three tables, following docs/03-domain-model.md exactly:
--
--   medications           the thing itself ("Metformin, 500 mg, tablet")
--   medication_schedules  when it is taken ("every day at 08:00")
--   medication_instances  one concrete dose that can be taken or skipped
--
-- Medication is its own domain, not a care task wearing a different label
-- (plans/phase5.md §1). The shapes rhyme with care tasks because the same care
-- problem recurs — a rule, and the occurrences it produces — but a dose is not
-- a chore: it is named after the medicine, it carries its dosage into history,
-- and one medicine can have several times of day, which a task template cannot.

CREATE TABLE medications (
    id                  uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    senior_id           uuid        NOT NULL REFERENCES senior_profiles (id) ON DELETE CASCADE,

    -- Kept for the audit trail, so the row survives the creator leaving the
    -- circle, exactly as care_task_templates.created_by_user_id does.
    created_by_user_id  uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,

    name                text        NOT NULL,

    -- Free text, as docs/03 specifies: "500 mg", "1 tablet", "two puffs". Not
    -- split into a number and a unit — GenxCare records what the family was
    -- told, and parsing it into quantities would invite arithmetic on a value
    -- nobody asked us to compute (plans/phase5.md §§2, 32).
    dosage              text        NOT NULL DEFAULT '',

    -- NULL when nobody said. A short recognised list rather than free text, so
    -- the create screen can offer choices instead of asking a person to spell
    -- "capsule".
    form                text,

    instructions        text        NOT NULL DEFAULT '',
    notes               text        NOT NULL DEFAULT '',

    -- Stopped medication is deactivated, never deleted: the doses already
    -- recorded against it are care history (plans/phase5.md §13).
    active              boolean     NOT NULL DEFAULT true,

    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT medications_name_not_blank
        CHECK (length(btrim(name)) > 0),

    CONSTRAINT medications_form_recognised
        CHECK (form IS NULL OR form IN (
            'tablet', 'capsule', 'liquid', 'drops', 'inhaler',
            'injection', 'patch', 'cream', 'other'
        )),

    -- Lets schedules and instances point at (medication, senior) as a pair,
    -- below.
    CONSTRAINT medications_id_senior_key UNIQUE (id, senior_id)
);

-- "What is this person taking?" — the only way medications are listed.
CREATE INDEX medications_senior_idx
    ON medications (senior_id);

CREATE TRIGGER medications_set_updated_at
    BEFORE UPDATE ON medications
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE medication_schedules (
    id                  uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    medication_id       uuid        NOT NULL,
    senior_id           uuid        NOT NULL REFERENCES senior_profiles (id) ON DELETE CASCADE,

    -- An RRULE subset: 'FREQ=DAILY' or 'FREQ=WEEKLY;BYDAY=MO,WE,FR'.
    -- internal/recurrence.Parse is the authority on what parses.
    recurrence_rule     text        NOT NULL,

    -- Wall-clock time in the senior's timezone, not an instant. 08:00 means
    -- "eight in the morning where they live", which is the only reading that
    -- survives a daylight-saving change (plans/phase5.md §23).
    scheduled_time      time        NOT NULL,

    active              boolean     NOT NULL DEFAULT true,

    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),

    -- "Twice a day" is two schedules, 08:00 and 20:00, rather than one rule
    -- with a list of times. It costs a row and buys the ability to stop the
    -- evening dose without touching the morning one (plans/phase5.md §3).
    CONSTRAINT medication_schedules_medication_same_senior
        FOREIGN KEY (medication_id, senior_id)
        REFERENCES medications (id, senior_id)
        ON DELETE CASCADE,

    -- Lets instances tie a dose to a schedule of the same medication, below.
    CONSTRAINT medication_schedules_id_medication_key UNIQUE (id, medication_id)
);

-- The same medicine cannot be due twice at the same time under the same rule.
-- Partial on active, so a schedule that was stopped and later restarted is not
-- blocked by its own history.
CREATE UNIQUE INDEX medication_schedules_no_duplicate_time_idx
    ON medication_schedules (medication_id, scheduled_time, recurrence_rule)
    WHERE active;

-- "What are this medicine's live times?" — read whenever doses are generated.
CREATE INDEX medication_schedules_medication_idx
    ON medication_schedules (medication_id)
    WHERE active;

CREATE TRIGGER medication_schedules_set_updated_at
    BEFORE UPDATE ON medication_schedules
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE medication_instances (
    id                  uuid        PRIMARY KEY DEFAULT gen_random_uuid(),

    -- NULL for a one-off dose entered directly rather than produced by a rule.
    schedule_id         uuid        REFERENCES medication_schedules (id) ON DELETE CASCADE,
    medication_id       uuid        NOT NULL,
    senior_id           uuid        NOT NULL REFERENCES senior_profiles (id) ON DELETE CASCADE,

    created_by_user_id  uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,

    -- What was to be taken, as it read at the time. A dose recorded as "500 mg"
    -- must still say 500 mg after the prescription changes to 1000 mg: the
    -- history is a record of what happened, not a view of today's dosage
    -- (plans/phase5.md §12).
    name                text        NOT NULL,
    dosage              text        NOT NULL DEFAULT '',

    -- The absolute instant the dose is due, derived from the senior's local
    -- date and the schedule's wall-clock time.
    scheduled_for       timestamptz NOT NULL,

    status              text        NOT NULL DEFAULT 'pending',

    taken_at            timestamptz,
    taken_by            uuid        REFERENCES users (id) ON DELETE SET NULL,

    skipped_at          timestamptz,
    skipped_by          uuid        REFERENCES users (id) ON DELETE SET NULL,

    notes               text        NOT NULL DEFAULT '',

    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT medication_instances_name_not_blank
        CHECK (length(btrim(name)) > 0),

    -- 'missed' is deliberately absent. A dose is missed when its time and its
    -- grace period have passed and nobody has acted on it — a reading of the
    -- clock, not a state anybody moves it into. Storing it would mean a
    -- background job had to run for the data to be true, and a care record that
    -- is only correct while a sweeper is alive is not a care record
    -- (plans/phase5.md §8).
    CONSTRAINT medication_instances_status_recognised
        CHECK (status IN ('pending', 'taken', 'skipped')),

    -- Taking or skipping must say who and when, so the audit trail cannot be
    -- half-written.
    CONSTRAINT medication_instances_taken_is_attributed
        CHECK (
            status <> 'taken'
            OR (taken_at IS NOT NULL AND taken_by IS NOT NULL)
        ),

    CONSTRAINT medication_instances_skipped_is_attributed
        CHECK (
            status <> 'skipped'
            OR (skipped_at IS NOT NULL AND skipped_by IS NOT NULL)
        ),

    -- A dose exists at most once per schedule. This is what makes generating a
    -- window idempotent: two concurrent readers of the same day race, and the
    -- loser's insert is discarded rather than telling somebody to take a second
    -- tablet.
    CONSTRAINT medication_instances_one_per_dose
        UNIQUE (schedule_id, scheduled_for),

    -- A dose cannot borrow a medication from another senior's circle, nor a
    -- schedule belonging to a different medicine. The application checks both;
    -- the database makes them impossible.
    CONSTRAINT medication_instances_medication_same_senior
        FOREIGN KEY (medication_id, senior_id)
        REFERENCES medications (id, senior_id)
        ON DELETE CASCADE,

    CONSTRAINT medication_instances_schedule_same_medication
        FOREIGN KEY (schedule_id, medication_id)
        REFERENCES medication_schedules (id, medication_id)
        ON DELETE CASCADE
);

-- "What is due for this senior between these two instants?" — today's
-- medication, the coming days, and the missed list.
CREATE INDEX medication_instances_senior_schedule_idx
    ON medication_instances (senior_id, scheduled_for);

-- "What has happened to this medicine?" — the history screen, newest first and
-- paged.
CREATE INDEX medication_instances_medication_history_idx
    ON medication_instances (medication_id, scheduled_for DESC, id DESC);

-- Supports regenerating a schedule's future doses after an edit.
CREATE INDEX medication_instances_schedule_idx
    ON medication_instances (schedule_id, scheduled_for)
    WHERE schedule_id IS NOT NULL;

CREATE TRIGGER medication_instances_set_updated_at
    BEFORE UPDATE ON medication_instances
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
