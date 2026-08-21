# Notifications and Delivery

How MeraCare decides what to tell each person, when to tell them, and how it
reaches their phone. Implemented in Phase 11 (`plans/phase11.md`), on top of the
device-scheduled reminders of Phase 8.

Nothing here is a secret. Where a production credential is needed, this document
says which one and where it belongs — never its value.

> Numbering note: `plans/phase11.md` §67 suggests `docs/16-notifications.md`, but
> `16` and the numbers around it are already taken (`docs/INDEX.md`). This is the
> next free number.

## Two Ways a Reminder Reaches a Phone

MeraCare has both, and that is deliberate.

**Device-scheduled reminders** (Phase 8) — the app fetches a plan from
`GET /v1/notifications/reminders` and schedules local notifications with the
OS. They fire with no network, no server, and no push credentials. This is the
only path that works today.

**Server-delivered notifications** (Phase 11) — the API decides, records, and
pushes. Only a server can know that a task went unrecorded, or that somebody
*else* just gave a dose, so these are the only route for `TASK_OVERDUE` and
`CARE_ACTIVITY`. They also carry an inbox, which a fire-and-forget local
notification cannot.

**Exactly one of them schedules any given reminder.** The device decides which:
if its registration comes back with `pushTokenRegistered: true`, the server is
delivering and the app clears its local schedule; otherwise the app schedules
locally. Two would announce every dose twice, which is the kind of bug that
teaches people to ignore the app.

Because MeraCare holds no push credentials yet, `pushTokenRegistered` is false
everywhere and local scheduling is what actually runs. The server path is
complete and dormant.

## Notification Types

| Type | When | Points at |
| --- | --- | --- |
| `MEDICATION_REMINDER` | 15 min before a dose | today's medication |
| `TASK_REMINDER` | 15 min before a task | the task |
| `APPOINTMENT_REMINDER` | 24 h and 1 h before | the appointment |
| `TASK_OVERDUE` | 30 min after a task's time, if still unrecorded | the task |
| `CARE_ACTIVITY` | when somebody else records care | the activity timeline |

The 15-minute and 1-hour offsets are Phase 8's, unchanged, so a dose reminds at
the same moment whichever path delivers it. The 24-hour appointment reminder is
Phase 11's one addition: a day's notice is what lets somebody arrange a lift; an
hour's notice is what gets them out of the door.

There is deliberately **no missed-medication type**. Missed is derived from the
clock and never stored (`plans/phase4.md` §8, `plans/phase5.md` §8); a
notification type for it would mean inventing the background sweep those phases
refused.

`CARE_ACTIVITY` covers four of the fifteen care-event types — a dose recorded, a
task completed, an appointment completed, a member joining. A care event is a
record; a notification is an interruption, and most records do not deserve one.

## Architecture

```text
tasks / medications / appointments        care_events
            │                                  │
            └────────────┬─────────────────────┘
                         ▼
                  Scheduler pass
              (decide → insert → deliver)
                         │
                notifications table
                    │          │
                 inbox      push provider ──→ device
```

The notification package never learns what a dosage is. It reads the domains
through narrow adapters at the composition root (`internal/server/adapters.go`),
so the only vocabulary it has is "something falls due at a time" and "something
happened". That is what makes the privacy policy below structural rather than a
matter of care.

### The scheduler

One background process, started in `cmd/api/main.go` and stopped before the pool
closes. A pass:

1. **Decides.** Reads the roster of active memberships, each user's preferences,
   and the domains; produces the notifications that should exist in the next
   hour. A pure function — no writes, no clock of its own.
2. **Inserts.** `ON CONFLICT (dedupe_key) DO NOTHING`.
3. **Delivers.** Claims what is due, pushes it, records the outcome.
4. **Forgets.** Hourly, deletes anything past retention.

It runs a pass before its first tick, so a restart delivers whatever fell due
while the process was down.

**Concurrency.** Two API instances may run it. Materialisation is safe because
the dedupe key is unique in the database; delivery is safe because claiming is
one `UPDATE … WHERE id IN (SELECT … FOR UPDATE SKIP LOCKED) RETURNING`, which
also pushes `available_at` forward by a two-minute lease. A worker that dies
mid-flight loses its lease, not its work.

### Deduplication

The key is a UUIDv5 over recipient, type, subject, and the exact moment the
notification is for. The scheduler re-examines an overlapping window every
minute; the second pass inserts nothing. `scheduled_for` is part of the identity
on purpose — an appointment moved from 10:00 to 14:00 is a different
notification, not an amendment to one already sent.

### Transactional consistency

Care activity is read from `care_events` **after commit**. An action that rolled
back was never visible, so there is nothing to un-notify. This gets the
guarantee `plans/phase11.md` §52 asks for without threading a notification write
through tasks, medications, appointments, and members.

