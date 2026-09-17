-- 0008_notifications — what GenxCare is allowed to remind each user about, and
-- which devices it may one day push to.
--
-- Two tables, and deliberately no third one for the reminders themselves.
--
-- A reminder is not a record; it is a consequence. "Remind me 15 minutes before
-- Amma's 08:00 dose" is entirely decided by the dose, the schedule, the
-- senior's timezone, and this user's preferences — all of which are already
-- stored. Writing the consequence down as well would create a second copy that
-- can disagree with the first: the medicine is stopped, but the reminder row
-- still says 07:45. plans/phase8.md §31 forbids exactly that disagreement, and
-- §22 forbids the stale reminder it produces. Computing the plan on every
-- request makes both structurally impossible rather than a thing the code has
-- to remember to clean up. See internal/notifications/reminder.go.

-- Which categories of reminder this user wants.
--
-- Per user, not per senior (plans/phase8.md §3): a daughter who wants
-- medication reminders wants them for both her parents, and a professional
-- caregiver who silences task reminders overnight means all of them. The
-- senior is the subject of the care; the user is the person holding the phone.
CREATE TABLE notification_preferences (
    -- The user is the identity, so the primary key is the user. One row per
    -- person, no way to accumulate two contradictory preference sets.
    user_id             uuid        PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,

    -- Defaults are on. Someone who installed a care app to be reminded about
    -- care should not have to discover a settings screen before the first
    -- reminder arrives; turning them off is one tap and is remembered.
    task_reminders          boolean NOT NULL DEFAULT true,
    medication_reminders    boolean NOT NULL DEFAULT true,
    appointment_reminders   boolean NOT NULL DEFAULT true,

    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

-- Only the three categories that can actually be delivered today have a column
-- here. docs/08-notifications-and-background.md also lists activity, messages,
-- invitations, and escalation alerts; none of those has a delivery path yet,
-- and a switch that controls nothing is worse than a missing switch — the user
-- turns it off and still gets nothing, or turns it on and still gets nothing,
-- and either way the app has lied. They arrive with the push phase that sends
-- them, as columns on this table (plans/phase8.md §3).

-- A device that may be sent a push notification.
--
-- One row per device per user, because a user genuinely has several: a phone
-- and a tablet, or a new phone whose old one has not been wiped yet. Storing a
-- single token per user would silently move every notification to whichever
-- device signed in most recently (plans/phase8.md §7).
CREATE TABLE notification_devices (
    id                  uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,

    -- A stable identifier the app generates once per installation and keeps in
    -- secure storage. It is what makes registration idempotent: the same
    -- install re-registering after every launch updates one row rather than
    -- adding one (plans/phase8.md §§25, 27).
    device_id           text        NOT NULL,

    platform            text        NOT NULL,

    -- Nullable on purpose. An install registers as soon as it signs in, before
    -- the user has been asked for notification permission and possibly forever
    -- if they say no. A device we may not push to is still a device we know
    -- about, and refusing to record it would mean re-deriving that state on
    -- every launch.
    --
    -- This column is a credential for reaching someone's phone. It is never
    -- returned by any endpoint and never logged (plans/phase8.md §8).
    push_token          text,

    -- Free text, for support: "1.4.0 (57)". Never parsed for behaviour.
    app_version         text        NOT NULL DEFAULT '',

    -- False for a device that has signed out, or whose token the push provider
    -- has rejected. The row stays so the same install re-registering is still
    -- an update rather than a duplicate (plans/phase8.md §9).
    active              boolean     NOT NULL DEFAULT true,

    -- When this install last told us it exists. Lets a later phase retire
    -- devices that have not been seen for months without guessing.
    last_seen_at        timestamptz NOT NULL DEFAULT now(),

    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),

    -- Matches internal/notifications.Platforms. A typo'd platform would route
    -- pushes to the wrong provider, which fails silently rather than loudly.
    CONSTRAINT notification_devices_platform_recognised
        CHECK (platform IN ('ios', 'android', 'web')),

    -- An empty device id would collide with every other empty one and make
    -- registration non-idempotent for the installs that need it most.
    CONSTRAINT notification_devices_identified
        CHECK (length(trim(device_id)) > 0)
);

-- The idempotency guarantee, enforced by the database rather than by a
-- read-then-write in the service: one install, one row, however many times it
-- registers and however concurrently (plans/phase8.md §25).
CREATE UNIQUE INDEX notification_devices_install_idx
    ON notification_devices (user_id, device_id);

-- No second index. The only other access pattern is "the devices to push to for
-- this user", which the unique index above already serves on its leading
-- column (plans/phase8.md §34, docs/11-performance-requirements.md).
