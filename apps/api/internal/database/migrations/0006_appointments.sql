-- 0006_appointments — where somebody has to be, and whether they got there.
--
-- One table, following docs/03-domain-model.md's Appointment exactly: id,
-- senior_id, title, provider_name, location, scheduled_at, assigned_user_id,
-- notes, status.
--
-- An appointment is not a care task (plans/phase6.md, objective). A task is a
-- routine somebody in the circle carries out and can repeat every weekday; an
-- appointment is a single commitment at a place, with a provider, that a person
-- travels to and that somebody else scheduled. They share a shape — a time and
-- an outcome — and nothing else: there is no template, no recurrence, and no
-- occurrence to materialise, because an appointment is already the concrete
-- thing.

CREATE TABLE appointments (
    id                  uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    senior_id           uuid        NOT NULL REFERENCES senior_profiles (id) ON DELETE CASCADE,

    -- Always the authenticated caller, never a value from the request body
    -- (plans/phase6.md §13). ON DELETE RESTRICT keeps the audit trail intact
    -- when the creator leaves the circle, as care_task_templates does.
    created_by_user_id  uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,

    title               text        NOT NULL,

    -- NULL when nobody said what sort of visit it is. A short recognised list
    -- rather than free text, so the create screen can offer choices — and
    -- deliberately not a medical taxonomy: GenxCare coordinates appointments,
    -- it does not classify care (plans/phase6.md §§2, 28).
    kind                text,

    provider_name       text        NOT NULL DEFAULT '',
    location            text        NOT NULL DEFAULT '',
    notes               text        NOT NULL DEFAULT '',

    -- Who in the circle is taking them. NULL means nobody in particular yet,
    -- which is the ordinary state of a newly booked appointment
    -- (docs/04, "Assign caregiver").
    assigned_user_id    uuid        REFERENCES users (id) ON DELETE SET NULL,

    -- The absolute instant it starts. Stored as timestamptz and rendered in the
    -- senior's timezone, so a family member abroad sees the hour their relative
    -- will experience (plans/phase6.md §4).
    scheduled_at        timestamptz NOT NULL,

    -- NULL when nobody knows how long it will take, which is most of the time.
    -- Because both ends are instants, an appointment that runs past midnight
    -- needs no special case.
    ends_at             timestamptz,

    -- Exactly the vocabulary in docs/03 and plans/phase6.md §3. There is no
    -- derived state here, unlike a task's 'overdue' or a dose's 'missed': an
    -- appointment whose time has passed has not become anything — somebody
    -- still has to say whether it happened. The client compares scheduled_at
    -- with the clock to separate upcoming from past.
    status              text        NOT NULL DEFAULT 'scheduled',

    completed_at        timestamptz,
    completed_by        uuid        REFERENCES users (id) ON DELETE SET NULL,

    -- A cancelled appointment is kept, never deleted: knowing that Tuesday's
    -- cardiology visit was called off is part of the care record
    -- (plans/phase6.md §9).
    cancelled_at        timestamptz,
    cancelled_by        uuid        REFERENCES users (id) ON DELETE SET NULL,

    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT appointments_title_not_blank
        CHECK (length(btrim(title)) > 0),

    CONSTRAINT appointments_kind_recognised
        CHECK (kind IS NULL OR kind IN (
            'doctor_visit', 'hospital_visit', 'therapy',
            'laboratory', 'care_meeting', 'other'
        )),

    CONSTRAINT appointments_status_recognised
        CHECK (status IN ('scheduled', 'completed', 'cancelled')),

    -- An appointment cannot finish before it starts. Equality is refused too:
    -- a zero-length appointment is a mistake, not a preference.
    CONSTRAINT appointments_ends_after_it_starts
        CHECK (ends_at IS NULL OR ends_at > scheduled_at),

    -- Settling must say who and when, so the record of who marked a visit done
    -- cannot be half-written (plans/phase6.md §10).
    CONSTRAINT appointments_completed_is_attributed
        CHECK (
            status <> 'completed'
            OR (completed_at IS NOT NULL AND completed_by IS NOT NULL)
        ),

    CONSTRAINT appointments_cancelled_is_attributed
        CHECK (
            status <> 'cancelled'
            OR (cancelled_at IS NOT NULL AND cancelled_by IS NOT NULL)
        )
);

-- "What is coming up for this senior?", and, scanned backwards, "what has
-- already happened?" — the only two ways appointments are listed.
--
-- id is the third column because the history pages by keyset on
-- (scheduled_at, id): two appointments at the same hour need a stable
-- tie-break, and including it here keeps that page an index scan. A btree is
-- read in either direction, so one index serves the ascending upcoming list and
-- the descending history alike.
--
-- Deliberately no index on status: nothing filters by it. Upcoming and past
-- both return every status, because a cancelled visit that vanished from the
-- list would look like one nobody had told you about (plans/phase6.md §§15, 31).
CREATE INDEX appointments_senior_schedule_idx
    ON appointments (senior_id, scheduled_at, id);

CREATE TRIGGER appointments_set_updated_at
    BEFORE UPDATE ON appointments
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