## Storage

`0009_notification_delivery.sql` adds two preference columns and one table.

One table, not two. §51 allows a separate job table; it would carry the
notification id, its time, and its delivery state — a second row that exists
only to describe the first and can disagree with it. Delivery state lives on the
notification, so "claim the work" and "record what happened" are one update.

Indexes: unique on `dedupe_key`; `(recipient_user_id, scheduled_for DESC, id
DESC)` for the inbox; partial on unread for the badge; partial on
`(available_at, scheduled_for)` where pending for the delivery sweep.

### Retention

**30 days**, configurable with `NOTIFICATION_RETENTION`, swept hourly. A
notification is a copy of something the care record already holds — the dose,
the task, the care event all remain — so keeping it forever grows a table
without preserving anything.

## Privacy and Lock-Screen Content

A notification appears on a locked phone, in front of whoever is near it. The
wording is a privacy decision before it is a copy decision, and the rule is
absolute (`docs/09-security-privacy.md`, `plans/phase11.md` §48):

**Never** a medicine's name, dosage, form, or instructions; never a condition,
diagnosis, symptom, or reason for care; never an appointment's title, clinic, or
practitioner; never a task's description.

**Allowed** is who it concerns and when — a caregiver looking after two people
needs both to know whether to act, and neither says anything medical.

```text
Medication reminder
A dose is due for Amma at 08:00.

Upcoming appointment
Amma has an appointment tomorrow at 10:00.

Care activity
Priya recorded a dose for Amma.
```

Everything specific is fetched after the app is opened, under the reader's own
authorization. `internal/notifications/wording.go` is the only place that
composes a sentence, and it is never given anything medical to compose with —
the mechanism, not the intention. `wording_test.go` pins the policy against a
list of the material that would actually leak.

Push payloads carry five identifiers and nothing else.

The words are stored on the notification as they were sent, so the inbox shows
what the phone showed, and goes on saying so after the appointment moves — the
same reasoning that makes a care event a historical record.

## Time Zones

Care runs on the senior's clock. A daughter in London is told about her mother's
08:00 dose, not about 02:30. The IANA zone comes from the senior profile;
`cmd/api` embeds tzdata so a minimal container does not silently resolve every
zone to UTC. An unknown or missing zone falls back to UTC — consistent and
visibly wrong, rather than a reminder that fails.

**Daylight saving.** Lead times are absolute durations subtracted from an
instant the domains produced. Calendar recurrence — "every day at 08:00", which
must stay 08:00 across a boundary — belongs to `internal/recurrence` and is not
duplicated here, so this layer can neither gain nor lose an hour of its own. A
test pins the 29 March 2026 London spring-forward.

**The inbox** is the one place the *reader's* zone is used: "when did I get
this?" is a question about the reader, and a professional caregiver's inbox
spans several seniors' zones.

## API

| Method | Path | |
| --- | --- | --- |
| `GET` | `/v1/notifications` | inbox page + `unreadCount`; `cursor`, `limit` |
| `PATCH` | `/v1/notifications/{id}/read` | mark one read (idempotent) |
| `POST` | `/v1/notifications/read-all` | mark every arrived one read |
| `GET`/`PATCH` | `/v1/notifications/preferences` | the five switches |
| `POST` | `/v1/notifications/devices` | register or refresh an install |
| `DELETE` | `/v1/notifications/devices/{deviceId}` | deactivate |
| `GET` | `/v1/notifications/reminders` | the device-scheduled plan (Phase 8) |

Every inbox route is scoped to the caller in the SQL itself; no user id reaches
them from anywhere but the verified token. Somebody else's notification id and
an invented one both answer 404, so an id cannot be probed. Paging is the shared
keyset cursor (`internal/paging`), on `(scheduled_for, id)` — several
notifications routinely share an instant, and a timestamp-only cursor would drop
one at every page boundary.

The inbox never exposes delivery state. Whether a push reached a phone is
operational, not something the reader has any use for.

## Devices and Push

An install registers on every sign-in with a stable device id kept in the
keychain, so registration is an upsert rather than an accumulation. A user has
several devices and each is pushed. A registration with no token is still
recorded — that is an install that has not been granted permission.

Tokens are credentials for making somebody's phone buzz: never returned by any
endpoint, never logged, never written into `last_error`, and only ever
registerable for the authenticated caller. `DeviceResponse` reports
`pushTokenRegistered` — whether we hold one, not what it is.

**Provider: Expo.** It speaks to both APNs and FCM and needs no Apple key or
Firebase service account in this repository. It sits behind `PushSender`, so
nothing outside `internal/notifications/push.go` knows what Expo is.

