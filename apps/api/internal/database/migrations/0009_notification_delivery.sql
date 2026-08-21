-- 0009_notification_delivery — the notifications MeraCare has actually decided
-- to send, and the state of sending them.
--
-- 0008 deliberately stored no reminders, because a reminder is a consequence of
-- care and recomputing it is always right where a stored copy can go stale.
-- That reasoning still holds for the plan a device schedules locally, and that
-- plan is unchanged.
--
-- What changed is that a *delivered* notification is not a consequence. It is
-- an event: it happened, at a time, to a person, and it has to be remembered
-- afterwards — because an inbox is a history, because "already sent" is the
-- only thing that stops sending it twice, and because a push that failed has to
-- be retried without anyone's phone having to be awake. None of that can be
-- recomputed from the care, so all of it is written down here
-- (plans/phase11.md §§6, 20, 21, 27).
--
-- One table, not two. plans/phase11.md §51 allows a separate job table; it
-- would carry the notification id, its scheduled time, and its delivery state,
-- which is a second row that exists only to describe the first and can
-- disagree with it. Delivery state lives on the notification itself, and
-- "claim the work" and "record what happened" become one update rather than
-- two rows to keep in step (§50, "only create tables that are actually
-- required").

-- The two preference categories Phase 11 gives a delivery path.
--
-- 0008 refused to add switches for categories nothing could send, on the
-- grounds that a switch controlling nothing is worse than a missing one. These
-- two now control something, so they arrive now.
ALTER TABLE notification_preferences
    ADD COLUMN overdue_task_alerts boolean NOT NULL DEFAULT true,
    -- Care activity defaults ON for parity with the rest, and is the one
    -- category most likely to be turned off by a professional caregiver
    -- watching several people (plans/phase11.md §45).
    ADD COLUMN care_activity       boolean NOT NULL DEFAULT true;

CREATE TABLE notifications (
    id                  uuid        PRIMARY KEY DEFAULT gen_random_uuid(),

    -- The person, not the senior. Several people care for the same senior and
    -- each of them decides separately what they want to hear about, so a
    -- notification addressed to a senior would have no one to deliver it to and
    -- no preferences to consult (plans/phase11.md §7).
    recipient_user_id   uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,

    -- Who the notification is about. Always present: every type MeraCare sends
    -- today concerns one senior's care.
    senior_id           uuid        NOT NULL REFERENCES senior_profiles (id) ON DELETE CASCADE,

    notification_type   text        NOT NULL,

    -- The words, as they were sent.
    --
    -- Stored rather than composed at read time, for the same reason a care
    -- event stores what was true when it was written: the inbox must show what
    -- the phone showed, and it must go on saying so after the appointment moves
    -- or the medicine is renamed. The privacy policy that produced them is in
    -- internal/notifications/wording.go — nothing here ever names a medicine, a
    -- dosage, or a condition (plans/phase11.md §48).
    title               text        NOT NULL,
    body                text        NOT NULL,

    -- What to open. Identifiers only: the screen re-authorizes when it loads,
    -- so holding a notification is never holding access (plans/phase11.md §31).
    entity_type         text        NOT NULL,
    entity_id           uuid        NOT NULL,

    -- When this notification is *for*. It is also the inbox's sort key and the
    -- moment delivery becomes allowed, which is why a row may exist before it
    -- is visible to anyone: the scheduler materialises the next hour ahead so a
    -- push does not depend on a sweep happening at exactly the right second.
    scheduled_for       timestamptz NOT NULL,

    -- The idempotency mechanism, and the reason a scheduler that runs every
    -- minute over the same hour does not send sixty notifications
    -- (plans/phase11.md §20). Derived from recipient, type, subject, and
    -- occurrence; see internal/notifications/materialise.go.
    dedupe_key          text        NOT NULL,

    -- pending → sent, failed, or skipped. Skipped is not a failure: it is what
    -- a notification gets when there is nowhere to push it, which is the normal
    -- state of every notification until push credentials exist. The inbox does
    -- not care either way (plans/phase11.md §§38, 43).
    delivery_status     text        NOT NULL DEFAULT 'pending',
    attempts            integer     NOT NULL DEFAULT 0,
    -- Not before this instant. Advanced on each failure, which is the whole of
    -- the backoff (plans/phase11.md §38).
    available_at        timestamptz NOT NULL DEFAULT now(),
    delivered_at        timestamptz,
    -- Kept for support. Provider messages only; no token is ever written here
    -- (plans/phase11.md §24).
    last_error          text        NOT NULL DEFAULT '',

    read_at             timestamptz,

    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),

    -- Mirrors internal/notifications.Types. A type the app cannot render would
    -- be an inbox row with no icon and no destination.
    CONSTRAINT notifications_type_recognised
        CHECK (notification_type IN (
            'MEDICATION_REMINDER',
            'APPOINTMENT_REMINDER',
            'TASK_REMINDER',
            'TASK_OVERDUE',
            'CARE_ACTIVITY'
        )),

    -- Mirrors internal/notifications.EntityTypes.
    CONSTRAINT notifications_entity_recognised
        CHECK (entity_type IN (
            'task_instance',
            'medication_dose',
            'appointment',
            'care_event'
        )),

    CONSTRAINT notifications_delivery_status_recognised
        CHECK (delivery_status IN ('pending', 'sent', 'failed', 'skipped')),

    CONSTRAINT notifications_attempts_not_negative CHECK (attempts >= 0)
);

-- One notification per recipient per occurrence, enforced here rather than by a
-- read-then-write in the scheduler. Two schedulers racing on the same minute
-- both try to insert; one wins and the other's ON CONFLICT DO NOTHING is the
-- entire concurrency story for materialisation (plans/phase11.md §§20, 37).
CREATE UNIQUE INDEX notifications_dedupe_idx ON notifications (dedupe_key);

-- The inbox: this user's notifications, newest first, keyset-paged on
-- (scheduled_for, id) exactly like every other history in the API
-- (plans/phase11.md §§41, 56).
CREATE INDEX notifications_inbox_idx
    ON notifications (recipient_user_id, scheduled_for DESC, id DESC);

-- The badge. Partial, because the count only ever asks about unread rows and a
-- full index would be mostly read notifications nobody is counting.
CREATE INDEX notifications_unread_idx
    ON notifications (recipient_user_id)
    WHERE read_at IS NULL;

-- The delivery sweep: what is due, not yet sent, and off backoff. Partial for
-- the same reason — a table that is 99% delivered should not make the sweep
-- read 99% of it (plans/phase11.md §71).
CREATE INDEX notifications_delivery_idx
    ON notifications (available_at, scheduled_for)
    WHERE delivery_status = 'pending';