**Retry.** Three attempts, backing off 1 → 5 → 15 minutes, then abandoned. A
push that has failed three times over twenty minutes is not failing for a reason
a fourth attempt fixes, and the notification is in the inbox either way.

**Rejected tokens.** `DeviceNotRegistered` retires the token immediately — the
device row survives with its token cleared, so the same install signing in again
is still an update.

**No reachable device** is `skipped`, not `failed`. It is not an error; there is
simply nowhere to send it.

## Permission, and What Happens Without It

The app asks for notification permission from the settings screen, where the
user is already thinking about notifications — never unprompted at startup. Once
the OS has been refused it will not ask again, so the screen points at system
settings rather than offering a button that does nothing.

With permission denied the app works: the inbox, history, read state, and every
care screen are unaffected. Only the buzz is missing. The settings screen shows
both halves — what MeraCare will send, and what the OS allows — because an app
that conflates them insists reminders are on while the phone stays silent.

## Configuration

| Variable | Default | |
| --- | --- | --- |
| `NOTIFICATION_SCHEDULER_ENABLED` | `true` | the inbox depends on it |
| `NOTIFICATION_SCHEDULER_INTERVAL` | `1m` | |
| `NOTIFICATION_RETENTION` | `720h` | 30 days |
| `PUSH_ENABLED` | `false` | no credentials exist yet |
| `EXPO_ACCESS_TOKEN` | — | secret; only for enhanced push security |

### Turning push on

Not done, and it cannot be done from this repository. It needs:

1. **An EAS project.** `eas init` in `apps/mobile`, which writes
   `extra.eas.projectId` into `app.json`. Without it the app cannot obtain an
   Expo push token at all, which is why `pushTokenRegistered` is false today.
2. **A development or production build.** Expo Go cannot receive push
   notifications on Android at all, and its iOS support is not a substitute for
   a real build. `eas build --profile development`.
3. **Push credentials in EAS** — an Apple push key, an FCM service account. EAS
   holds them. They must never enter this repository.
4. **`PUSH_ENABLED=true`** on the API.

Turning `PUSH_ENABLED` on *without* steps 1–3 would replace working local
reminders with pushes that go nowhere.

## Limitations

- **No real push has ever been sent.** No EAS project, no push credentials, no
  physical device. Everything below the provider boundary is tested against a
  stand-in; the provider itself is not. See `docs/IMPLEMENTATION_STATUS.md`.
- **Browser push is not implemented.** Web registers as a device and gets the
  full in-app inbox, badge, read state, and deep links, but never a token — Web
  Push needs a service worker and VAPID keys, which is its own piece of work.
  This is a deliberate Phase 11 limitation.
- **The roster is unpaged.** One pass reads every active membership at once. At
  MeraCare's scale that is a few thousand rows; the first deployment where it is
  not will need the sweep sharded by senior.
- **Care activity is polled, not pushed.** Each pass re-reads the last fifteen
  minutes of care events. Redundant, and self-healing after downtime; a
  watermark table would be less redundant and one more thing to keep correct.
- **A task completed between a sweep and its delivery** still gets its overdue
  alert. The window is one pass wide.
- **No badge on the app icon.** The in-app badge is the unread count from the
  inbox and cannot drift from it. The OS icon badge would be a second count with
  its own failure modes; `plans/phase11.md` §61 asks for a documented limitation
  over an unreliable implementation.

## Verifying It

```bash
cd apps/api && gofmt -l . && go vet ./... && go test -race -count=1 ./...
cd apps/mobile && pnpm typecheck && pnpm lint && pnpm test
pnpm format:check
```

Fresh-database migrations:

```bash
createdb meracare_fresh   # or docker exec … psql -c 'CREATE DATABASE …'
DATABASE_URL=…/meracare_fresh go run ./cmd/migrate up
TEST_DATABASE_URL=…/meracare_fresh go test -count=1 ./...
```

### The manual push test, once push is configured

On a development build on a physical iPhone and a physical Android phone:

1. Sign in, accept the notification prompt, and confirm the settings screen says
   notifications are allowed.
2. Confirm `POST /v1/notifications/devices` returned
   `pushTokenRegistered: true`, and that the app has stopped scheduling local
   reminders.
3. Create a medication dose due in about 20 minutes.
4. Lock the phone and wait. The notification must appear ~15 minutes before the
   dose, say **"Medication reminder"** and no medicine name, and make a sound.
5. Tap it. The app must open today's medication for that senior — from cold
   start, from background, and while already open.
6. Confirm the notification is in the inbox, marked read once opened, and that
   the home badge decreased.
7. Uninstall the app, then let another notification fall due. The token must be
   retired: `notification_devices.active` false and `push_token` null.
