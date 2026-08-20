# Implementation Status

Last updated: 2026-08-20

## Current Phase

**Phase 10 — Google Social Authentication: code complete, blocked on console
configuration.**

The MVP (Phases 1–9) is complete. Phase 10 adds "Continue with Google" beside
email sign-in on iOS, Android, and web. Every code path, test, and platform
build is done and verified; the end-to-end sign-in is **not** verified, because
the Google provider is not enabled in the Supabase project and enabling it needs
Google Cloud and Supabase dashboard access. See Blocker 6 and
`docs/19-google-authentication.md`.

## Verification Is Local

GitHub Actions is unavailable on this account — every run failed on billing
within seconds of starting. The workflow is paused (`workflow_dispatch` only in
`.github/workflows/ci.yml`) so those failures stop marking healthy commits red,
and each phase is verified locally instead:

```bash
cd apps/api && gofmt -l . && go vet ./... && go test -race -count=1 ./...
cd apps/mobile && npx tsc --noEmit && pnpm lint && pnpm test
pnpm format:check
```

Migrations are additionally applied to a brand-new database once per phase, with
the integration suite re-run against it, because a migration that only ever runs
on an existing database can hide an ordering mistake.

`TEST_DATABASE_URL` is read from `apps/api/.env` (an inline value or a real
environment variable still wins), and must point at localhost. The suite truncates every
application table on every test, so aimed at the hosted Supabase project it
would erase real care records — silently, with nothing to notice until
afterwards. `testsupport.RequireLocalHost` refuses anything else before a
connection is opened, and there is deliberately no override: a flag to disable
it would be set once, in a shell nobody remembers. The API itself is a separate
setting (`DATABASE_URL`) and may point wherever you like.

Restoring CI means putting the `push` and `pull_request` triggers back; the jobs
themselves are unchanged and cover the same ground as the commands above.

## Repository State

```text
apps/
  api/                     Go modular monolith
    cmd/api/               HTTP server entrypoint
    cmd/migrate/           migration CLI (up | status)
    internal/
      appointments/        visits, cancel/complete; /v1/appointments
      auth/                Supabase JWT verification, Principal, RequireAuth
      authz/               relationship-based authorization middleware + guard
      care/                roles, permissions, statuses, defaults, delegation
      careevents/          one timeline across every domain; recorder + activity
      config/              environment configuration
      database/            pgx pool, embedded migrations + runner, error helpers,
                           Querier and InTx for transactional writes
        migrations/        0001_init … 0008_notifications
      invitations/         tokens, lifecycle, accept; /v1/invitations
      medications/         medicines, schedules, doses; /v1/medications
      members/             care-circle membership; /v1/seniors/{id}/members
      notifications/       preferences, device registration, reminder plan
      paging/              shared keyset cursor for every paged history
      recurrence/          shared RRULE subset and timezone-aware expansion
      tasks/               care tasks, completion; /v1/tasks
      relationships/       care relationship model and repository
      seniors/             senior profiles, /v1/seniors
      server/              router wiring, health/readiness
      testsupport/         integration-test database helper
      users/               application user model, repository, service, /v1/me
    pkg/
      httpx/               error envelope, JSON helpers, middleware
      logging/             slog setup
      validation/          request validation helpers
  mobile/                  Expo SDK 57 / React Native 0.86 / Expo Router
    src/
      app/                 index, sign-in, home, onboarding, seniors/[id]/*
                           (incl. activity), appointments/[id]/*,
                           medications/[id]/*, tasks/[id], invitations/[token],
                           settings/notifications
      components/ui/       ActivityRow, AppointmentCard, Button, Card,
                           MedicationCard, OptionCard, PermissionToggle, Screen,
                           TaskCard, Text, TextField
      features/activity/   activity timeline query
      features/appointments/ appointment queries and mutations
      features/auth/       session restore, sign in/up/out
      features/circle/     members, invitations, accept
      features/medications/ medication queries, mutations, offline replay
      features/notifications/ preferences, device registration, OS permission,
                           reminder reconciliation, deep links
      features/sync/       one offline queue drain across every entity
      features/tasks/      task queries, mutations, offline replay
      lib/offline/         expo-sqlite cache, sync queue, failure classification
      features/profile/    /v1/me query + mutation
      features/seniors/    senior queries and mutations
      lib/                 env, supabase, secure storage, api client, query
                           client, timezone helpers
      stores/              small Zustand UI store
      theme/               semantic design tokens + ThemeProvider
packages/
  config/                  shared tsconfig / prettier config
  contracts/               TypeScript contracts mirroring the Go API
docker-compose.yml         local PostgreSQL for development and tests
.github/workflows/ci.yml   Go + mobile CI
```

## Completed — Phase 1

| # | Item | Where |
|---|------|-------|
| 1 | Inspect repository | — |
| 2 | Project structure | `apps/`, `packages/` per docs/16 |
| 3 | pnpm workspace | `pnpm-workspace.yaml`, `.npmrc` (`node-linker=hoisted`) |
| 4 | Expo application | `apps/mobile` (SDK 57, RN 0.86.2, Expo Router, typed routes) |
| 5 | Go API | `apps/api`, module `github.com/meracare/api` |
| 6 | TypeScript | `packages/config/tsconfig.base.json`, strict everywhere |
| 7 | Lint / format | `eslint-config-expo`, Prettier, `gofmt`/`go vet` |
| 8 | Environment variables | `apps/api/.env.example`, `apps/mobile/.env.example` |
| 9 | Supabase | Auth client + JWT verification wired; project provisioning pending (see Blockers) |
| 10 | PostgreSQL migrations | embedded SQL + advisory-locked runner, `cmd/migrate` |
| 11 | Database connection | `internal/database`, pgx pool, startup ping, `/readyz` |
| 12 | Supabase Auth integration | `SessionProvider` restores the stored session; `users.Service` maps identity → app user |
| 13 | Go JWT verification | `internal/auth`, HS256 with audience/issuer/expiry checks |
| 14 | API error handling | `pkg/httpx` error envelope per docs/05 |
| 15 | Logging | `pkg/logging` structured slog, request-scoped, no tokens or health data |
| 16 | Testing infrastructure | Go unit + integration tests, Jest/jest-expo, CI workflow |

### Verified end to end

- `go build ./...`, `go vet ./...`, `gofmt -l .` clean.
- `go test ./...` — 10 packages pass, including integration tests against real
  PostgreSQL (`TEST_DATABASE_URL` set).
- Migration applied to a live database; `users` table has the expected
  constraints, partial unique email index, and `updated_at` trigger.
- API booted locally: `/healthz` 200, `/readyz` 200, `/v1/me` 401 without a
  token, 200 with a valid token (creating the application user on first call),
  `PATCH /v1/me` updates and rejects invalid input with `VALIDATION_FAILED`.
- Mobile: `tsc --noEmit` clean, `expo lint` clean, 16 Jest tests pass.

## Completed — Phase 2

| # | Item | Where |
|---|------|-------|
| 17 | User model | Phase 1; extended by the relationships that reference it |
| 18 | SeniorProfile | `senior_profiles` table, `internal/seniors` |
| 19 | CareRelationship | `care_relationships` table, `internal/relationships` |
| 20 | Solo mode | `mode=self` links the profile to the caller and grants the `senior` role, with no invitation |
| 21 | Create senior | `POST /v1/seniors`, mobile onboarding screen |
| 22 | Edit senior | `PATCH /v1/seniors/{id}`, mobile edit screen |
| 23 | Senior dashboard | `GET /v1/seniors/{id}`, mobile dashboard shell |
| — | Role/permission enforcement | `internal/authz` guard on every senior-scoped route |

### Verified end to end

- `go test -race ./...` green across 13 packages, including integration tests
  against real PostgreSQL.
- Over HTTP, with two separate accounts: a stranger listing seniors gets an
  empty list; reading or editing another circle's senior by ID returns 404, not
  403, so senior IDs cannot be probed.
- A professional caregiver — a legitimate member — is refused `PATCH` on their
  own client, proving the check is per-permission and not merely per-membership.
- Unknown request fields are rejected, so a client cannot submit its own
  `permissions`; a second Solo Mode profile returns `CONFLICT`.
- Constraint-level: an unrecognised permission cannot be stored, a circle
  cannot hold two seniors, a user cannot hold two memberships of one circle, and
  a failed create leaves no orphan profile.
- Mobile: `tsc --noEmit` clean, `expo lint` clean, 20 Jest tests pass.

## Completed — Phase 3

| Area | Where |
|------|-------|
| Care circle | `internal/members`, `GET/PATCH/DELETE /v1/seniors/{id}/members` |
| Invitations | `internal/invitations`, `POST/GET /v1/seniors/{id}/invitations` |
| Accept flow | `GET /v1/invitations/{token}`, `POST /v1/invitations/{token}/accept` |
| Revoke invitation | `POST /v1/invitations/{id}/revoke` |
| Permission delegation | `internal/care/delegation.go` |
| Database | migration `0003_invitations` |
| Mobile | care circle, invite flow, pending invitations, accept screen, member access editing |

### Verified end to end

- `go test -race -count=1 ./...` green across 15 packages, including the
  invitation and membership integration suites.
- Migrations applied to a brand-new database, then the full integration suite
  re-run against it.
- Over HTTP with three separate accounts: an intruder gets 404 for listing
  members, listing invitations, and revoking an invitation; 403 with a distinct
  message for attempting to accept an invitation addressed to somebody else.
- A spent token returns `CONFLICT` on reuse. A family member holding
  `members.invite` but not `members.manage` gets 404 on both `PATCH` and
  `DELETE` of a member. A caregiver attempting to grant themselves `senior.edit`
  and `members.manage` gets 404.
- Revoking a member removed their access immediately (0 seniors reachable, 404
  on the senior) while the relationship row survived as `revoked`.
- The invitation token never appeared in the server log; the path was recorded
  as `/v1/invitations/[redacted]/accept`.
- Mobile: `tsc --noEmit` clean, `expo lint` clean, 34 Jest tests pass.

## Completed — Phase 4

| Area | Where |
|------|-------|
| Task domain | `internal/tasks/task.go` — statuses, transitions, derived overdue |
| Recurrence | RRULE subset, timezone-aware expansion (moved to `internal/recurrence` in Phase 5) |
| Task API | `GET/POST /v1/seniors/{id}/tasks`, `GET/PATCH/DELETE /v1/tasks/{id}` |
| Complete / skip | `POST /v1/tasks/{id}/complete`, `POST /v1/tasks/{id}/skip` |
| Recurring routines | `GET/PATCH /v1/seniors/{id}/tasks/templates/{id}` |
| Assigned to me | `GET /v1/tasks` — across every circle the caller belongs to |
| Senior timezone | `senior_profiles.timezone`, validated against tzdata |
| Database | migration `0004_care_tasks` |
| Offline | `src/lib/offline/` — SQLite cache and mutation queue |
| Mobile | today/upcoming/missed list, create flow, task detail, home "yours to do" |

### Verified end to end

- `go test -race -count=1 ./...` green across 16 packages.
- Migrations applied to a brand-new database, then the full suite re-run
  against it.
- The authorization matrix is automated rather than checked by hand: an
  intruder gets 404 on reading, editing, completing, skipping, cancelling and
  listing tasks; a caregiver with `tasks.complete` but not `tasks.manage` may
  complete but gets 404 on create, edit and cancel; `tasks.view` alone gets 404
  on complete and skip; a task in another circle is unreachable by ID.
- Completing twice over HTTP succeeds both times with an unchanged timestamp;
  skipping an already-completed task returns `CONFLICT` and the completion
  stands.
- An overdue task reports `overdue` while its stored status is still `pending`.
- The stored recurrence rule never appears in a response body.
- Mobile: `tsc --noEmit` clean, `expo lint` clean, 75 Jest tests pass.

## Completed — Phase 5

| Area | Where |
|------|-------|
| Medication domain | `internal/medications/medication.go` — statuses, transitions, derived missed |
| Schedules | one schedule per time of day; "twice a day" is two rows |
| Medication API | `GET/POST /v1/seniors/{id}/medications`, `GET/PATCH /v1/medications/{id}` |
| Today's medication | `GET /v1/seniors/{id}/medications/doses?scope=today\|upcoming\|missed\|window` |
| Take / skip | `POST /v1/medications/{id}/instances/{instanceId}/take` and `/skip` |
| Schedules API | `GET/POST /v1/medications/{id}/schedules`, `PATCH .../schedules/{id}` |
| One-off dose | `POST /v1/medications/{id}/doses` |
| History | `GET /v1/medications/{id}/instances` — keyset paged, newest first |
| Database | migration `0005_medications` |
| Shared recurrence | `internal/recurrence` — extracted from Phase 4, used by both domains |
| Offline | medication doses share Phase 4's queue; today's doses cached to SQLite |
| Mobile | today/upcoming/missed doses, medication list, create, detail with history, edit |

### Verified end to end

- `go test -race -count=1 ./...` green across 17 packages.
- Migrations applied to a brand-new database, then the medication and task
  suites re-run against it.
- The authorization matrix is automated: a stranger gets 404 on reading,
  editing, scheduling, recording and listing medication, and the refusal never
  names the medicine; a member with `medications.view` alone gets 404 on edit,
  schedule, take and skip; `medications.record` without `medications.manage` may
  take a dose but gets 404 on edit; a revoked member loses access immediately; a
  medication in another circle is unreachable by ID; a dose cannot be recorded
  through a different medication.
- The actor comes from the session: a body naming `takenBy` is refused outright
  (400), and the recorded actor is the authenticated caller.
- Taking twice over HTTP succeeds both times with an unchanged timestamp;
  skipping an already-taken dose returns `CONFLICT` and the first outcome
  stands.
- A dose past its window reports `missed` while its stored status is still
  `pending`, so it can be taken late.
- Missed doses are found after three days with nobody opening the app — the
  query generates the recent past before reading it.
- Editing a dosage leaves a dose already taken saying what it said; upcoming
  doses pick up the new value.
- The stored recurrence rule never appears in a response body.
- Mobile: `tsc --noEmit` clean, 117 Jest tests pass, Prettier clean.

## Completed — Phase 6

| Area | Where |
|------|-------|
| Appointment domain | `internal/appointments/appointment.go` — statuses, kinds, transitions |
| Appointment API | `GET/POST /v1/seniors/{id}/appointments`, `GET/PATCH /v1/appointments/{id}` |
| Views | `?scope=upcoming` (default), `today`, `past` — one endpoint, one envelope |
| Cancel / complete | `POST /v1/appointments/{id}/cancel` and `/complete` |
| History | `?scope=past&cursor=` — keyset paged, newest first |
| Assignment | `assignedUserId`, validated against active circle membership |
| Database | migration `0006_appointments` |
| Shared paging | `internal/paging` — extracted from Phase 5, used by both histories |
| Offline | upcoming appointments cached to SQLite for reading; no queued mutations |
| Mobile | upcoming/today/past lists, create, detail with cancel and complete, edit |

### Verified end to end

- `go test -race -count=1 ./...` green across 19 packages.
- All six migrations applied to a brand-new database, then the whole suite
  re-run against it.
- The authorization matrix is automated: a stranger gets the same 404 for an
  appointment that exists and one that does not, so the refusal never reveals
  that another family's relative has a hospital visit; a member with
  `appointments.view` alone gets 404 on create, edit, cancel and complete, and
  the appointment is verified unchanged afterwards; a revoked member loses both
  read and list access immediately.
- The creator comes from the session: a body naming `createdBy` is refused
  outright (400, unknown field), and the stored creator is the caller.
- Cancelling twice over HTTP succeeds both times, keeps the first actor and the
  first timestamp, and completing an already-cancelled appointment returns
  `CONFLICT` — so the outcome recorded first stands.
- Editing an appointment that has been completed or cancelled returns
  `CONFLICT` and the stored row is verified untouched.
- An edit that only moves the time is verified to leave the title, provider,
  location, notes and creator exactly as they were.
- Moving an appointment past its own previous end time succeeds: the end is
  checked against the start the appointment will have, not the one it is being
  moved away from.
- "Today" is the senior's own day: appointments ten minutes after and ten
  minutes before midnight in `Asia/Karachi` are both inside it, and the ones ten
  minutes outside are not.
- The history cursor walks five appointments — two sharing an instant — in pages
  of two, and every one appears exactly once.
- The database refuses an appointment that ends before it starts, one with a
  blank title, and one with an unrecognised kind.
- A cancelled appointment stays in both the upcoming list and the history.
- Mobile: `tsc --noEmit` clean, `expo lint` clean, 150 Jest tests pass across 16
  suites, Prettier clean.

## Completed — Phase 7

| Area | Where |
|------|-------|
| CareEvent domain | `internal/careevents/event.go` — vocabulary, entity kinds, metadata |
| Recorder | `internal/careevents/recorder.go` — one transaction boundary for every domain |
| Transaction plumbing | `internal/database/transaction.go` — `Querier` and `InTx`; repositories gained `WithTx` |
| Activity API | `GET /v1/seniors/{id}/activity?cursor=&limit=` — keyset paged, newest first |
| Task events | `TASK_CREATED`, `TASK_COMPLETED`, `TASK_SKIPPED` |
| Medication events | `MEDICATION_CREATED`, `MEDICATION_TAKEN`, `MEDICATION_SKIPPED` |
| Appointment events | `APPOINTMENT_CREATED`, `APPOINTMENT_COMPLETED`, `APPOINTMENT_CANCELLED` |
| Care-circle events | `MEMBER_INVITED`, `MEMBER_JOINED`, `MEMBER_REVOKED` |
| Database | migration `0007_care_events` |
| Rendering | `packages/contracts/src/care-event-labels.ts` — one place for every sentence |
| Mobile | activity timeline, grouped by the senior's day, paged, with empty/loading/error states |

### Verified end to end

- `go test -race -count=1 ./...` green across 20 packages.
- All seven migrations applied to a brand-new database, then the whole suite
  re-run against it.
- Every Phase 2–6 test still passes unchanged with events now being written
  inside their transactions, which is the strongest evidence the integration did
  not alter domain behaviour.
- Transactional consistency is tested in both directions: an event the database
  refuses rolls back the appointment that was written beside it, and an
  appointment the database refuses leaves no event behind.
- Retrying a completion three times produces one `TASK_COMPLETED`. The existing
  state machine already reports the second attempt as a repeat, and the event is
  written only when it is not — so no second idempotency mechanism exists.
- A refused action records nothing: skipping an already-completed task returns
  `CONFLICT` and writes no `TASK_SKIPPED`.
- A recurring task writes one `TASK_CREATED`, not one per generated occurrence.
- The keyset cursor walks seven events — four sharing a single instant — in
  pages of two, and every one appears exactly once.
- Authorization is tested rather than asserted: a stranger gets the same 404 for
  a senior with activity and one that does not exist; a member holding
  everything except `activity.view` gets 404; a revoked member who could read
  the timeline a moment earlier gets 404 afterwards; one circle's events never
  appear in another's.
- There is no way to fabricate an event: `POST /v1/care-events`, and POST, PUT
  and DELETE against the activity route, are all refused, and the timeline is
  verified empty afterwards.
- The database refuses an undocumented event type and an unrecognised entity
  type. A test also reads the migration and asserts the CHECK constraint lists
  exactly the Go vocabulary, so the two cannot drift.
- A test asserts that no code path emits the three documented-but-unemitted
  types, so "not yet" cannot quietly become "nobody noticed".
- Mobile: `tsc --noEmit` clean, `expo lint` clean, 169 Jest tests across 18
  suites, Prettier clean.

## Completed — Phase 8

| Area | Where |
|------|-------|
| Notification domain | `internal/notifications/notification.go` — reminder types, platforms, preferences, lead times |
| Reminder plan | `internal/notifications/reminder.go` — projection of care through authorization and preferences |
| Preferences API | `GET` / `PATCH /v1/notifications/preferences` |
| Device registration | `POST /v1/notifications/devices`, `DELETE /v1/notifications/devices/{deviceId}` |
| Reminder plan API | `GET /v1/notifications/reminders` |
| Schedule adapters | `internal/server/adapters.go` — tasks, medication, appointments, care circle |
| Database | migration `0008_notifications` — preferences and devices; deliberately no reminder table |
| Wording | `packages/contracts/src/notification-labels.ts` — one place for every notification sentence |
| Mobile scheduling | `src/features/notifications/` — reconciliation against the OS scheduler |
| Mobile settings | `src/app/settings/notifications.tsx` — screen 25 of docs/13 |

### Verified end to end

- `go test -race -count=1 ./...` green across 21 packages.
- All eight migrations applied to a brand-new database, the whole suite re-run
  against it, and the database dropped afterwards.
- Every Phase 2–7 test still passes unchanged.
- The three documented lead times are asserted literally against the brief's own
  worked examples: a dose at 08:00 reminds at 07:45, a task due 09:00 at 08:45,
  an appointment at 14:00 at 13:00.
- Idempotency is tested as identity rather than as behaviour: planning twice
  produces byte-identical reminder ids, two users get different ids for the same
  dose, and moving a dose changes its id — which is what makes the device cancel
  the stale reminder rather than keep it.
- Cancellation is tested through the domain, not through a notification API:
  cancelling an appointment, taking a dose, and revoking a caregiver each empty
  the relevant reminders from the plan without anything cancelling them
  explicitly.
- Authorization: a stranger's plan is empty; a caregiver holding
  `appointments.view` but not `medications.view` gets appointment reminders and
  no dose reminders; a revoked caregiver's plan empties while the owner's is
  untouched; an assigned appointment reminds only its assignee.
- Devices: re-registering keeps one row and refreshes the token; a registration
  carrying no token does not erase a known one; deactivation keeps the row and
  drops the token; signing back in reactivates the same row; two users may share
  one device identifier; another user's device id answers 404, not 403.
- A test asserts the push token never appears in any response body, and another
  asserts no reminder response contains a medicine name, a dosage, or any title
  or body field at all.
- The plan is capped at 50 reminders and the survivors are the soonest, which is
  asserted against a circle with four daily tasks and 38 doses.
- Mobile: `tsc --noEmit` clean, `expo lint` clean, 221 Jest tests across 26
  suites, Prettier clean. Reconciliation is tested as a pure function: running
  it twice schedules nothing the second time, an empty plan cancels everything,
  and a notification scheduled beyond the plan's horizon is left alone.


## Completed — Phase 9

The final MVP phase: no new domains, one integration and polish pass over
Phases 1–8.

| Area | Where |
|------|-------|
| End-to-end journey test | `internal/server/journey_integration_test.go` — a mixed circle, in order, over real HTTP |
| Circle administration fix | `internal/seniors/service.go` — the creator holds `members.manage` |
| Senior dashboard | `app/seniors/[seniorId]/index.tsx` — today's care, summarised and linked |
| First-time guidance | the same screen — "Getting started", showing only permitted steps |
| Access lost while open | `lib/query-client.ts` — a senior's cache is dropped when the API stops recognising the caller |
| Deep links before sign-in | `stores/ui-store.ts` + `features/notifications/use-reminder-sync.ts` |
| Environment documentation | `apps/mobile/.env.example` — notification configuration |

### The bug this phase existed to find

A family circle had **nobody who could ever revoke a member**.

The daughter who sets up her mother's care is a `family_member`, and
`members.manage` is deliberately not a family-member default — a relative
invited to help should not be able to remove the person who invited them. The
mother has no account to hold the permission either, and an invitation can only
delegate what the inviter already has, so the permission could not enter the
circle by any route. A professional caregiver's access to somebody's medical
information could be granted and never withdrawn.

Nothing in Phases 1–8 could have caught it: every phase tested its own domain,
and the gap only appears when circle creation, invitation, and revocation are
walked in sequence. That sequence is now a test.

The fix follows docs/01's Ownership section, which asks that creator, senior,
member, and permission stay four different things so coordination can later be
transferred: the **creator** of a circle holds `members.manage`, stored on their
relationship like every other permission. Role defaults are unchanged, so an
invited family member still does not administer the circle — and handing
coordination to somebody else is now just granting them the permission.

### Verified end to end

- `go test -race -count=1 ./...` green across 21 packages.
- All eight migrations applied to a brand-new database — 46 indexes, 38 check
  constraints — the whole suite re-run against it, then the database dropped.
- The MVP journey runs as one test, in order, entirely over HTTP: daughter
  creates the circle → adds a recurring task, a medication with a schedule, and
  an appointment → a stranger gets 404 on every route and an empty reminder plan
  → son invited and joins → caregiver invited with a deliberately narrow
  permission set → caregiver completes a task, records a dose, reads the
  appointment → is refused profile edits, prescription changes, cancellation,
  invitations, and the member list → her work appears in the family's timeline →
  she has reminders → she is revoked → every route answers 404, her senior list
  empties, her reminders stop → **her recorded care survives**, and the dose she
  gave is still marked as taken.
- Solo self-care is a second journey, asserting it needs nobody else: create own
  profile, add task, medication and appointment, take a dose, read own activity,
  receive own reminders.
- The unauthenticated invitation preview is asserted not to leak the senior's
  phone number, medication, or appointments.
- Mobile: `tsc --noEmit` clean, `expo lint` clean, **246 tests across 30
  suites**, Prettier clean.
- Dashboard tests pin the three failure modes that matter: a section the reader
  may not see is absent entirely, a failed section shows a retry rather than
  blank space, and a raw server error (`SQLSTATE …`) never reaches the screen.


## Completed — Phase 10

**Google sign-in, behind the existing authentication abstraction.** Screens call
`useAuthActions()`; nothing outside `src/features/auth` touches
`supabase.auth.signInWithOAuth`, and nothing downstream of the session knows
which provider was used. `SessionProvider`, session persistence, token refresh,
and sign-out are untouched.

- `src/features/auth/google.ts` (iOS, Android) opens Google with
  `WebBrowser.openAuthSessionAsync`, reads the authorization code off the
  `meracare://auth/callback` deep link, and exchanges it with the PKCE verifier
  the client stored. `google.web.ts` navigates the page instead and lets
  `detectSessionInUrl` do the exchange. The two share a result type; the public
  interface is identical, and Metro picks the right file per platform — verified
  by exporting all three bundles and confirming `openAuthSessionAsync` is absent
  from the web bundle.
- `flowType: 'pkce'` is now explicit in `src/lib/supabase.ts`. No token is ever
  returned in a redirect URL.
- `src/app/auth/callback.tsx` is where the redirect lands; it waits for the
  session and hands over to `/home`, or to `/sign-in` if the exchange failed.
- `src/components/ui/google-button.tsx` uses Google's own mark, keeps the 48dp
  touch target, and carries pressed, loading, and disabled states.
  `useAuthActions` tracks *which* action is pending, so one tap starts exactly
  one flow and the email button never spins for a Google sign-in.
- Cancellation is a distinct outcome from failure throughout: backing out of the
  Google screen, dismissing the sheet, and declining consent all return the
  person to sign-in with no error and no account.

**The Go API was not changed**, which was the point. It keys application users to
`auth.users.id`, so a Google identity linked to an existing account resolves to
the same `users.id` and the same care data. A test now pins that a
Google-provider token verifies on the same path as any other
(`internal/auth/token_test.go`). No Google verification, no Google tokens in
Postgres, no `google_users` table, no email matching in the API.

**Tests:** 6 native-flow cases (success, dismissal, declined consent, provider
error, malformed callback, failed exchange, failed start), 5 hook cases
(success, cancellation, failure, an unexpected throw replaced with a message
that leaks nothing, single-flight), and 5 screen cases. Mobile 263 passing (246
before this phase), Go suite green with `-race`, typecheck and lint clean, all
three bundles export.

**Not verified:** the genuine Google sign-in and the `GET /v1/me` call that
follows it. Blocker 6.

## Architectural Decisions Taken in Phase 1

These are implementation choices within the locked architecture — nothing in
docs/12 or docs/17 was changed.

1. **HTTP router: `go-chi/chi/v5`.** Standard library `http.ServeMux` cannot
   express the nested middleware groups the `/v1` authorization layer needs.
   chi is `http.Handler`-compatible, so it is not a framework lock-in.
2. **Database driver: `pgx/v5` with `pgxpool`.** The standard PostgreSQL driver
   for Go; no ORM, matching "avoid premature database abstraction" (docs/07).
3. **Migrations: embedded SQL with an in-repo runner** rather than a third-party
   CLI. Each migration runs in its own transaction with the bookkeeping insert,
   under a PostgreSQL advisory lock so concurrent deploys serialise. No extra
   tool to install; `go run ./cmd/migrate up`.
4. **JWT verification is asymmetric (JWKS) by default.** The API fetches the
   project's public signing keys from
   `<SUPABASE_URL>/auth/v1/.well-known/jwks.json` and verifies ES256/RS256
   tokens against them. It therefore holds no key capable of minting a token:
   a full compromise of the API's environment does not let an attacker
   impersonate a user. Keys are cached and refreshed only when a token presents
   an unknown `kid` — which is what a rotation looks like — with a one-minute
   throttle so forged tokens cannot amplify requests against Supabase. No
   background timer runs.

   `SUPABASE_JWT_MODE=legacy_hs256` selects the shared-secret verifier for
   projects still on the legacy JWT secret. Configuring a secret while in
   asymmetric mode is a startup error, so an unused forgeable key cannot sit
   unnoticed in a deployment.
5. **Application users are created lazily on first authenticated request.** The
   upsert is idempotent on `auth_user_id`, so no separate registration endpoint
   is needed and concurrent first requests collapse onto one row.
6. **Session storage is chunked SecureStore.** SecureStore warns above ~2KB and
   a Supabase session exceeds that, so values are split across numbered keys.
   docs/09 forbids plain local storage for tokens.
7. **Email/password sign-in first.** Apple and Google are the documented launch
   providers (docs/12) but require provider configuration in the Supabase
   project. Email keeps the foundation verifiable end to end meanwhile.
8. **`/healthz` and `/readyz` are separate.** Liveness performs no dependency
   checks so a database blip never restarts the API; readiness pings PostgreSQL.
9. **Authentication runs before routing inside `/v1`.** An unauthenticated
   caller gets 401 for any `/v1` path, so the API is not a probe for which
   endpoints exist.
10. **Local PostgreSQL uses host port 55432**, leaving the conventional 5432
    free for other projects on a developer machine.

## Architectural Decisions Taken in Phase 2

1. **Permissions are stored per relationship, not derived from the role.** A
   circle can narrow or widen one person's access without inventing new roles,
   which is what Phase 3's invitations will set. The role remains a label and a
   source of defaults; the stored set is what the API enforces.
2. **A denied senior returns 404, never 403.** Distinguishing "you may not see
   this" from "this does not exist" would let anyone enumerate the platform's
   seniors by probing IDs. Malformed IDs take the same path, without a query.
3. **Authorization is middleware, not a handler convention.** A handler obtains
   the senior it operates on from `authz.MustRelationship`, which only exists
   after a guard has run — so a route cannot accidentally skip the check and
   still function.
4. **A profile and its creator's membership are committed in one transaction.**
   A senior with no relationship would be visible to nobody, and unreachable
   even by the person who created it.
5. **The database mirrors the domain vocabulary.** CHECK constraints on role,
   status, and the permissions array mean the API cannot persist a value the
   domain does not define, whatever reaches the handler.
6. **The emergency contact is visible to every member, including professional
   caregivers.** It was initially withheld from caregivers as "private family
   information", which was the wrong reading of docs/02: a caregiver present
   with a senior is exactly who needs to know who to call. The restriction in
   docs/02 concerns information unrelated to the senior's care.
7. **Care events are deferred to Phase 7.** Creating a circle is a
   `MEMBER_JOINED` event under docs/04, but the events table belongs to Phase 7
   and the plan is explicit about building incrementally. No backfill will be
   needed, since there is no production data.
8. **`emergency_contact` is a single text field**, matching docs/03. It may
   split into a name and a phone number when the UI calls for it.

## Architectural Decisions Taken in Phase 3

1. **Only the token's hash is stored.** The raw token exists in memory and in
   the single response that delivers it. A database disclosure therefore hands
   an attacker no working invitations. Plain SHA-256 is the right primitive for
   a 256-bit random value; bcrypt-style hashes exist to slow brute force against
   low-entropy human secrets, and would only add cost per lookup here.
2. **Expiry is computed, never merely stored.** `EffectiveStatus` treats a
   lapsed invitation as expired at read time, so correctness never depends on a
   sweep having run. `ExpirePending` exists for housekeeping only.
3. **A token is consumed by a conditional UPDATE.** Acceptance requires the row
   to still be pending and unexpired, inside the same transaction that creates
   the membership. Two concurrent accepts therefore produce exactly one
   membership — the loser finds no row to update.
4. **Delegation distinguishes an explicit request from a default.** Asking for
   permissions the inviter lacks is an escalation attempt and is refused
   outright; omitting the list means "the usual", which is silently narrowed to
   what the inviter can confer. The same rule governs editing a member, so
   `members.invite` and `members.manage` are not routes to every other
   permission.
5. **The senior's own membership is immutable.** It cannot be edited or revoked
   through the member endpoints. Letting a member strip the senior's access
   would be the sharpest escalation the product offers.
6. **The `senior` role is not invitable.** A circle has exactly one senior,
   established when the profile is created.
7. **Invitation tokens are redacted from logs.** They travel in the URL, which
   is the natural shape for a link, so `httpx.RedactPath` replaces the segment
   before anything is written — docs/09 forbids logging credentials.
8. **The preview endpoint is unauthenticated but deliberately thin.** Someone
   with no account must be able to see what they are joining. It carries the
   senior's name, the inviter's name, the role and the permissions — no contact
   details, no care data, no member list.
9. **Acceptance is bound to the invited address.** The signed-in user's email
   must match the invitee, so obtaining a token is not by itself enough to join
   a care circle.
10. **A revoked member is invited back onto the same relationship row.** The
    membership is revived rather than replaced, so anything they authored keeps
    its author.
11. **Emergency contact is unchanged from Phase 2.** It follows `senior.view`
    like every other profile field. There is no role-based rule for it, which is
    what the Phase 3 brief asked to avoid.

## Blockers

1. **The real sign-in round trip is still unexercised — blocked on email
   confirmation.** Attempted again in Phase 9 (plans/phase9.md §28), with the
   same result: the account created during the Phase 8 attempt still answers
   `email_not_confirmed` to a password grant, so the project's configuration is
   unchanged. No further account was created. What is established, and what is
   not:

   - The JWKS endpoint is reachable and publishes an ES256 key.
   - The API, booted in asymmetric mode against the real project, logs
     `loaded Supabase signing keys` at startup, answers `/readyz` 200, and
     rejects both a missing and a forged bearer token with 401
     `UNAUTHENTICATED`. So the API genuinely verifies against the live
     project's published keys. Phase 8 re-ran this against the new
     `/v1/notifications/*` routes: all three answer 401 unauthenticated and 401
     to a forged token.
   - **What could not be done:** obtain a genuine token. The project requires
     email confirmation. A sign-up through `/auth/v1/signup` with the anon key
     returns 200 but no session, and the password grant then fails with
     `email_not_confirmed`. Confirming requires access to the recipient inbox,
     and minting or confirming a user directly would need the `service_role`
     key, which is deliberately not available here.
   - Steps 4 and 5 of §32 — the token resolving to the correct application user,
     and that same context reaching an authorized activity request — therefore
     remain **unverified**. They are not claimed.

   **To close it**, one of: disable email confirmation for the project (Auth →
   Providers → Email) and re-run the check; or confirm a real address and sign
   in with it once. Either takes a few minutes and needs no code change.

   Note: two unconfirmed accounts were created in the Supabase project during
   these attempts — `phase7.verification@gmail.com` and
   `phase8.verification@gmail.com`. Neither has a session or an application
   user; both can be deleted from the Auth dashboard.
2. **Migrations have not been applied to the hosted Supabase project.**
   `apps/api/.env` currently points `DATABASE_URL` at the local container, which
   is the right setting for development and is what every check in Phases 1–9
   ran against. The hosted setting is documented in `apps/api/.env.example`: the
   **session pooler**
   (`postgres://postgres.<ref>:<password>@aws-0-<region>.pooler.supabase.com:5432/postgres`),
   which works over IPv4 — unlike the direct `db.<ref>.supabase.co` host, which
   resolves to IPv6 only and has no route from this network.

   When the pooler string was configured during Phase 8 it failed authentication
   (`failed SASL auth ... for user "postgres"`, SQLSTATE 28P01), so the password
   in use was not the right one. Migrations `0001`–`0008` are therefore **not**
   on the hosted project.

   **To close it**, put the project's database password into `apps/api/.env`
   using the pooler form above — never into a commit or a chat — and run
   `pnpm api:migrate`.

3. **Push delivery and real-device notification behaviour are unverified.**
   No physical iOS or Android device and no push credentials were available, so
   plans/phase8.md §43 could not be satisfied and no claim is made that a
   notification has ever appeared on a phone. What *was* verified is everything
   below the OS boundary: the plan the server produces, the reconciliation
   arithmetic, and the exact scheduling calls made to `expo-notifications`
   (identifier, trigger instant, content, payload), asserted against a mocked
   module. The first run on a device should check three things in particular:
   that a scheduled reminder actually fires, that reopening the app does not
   duplicate it, and that tapping it opens the right screen.

   Push notifications are not implemented at all — there is no sender, and no
   Expo/APNs/FCM code anywhere in the repository. Device registrations are
   stored so that the phase which adds one has the tokens it needs.

4. **A revoked caregiver's already-scheduled reminders survive until their app
   next reconciles.** Server-side eligibility is authoritative and immediate:
   the moment a relationship is revoked, that user's plan is empty. But
   reminders are scheduled on the device, and the device only learns this when
   it next fetches the plan — on foreground, or on a query refetch. The window
   is bounded by the 7-day horizon and in practice by how soon the app is next
   opened. Closing it entirely would need a push to the revoked device telling
   it to clear, which is the push phase's work.

5. **Brand assets are still the Expo template placeholders.** `assets/images`
   holds the generated icon/splash. Real MeraCare icon, splash, and the first
   unDraw/Storyset illustrations are needed, along with `ASSET_LICENSES.md`
   (docs/18).

6. **Google sign-in cannot be exercised — the provider is not enabled.** The
   client is complete and the failure is confirmed to be configuration, not
   code. The project's own settings endpoint reports it:

   ```bash
   curl -s "$SUPABASE_URL/auth/v1/settings" -H "apikey: $SUPABASE_ANON_KEY"
   # → "external": { ..., "google": false, ... }
   ```

   and the authorize endpoint the app calls answers 400
   `Unsupported provider: provider is not enabled` — which is exactly the
   message the button surfaces today. Enabling it needs two consoles this
   environment has no access to: a Google Cloud OAuth client, and the Supabase
   dashboard to hold its secret. `docs/19-google-authentication.md` has the
   exact settings for both, including the redirect URLs.

   **Therefore unverified, and not claimed:** a real Google sign-in on any
   platform, the session it produces, `GET /v1/me` with that session, and the
   account-linking behaviour when a Google address matches an existing
   email/password account. The linking behaviour follows from Supabase's
   automatic identity linking and from the API keying users to `auth.users.id`
   rather than to an email, and the reasoning is written down — but it has not
   been observed. `docs/19-google-authentication.md` ends with the seven-step
   manual test to run once the consoles are configured.

   Note this compounds Blocker 1: even with Google enabled, a *linked* sign-in
   needs a confirmed email/password account to link to.

## Pending

Near-term, before or alongside Phase 2:

- **Apple sign-in** once the Supabase project has the provider configured
  (docs/12). Google is implemented (Phase 10) and slots in beside it in
  `use-auth-actions.ts`; Apple is the same shape.
- **OpenAPI document** for `/v1` (docs/05 lists OpenAPI as the contract format).
  Deferred until there are enough endpoints for it to be worth maintaining.
- **Inter typography** — the type scale is in place, but the Inter font files are
  not yet bundled, so the platform default renders at those sizes.

Later phases, unchanged from docs/14 and the Phase 1 plan:

- Phase 2 — User + SeniorProfile + CareRelationship, solo mode, senior dashboard
- Phase 3 — invitations and care circle
- Phase 4 — tasks; Phase 5 — medication; Phase 6 — appointments
- Phase 7 — care events and activity; Phase 8 — notifications
- Phase 9 — dashboards; Phase 10 — messaging
- Phase 11 — offline (`expo-sqlite`, sync queue); Phase 12 — quality

## Architectural Decisions Taken in Phase 4

1. **`overdue` is derived, never stored.** docs/03 lists it among the instance
   statuses, but a task is overdue exactly when its due time has passed and
   nobody has acted on it — a reading of the clock, not a state anybody moves it
   into. Storing it would mean the data is only true if a background job has
   run, and wrong whenever that job is behind. The database CHECK constraint
   deliberately does not accept the value.

2. **A one-time task is an instance with no template.** docs/03 defines a
   template as the rule for a *recurring* task, so a task that happens once has
   no rule to describe. One table answers "what is due today" either way.

3. **Occurrences carry their own title and description.** Renaming a routine
   must not rewrite what somebody already did, so the wording is copied at
   materialisation. This is what makes §18's history guarantee structural rather
   than a rule the code has to remember.

4. **Occurrences are materialised on read, for the window being read.** The
   alternative is a scheduler that must be running for the data to be correct,
   and a care schedule that silently stops when a job dies is worse than one
   that costs an insert to look at. A unique constraint on
   `(template_id, scheduled_for)` makes the write idempotent and safe under
   concurrent readers. Generation starts no earlier than the template's own
   creation, so asking for last month cannot invent a month of missed care.

5. **Editing a routine discards only future, untouched occurrences.** Anything
   completed, skipped, cancelled, or already due is left exactly as it was.

6. **Idempotency comes from the state machine, not a key table.** Completing an
   already-completed task returns the existing record unchanged; completing a
   *skipped* one is refused with `CONFLICT`. That satisfies both §27 and §28
   without a separate idempotency store, because the operation is already
   idempotent in the domain. The client still sends an `Idempotency-Key`, which
   a later phase can use if a non-idempotent mutation appears.

7. **A repeat of an action keeps the original actor and timestamp.** The retry
   must not re-attribute care to whoever's phone happened to reconnect first.

8. **Recurrence is stored as an RRULE subset** (`FREQ=DAILY`,
   `FREQ=WEEKLY;BYDAY=MO,WE,FR`) and parsed strictly. A subset of a standard
   rather than an invention, with somewhere to put monthly rules later. It is
   never sent to a client: the API returns `{frequency, weekdays}` and the app
   turns that into "Every weekday".

9. **The senior has a timezone; it decides when their day is.** Added to
   `senior_profiles` and validated against tzdata at write time. Recurrence
   walks local calendar dates rather than adding 24 hours, so a task due at
   09:00 stays at 09:00 across a daylight-saving change. `time/tzdata` is
   embedded in the binary so a minimal container image cannot silently reduce
   every zone to UTC.

10. **Completing a task does not require being its assignee.** docs/02 says a
    caregiver may "complete assigned tasks", but care given by whoever was
    present is still care given, and the record names who actually did it.
    `tasks.complete` is the check; assignment is a statement of intent.

11. **The offline queue is task-shaped, not general.** It carries the two
    mutations Phase 4 needs in the record shape docs/07 specifies. The replay
    decision — which failures are transient and which are final — is separated
    from the storage so it can be tested without the native module.

12. **A failed replay is kept, marked failed, not deleted.** An action the
    server will never accept has to be surfaced to somebody rather than
    disappearing.

## Architectural Decisions Taken in Phase 5

1. **Medication is its own domain, not a task with a label.** The tables and the
   package are separate, as docs/03 defines them. The shapes rhyme with care
   tasks because the same care problem recurs — a rule and the occurrences it
   produces — but a dose is not a chore: it is named after the medicine, carries
   its dosage into history, and one medicine can have several times of day,
   which a task template cannot express.

2. **The recurrence engine moved to `internal/recurrence`.** Medication
   schedules need the same rule grammar, the same timezone-correct expansion and
   the same wire shape, and a second copy of a daylight-saving-aware calendar
   walk is exactly the kind of duplication that goes wrong quietly. Phase 4's
   `tasks.Recurrence` and friends are now aliases, so no Phase 4 code or test
   changed. The contracts package mirrors the split (`recurrence.ts`,
   `datetime.ts`).

3. **"Twice a day" is two schedules, not one rule with two times.** It costs a
   row and buys the ability to stop the evening dose without touching the
   morning one, which is a real thing families do. A partial unique index
   prevents the same medicine being due twice at the same time under the same
   rule, and is partial on `active` so a stopped time can be restarted later.

4. **`missed` is derived, never stored.** A dose is missed exactly when its time
   and its grace period have passed with nobody acting on it — a reading of the
   clock, not a state anybody moves it into. The CHECK constraint does not
   recognise it. This is the Phase 3 `EffectiveStatus` precedent, applied a
   third time, and it means no job has to be running for the data to be true.

5. **A two-hour grace period before a dose counts as missed.** A dose is not
   missed at one minute past eight; somebody making breakfast is not late, and a
   screen that said so would train people to ignore it. Two hours is long enough
   to be a real window and short enough that a family checking in the same day
   still learns something.

6. **The missed query generates the recent past before reading it.** Doses are
   written when somebody reads the window they fall in, which leaves a gap: if
   nobody opened the app for a week, last Tuesday's dose was never written and
   so could never be reported missed. Generating a bounded lookback (14 days)
   before answering closes it without a background sweep.

7. **A missed dose stays `pending` underneath, so it can be taken late.** That
   is what people actually do, and the record should say so. The state machine
   therefore has no missed-specific transition at all.

8. **Doses snapshot the name and dosage they were scheduled with.** What was
   swallowed yesterday was 500 mg, and correcting the prescription today must
   not rewrite that. Editing a medication discards and regenerates only future
   *pending* doses; anything already due or acted on is untouched.

9. **Stopping a medicine is `active = false`, never a delete.** Its doses are
   care history. Stopping it discards future pending doses and ends generation;
   the medicine, its schedules and everything recorded remain readable.

10. **History is keyset-paged, not offset-paged.** A medicine taken twice a day
    for two years is fifteen hundred rows, and OFFSET makes the server count
    past every one of them to reach a page nobody has read. The cursor is opaque
    so clients cannot come to depend on the ordering.

11. **Medication doses joined the Phase 4 offline queue rather than starting
    one.** Two queues would replay a task completion and a dose taken a minute
    earlier in whichever order the passes happened to run. A dose is addressed
    through its medication, so both IDs travel in the entity ID as
    `medicationId/doseId` — one composite key is a smaller change than a schema
    every other entity would then carry.

12. **`medications.record` is separable from `medications.manage`.** A visiting
    caregiver can hand somebody their tablets without being able to change the
    prescription. This is enforced per relationship, not per role, and the
    automated matrix pins it.

## Architectural Decisions Taken in Phase 6

1. **An appointment is its own domain, not a task with a location.** They share
   a shape — a time and an outcome — and nothing else. A task is a routine the
   circle repeats and generates occurrences from; an appointment is already the
   concrete thing, booked once, at a place, with a provider. Modelling one as
   the other would mean either giving appointments a recurrence engine they have
   no use for, or asking a task to carry a provider it has no meaning for.

2. **No derived status, deliberately unlike tasks and medication.** A task has
   `overdue` and a dose has `missed`, both computed from the clock. An
   appointment has neither. A dose whose hour has passed really has been missed;
   an appointment whose hour has passed has not become anything — nobody knows
   whether the person went until somebody says so. Inventing "missed" here would
   be the app asserting care it cannot observe. Upcoming and past are read from
   `scheduled_at` against the clock, which is a question about the calendar
   rather than about the appointment.

3. **The status vocabulary is exactly `scheduled`, `completed`, `cancelled`.**
   docs/03 names these three; the CHECK constraint mirrors them and a unit test
   pins that `missed`, `overdue`, `past` and `pending` are all unrecognised.

4. **Whichever outcome is recorded first stands.** The state machine allows
   `scheduled → completed` and `scheduled → cancelled`, treats a repeat of the
   same action as a success that changes nothing, and refuses the contradiction
   in either direction with 409. The specification only forbids
   cancelled → completed; forbidding its mirror image falls out of the same rule
   and is the honest reading of "the server is authoritative". The guard is a
   conditional `UPDATE ... WHERE status = 'scheduled'`, so two devices acting at
   once resolve in the database rather than in the application.

5. **A settled appointment cannot be edited.** Once completed or cancelled it is
   no longer a plan somebody can revise; it is the record of what happened.
   `PATCH` answers 409 rather than silently rewriting it. The same conditional
   `UPDATE` does the refusing, and the service re-reads the row to distinguish
   "already settled" (409) from "no such appointment" (404).

6. **Cancelling and completing require `appointments.manage`.** docs/02 defines
   two appointment permissions and no third, and §11 of the brief is explicit
   that the existing vocabulary is the one to use. The consequence is real and
   intended: a professional caregiver's default permissions include
   `appointments.view` only, so a circle that wants a visiting caregiver to
   close off appointments grants them manage on that relationship. Inventing an
   `appointments.record` to mirror `medications.record` was the alternative and
   was rejected — permissions belong to the relationship, and the vocabulary is
   documented rather than ours to extend mid-phase.

7. **Both ends are instants, so midnight needs no special case.** `scheduled_at`
   and `ends_at` are `timestamptz`; an appointment running from 23:30 to 00:30
   is two instants and nothing else. Only "today" consults the senior's timezone,
   to find where their day begins — which is the one place a wall clock actually
   matters.

8. **One endpoint, one envelope.** docs/05 defines
   `GET /v1/seniors/{id}/appointments` and the brief forbids duplicate
   endpoints, so the view is chosen by `?scope=` rather than by a second
   `/history` route. Every scope returns `{items, nextCursor}`, with
   `nextCursor` null for the unpaged ones, so a client reading the response
   never has to know which kind of view it asked for.

9. **The keyset cursor moved to `internal/paging`.** Appointment history pages
   the same way medication history does, and this is the second caller. A second
   copy of an encoder and its decoder is the kind of duplication that drifts
   quietly, and a cursor two packages disagree about is a page of somebody
   else's care history. `internal/medications` now aliases the shared functions,
   so no Phase 5 behaviour changed and its suite proves it.

10. **One index, not three.** `appointments (senior_id, scheduled_at, id)`
    serves the ascending upcoming list and — read backwards — the descending
    history, with `id` present so the keyset page stays an index scan. There is
    deliberately no index on `status`, because nothing filters by it.

11. **Every status appears in every list.** A cancelled visit stays in both the
    upcoming list and the history. One that vanished on cancellation would look
    like an appointment nobody had mentioned, which is the opposite of what a
    care record is for. The UI distinguishes it three ways at once — the status
    in words, a tone, and a faded card — so status never depends on colour.

12. **Appointments are read offline and only changed online.** The upcoming list
    is cached to SQLite, because somebody in a car on the way to a hospital
    needs the address and that is exactly where the signal goes. No mutation is
    queued: §23 of the brief directs against it, and there is no second queue.
    A cancellation is optimistic with a rollback, and a lost connection rolls it
    back like any other failure — which is honest, because nothing will send it
    later.

13. **Cancelling is two taps.** It is the one action on the detail screen that
    cannot be undone, and a single button beside "Edit" is too easy to hit by
    mistake. The confirmation is an inline card rather than a platform alert, so
    it is large, readable, and behaves the same on both platforms.

## Architectural Decisions Taken in Phase 7

1. **One timeline, one table.** A family member asking "what happened
   yesterday?" is asking about their relative, not about tasks. Per-domain
   activity feeds would answer a question nobody has and would have to be merged
   in the client to answer the one everybody does.

2. **Events do not replace domain history.** The medication history still says
   what happened to every dose in medication's own terms, and the appointment
   history still pages through appointments. The timeline says what happened in
   the care, in the circle's terms. Both were kept; neither was rebuilt on the
   other.

3. **The vocabulary is the documentation's, and there is now only one.** A
   speculative list had stood in `packages/contracts/src/care.ts` since Phase 1,
   written before any of these domains existed. It had drifted — `MEMBER_REMOVED`
   for what the domain calls revoking, plus `APPOINTMENT_UPDATED` and
   `PERMISSIONS_CHANGED`, which docs/03 does not name and no action emits — and
   it was missing every creation event. Two vocabularies is exactly the parallel
   naming system the brief forbids, so the documented one was kept and the
   placeholder deleted. Nothing referenced it. **This is a deliberate change to
   a Phase 1 artefact**, recorded here rather than made silently.

4. **Three documented types are deliberately never emitted.** `TASK_MISSED` and
   `MEDICATION_MISSED` are derived from the clock, not performed by anybody —
   nothing writes "missed" anywhere in the system, precisely so no background
   sweep has to be alive for the data to be true (Phases 4 and 5). Emitting them
   would mean inventing the sweep those phases refused; they belong to Phase 8,
   where a notification is the thing that actually happens and has a time.
   `NOTE_ADDED` has no domain yet. All three stay in the vocabulary because the
   vocabulary is the documentation's, and a test asserts no code path produces
   one.

5. **A transaction, not a broker.** `careevents.Recorder` runs the domain change
   and its event in one PostgreSQL transaction. A completion with no event is a
   gap nobody would notice; an event with no completion is a timeline that lies.
   Kafka, NATS and the rest are ruled out by docs/12 and the brief, and would in
   any case give up the one guarantee that matters here — atomicity with the
   domain write.

6. **Repositories hold a `Querier`, not a pool.** `internal/database.Querier` is
   the subset shared by `pgxpool.Pool` and `pgx.Tx`, so one method body serves
   an ordinary request and a transactional one, and every repository gained a
   `WithTx`. That is what let five domains join a transaction without each
   growing a parallel set of `...Tx` methods. The invitation flow's bespoke
   `BeginTx` was removed in favour of the shared boundary, so there is now one
   way to start a transaction rather than two.

7. **Idempotency is the existing state machine, reused.** Tasks, doses and
   appointments already report a repeated action as a repeat rather than an
   error, which is what makes the offline queue safe. The event is written only
   when the action was not a repeat. No new idempotency mechanism exists,
   because the one that was already there answers exactly the right question.

8. **Metadata is a flat map of short labels, copied not referenced.** A task's
   title is stored on the event so that renaming the task next month does not
   rewrite what last week's entry says happened — which is what makes it a
   historical record. A CHECK constraint refuses anything that is not a JSON
   object, so it cannot drift into a copy of the row. Nothing sensitive goes in.

9. **The server sends no sentences.** The response carries the type, the actor,
   the instant and the metadata; every word a person reads is chosen in
   `care-event-labels.ts`. Wording can then change without a data migration, and
   a timeline entry cannot disagree with a future notification saying the same
   thing.

10. **`activity.view`, which docs/02 already defines.** No permission was
    invented. Family members and professional caregivers hold it by default, and
    a circle can withhold it from an individual relationship — which is tested.

11. **One index, `(senior_id, occurred_at DESC, id DESC)`.** `id` is in the
    index because events routinely share an instant: a domain change and its
    event are written in the same transaction, and a drained offline queue
    writes several at once. A timestamp-only cursor would drop or repeat one at
    every page boundary. Nothing filters by event type or actor, so neither is
    indexed.

12. **No `updated_at`, no `Update`, no `Delete`.** The table has no edit
    timestamp and the repository has no method to change a row. The absence is
    the enforcement.

13. **`actor_user_id` is nullable, and `ON DELETE SET NULL`.** NULL is reserved
    for an event no person performed; nothing fabricates a user to fill it. The
    event outlives the account that caused it, because losing the name is a
    smaller loss than losing the record.

14. **Category in words rather than an icon.** Each row carries "Task",
    "Medication", "Appointment" or "Care circle" as a caption. It fills the slot
    an icon would, and it is legible to somebody who cannot make out a
    sixteen-pixel glyph — which matters more here than iconography. The brand
    icon set is still outstanding regardless (see Blockers).

15. **The feed is deliberately quiet.** Most rows are neutral. A timeline where
    every entry is coloured is one where nothing stands out, and most care
    activity is ordinary — somebody did what they said they would.

## Architectural Decisions Taken in Phase 8

**Reminders are scheduled on the device, from a plan the server computes.**
docs/08 assigns exactly medication, task, and appointment reminders to local
notifications — "Server schedule → Mobile sync → OS local notification" — and
remote push to missed work, activity, invitations, and messages, none of which
Phase 8 covers. Local scheduling also means reminders arrive without a
connection and without MeraCare holding push credentials it does not have.

**There is no notifications table for reminders, and that is the design.** A
reminder is a consequence of care, not a record of it: the dose, the schedule,
the senior's timezone, and the user's preferences already determine it
completely. Storing it as well would create a second copy that can disagree with
the first — the medicine is stopped, but the reminder row still says 07:45.
Computing the plan on every request makes §22 (no stale reminders) and §31
(domain state wins) structurally true rather than rules the code must remember.
The cost is a plan query per refresh, which is three indexed window reads.

**The `SCHEDULED / SENT / CANCELLED / FAILED` vocabulary of §24 is not
implemented, deliberately.** Those states describe a server-side delivery
pipeline. Phase 8's server never delivers anything — the device does — so rows
in those states would be fiction. They belong to the push phase.

**Preferences cover three categories, not docs/08's seven.** docs/08 also lists
activity, messages, invitations, and escalation alerts. None has a delivery path
yet, and a switch that controls nothing is worse than a missing switch: the user
turns it off and still gets nothing, or on and still gets nothing, and either
way the app has lied. They arrive as columns on the same table in the phase that
sends them. This is the same judgement Phase 7 made about the stale
`CARE_EVENT_TYPES` placeholder.

**Lead times are fixed at 15 minutes, 15 minutes, and 1 hour.** Nothing in the
documentation defines offsets, and §12 forbids inventing options beyond the
documented MVP, so the three worked examples in the brief itself (§§12, 13, 14)
are treated as the specification. They are not configurable.

**Reminder identity is a UUIDv5 over recipient, type, subject, and firing
instant.** This is the entire idempotency mechanism, and no new one was
introduced. Because the identifier is a pure function of what the reminder
means, the device can use it directly as the OS notification identifier: asking
the OS to schedule an id it already holds replaces rather than duplicates.
Including the instant is what makes a rescheduled dose a *different* reminder,
so the stale one is cancelled instead of surviving.

**Assigned work reminds only its assignee; unassigned work reminds everyone who
can see it.** A circle of six should not all be told to drive Amma to the
clinic when one daughter is going. Medication has no assignee, so every member
with `medications.view` is reminded — which is what a family sharing care needs.

**No new permission vocabulary.** Reminder eligibility uses the existing
`tasks.view`, `medications.view`, and `appointments.view`. Being reminded about
a dose is a weaker act than reading the medication list, so a
`notifications.*` permission would have been an invention docs/02 does not
sanction.

**The plan is capped at 50 reminders.** iOS keeps at most 64 pending local
notifications per app and silently discards the rest, so a larger plan would be
partly imaginary. Fifty leaves headroom and is enforced server-side, soonest
first, with a stable tiebreak so the truncation does not shuffle between
refreshes.

**`internal/notifications` cannot reach a dosage.** It consumes a `Due`
— an id, an instant, an optional assignee — through per-domain interfaces
implemented by adapters at the composition root. Everything the notification
code could do with a medicine's name is something §§16, 17, and 47 forbid, so
the type system is where that is enforced rather than review.

**Notification wording lives in contracts, and the server sends none.** The
reminder response has no title and no body field at all; the device composes
both from `notification-labels.ts`. A lock screen has no idea who is looking at
it, so "Medication reminder" plus the senior's name and local time is the whole
vocabulary — no drug, no dosage, no condition.

**Routes are the app's business, not the server's.** The reminder says what it
is about; `src/features/notifications/routes.ts` turns that into a destination.
A server that named screens would need redeploying to move one.

**Preference changes are not queued for offline replay.** The offline queue
exists for care that was given, where losing the record loses something that
happened. A preference is a statement about the future; applied optimistically
offline it would show reminders as off while the server went on planning them.
§36's second branch — require connectivity — is what is implemented.

**The revocation window is bounded, not eliminated.** Server-side eligibility is
authoritative: a revoked caregiver's plan is empty immediately. But reminders
already scheduled on their device survive until that device next reconciles.
The 7-day horizon bounds it, reconciliation runs on every foreground, and
sign-out clears everything. This is a real residual window and is recorded in
Blockers rather than papered over.


## Architectural Decisions Taken in Phase 9

**The creator of a care circle holds `members.manage`.** See "The bug this phase
existed to find" above. Granted to the creator rather than to the role, and
stored on the relationship, so role defaults are untouched and coordination
remains transferable — which is what docs/01's Ownership section asks for.

**The senior dashboard summarises; Home lists people.** docs/13 has both
screens, and §9's ordering — senior, then today's care, then activity, then the
circle — is the senior dashboard's. Home stays a list of people plus the
caller's own assigned work, because a professional caregiver with six clients
opening the app to six days of care would have to scroll past all of it to find
their round.

**Losing access is handled by the cache, not by each screen.** A revoked
caregiver keeps whatever was already fetched until something asks again. A
single handler on the query cache watches for the API answering 404 or 401 on a
senior-scoped key and drops that senior's data. It removes only *inactive*
queries: removing an active one makes its observer refetch, fail, and arrive
back at the same handler — a loop that would hammer the API for as long as the
screen stayed open. The screen the user is looking at keeps its own query so it
can render its own "not available" message.

**A notification tapped before sign-in is held, not lost.** The destination goes
into the small UI store and is consumed once the session is through. It is
deliberately not persisted: a destination is only worth honouring in the moments
after the tap.

**Empty states distinguish "nothing today" from "nothing set up".** A circle
with no care at all gets "Getting started" with the first steps the *reader* is
permitted to take — a caregiver who can record care but not create it is never
invited to add a medicine the API would refuse.

**Error states never carry the underlying message.** `ApiError.message` is
always either the server's user-facing text or a generic fallback, and the raw
cause is kept in `cause` where nothing renders it. The Go side masks every
unexpected failure through `httpx.ErrInternal`. A test asserts a `SQLSTATE`
string cannot reach the dashboard.


## Deferred, and why

- **Care events.** Inviting and joining are `MEMBER_INVITED` and `MEMBER_JOINED`
  under docs/04, but the event infrastructure belongs to Phase 7 and the Phase 3
  brief is explicit that no parallel event system should be built. The
  invitation and membership tables already record who did what and when, so the
  events can be introduced without a data migration.
- **Invitation delivery.** The token is displayed to the inviter to pass on by
  hand. Emailing it belongs with the notification work in Phase 8.
- **Senior selection** (carried over from Phase 2). `selectedSeniorId` exists in
  the Zustand store and onboarding writes to it, but nothing reads it. It
  becomes load-bearing with the professional caregiver dashboard in Phase 9.
- **Task care events** (Phase 4). `TASK_COMPLETED` and `TASK_MISSED` belong to
  the Phase 7 timeline, and the Phase 4 brief forbids a parallel event system.
  Every fact those events need is already recorded on the occurrence — actor,
  timestamp, task, senior, and resulting state — so they can be introduced
  without a data migration.
- **Task reminders** (Phase 4). Scheduling metadata is in place (`due_time` plus
  the senior's timezone), but notifications belong to Phase 8. Nothing polls and
  no timer runs: the queue drains on app foreground, which is an event.
- **Date and time pickers** (Phase 4). The create flow takes `YYYY-MM-DD` and
  `HH:MM` in text fields. Native pickers are a usability requirement for an
  older adult and are worth doing properly rather than hurriedly; the API
  contract does not change when they arrive.
- **Offline for the rest of the app.** Only tasks and medication doses are
  cached and queued, as the briefs direct. Phase 11 generalises it.
- **Medication care events** (Phase 5). `MEDICATION_TAKEN` and
  `MEDICATION_MISSED` belong to the Phase 7 timeline, and the Phase 5 brief
  forbids a parallel event system. Every fact those events need is already on
  the dose — actor, timestamp, medication, senior, and resulting state — so they
  can be introduced without a data migration.
- **Medication reminders** (Phase 5). The scheduling foundation is in place: the
  senior's timezone, a schedule's wall-clock time, and `nextDoseAt` plus the
  upcoming-doses endpoint give Phase 8 everything it needs to schedule local
  notifications. Nothing polls and no timer runs; the queue drains on app
  foreground, which is an event.
- **Interval schedules ("every 12 hours")** (Phase 5). The MVP's documented
  schedule shape is a recurrence rule plus a time of day, and twice-daily is
  expressed as two schedules at 08:00 and 20:00 — which is what a person reads
  off a label anyway. A true interval rule drifts relative to the calendar and
  would need its own anchor; the RRULE subset has room for it when a phase
  actually asks.
- **Date and time pickers** (Phase 5, as Phase 4). The medication create and
  edit flows take `HH:MM` and `YYYY-MM-DD` in text fields, for the same reason.
- **Appointment care events** (Phase 6). `APPOINTMENT_CREATED` belongs to the
  Phase 7 timeline, and the Phase 6 brief forbids a parallel event system. Every
  fact those events need is already on the row — actor, timestamp, appointment,
  senior, and resulting state — so they can be introduced without a data
  migration.
- **Appointment reminders** (Phase 6). The data a later phase needs is in place:
  the start instant, the optional end, the senior's timezone, and the assignee
  who would be notified. Nothing polls and no timer runs.
- **Date and time pickers** (Phase 6, as Phases 4 and 5). The appointment create
  and edit flows take `YYYY-MM-DD` and `HH:MM` in text fields. §18 asks for
  pickers and also for existing project patterns; introducing a third way to
  enter a time in one domain while the other two use text fields would be worse
  than the shortcoming it fixed. When pickers land they land everywhere, and the
  API contract does not change.
- **Recurring appointments** (Phase 6). Nothing in the MVP documentation asks
  for them, and `internal/recurrence` is already shared and ready if a later
  phase does. Weekly physiotherapy is currently several appointments, which is
  also how a clinic books it.
- **Appointment conflict detection** (Phase 6). Nothing warns that two
  appointments overlap. It is a real convenience and no part of the brief; it
  needs a product decision about whether overlapping is an error or a fact.

- **Care events for editing** (Phase 7). Editing a task, a medication or an
  appointment records nothing. docs/03 names no such event, and the brief is
  explicit that not every database update deserves one — a timeline of every
  field change is a changelog, not an account of care. If a later phase wants
  "Sarah moved Thursday's appointment", it is one event type and one call.
- **Permission changes in the timeline** (Phase 7). The Phase 1 placeholder had
  a `PERMISSIONS_CHANGED` name, but no documentation defines it and docs/02
  files permission changes under auditability rather than activity. Membership
  events are recorded; changing what an existing member may do is not.
- **Cached activity offline** (Phase 7). The brief permits it ("where
  appropriate") rather than requiring it, and it was left out: the timeline is a
  record of the past that is never acted on, so a stale copy has none of the
  value that today's medication list has to somebody standing in a house with no
  signal. Nothing was queued either, which the brief does require.
- **Filtering the timeline** (Phase 7). No filter by domain, actor or date
  range. Nothing in the documentation asks for one, and the index is deliberately
  sized for the query that exists rather than for queries nobody makes.
- **Actor names in the response** (Phase 7). The API sends `actorUserId` and the
  client resolves it from the care-circle list it already holds, in one lookup
  built per render. That avoids a join on the hot path; if a later screen needs
  activity without the member list, the join is the obvious next step.

- **Push notifications** (Phase 8). No sender, no provider code, no delivery.
  docs/08 assigns push to missed work, caregiver activity, invitations, and
  messages — none of which Phase 8 covers — and the credentials to verify any of
  it were unavailable. Device registrations are stored so the phase that adds a
  sender starts with the tokens.
- **Escalation** (Phase 8). docs/08 describes reminder → grace period → overdue
  → notify assignee → optionally notify family. The first step exists; the rest
  is push, and the "overdue" and "missed" signals are still derived at read time
  rather than emitted, exactly as Phases 4, 5, and 7 left them.
- **The four other preference categories** (Phase 8). Activity, messages,
  invitations, and escalation alerts, from docs/08. Each arrives with the
  delivery path that makes it mean something, as a column on the existing table.
- **Configurable reminder offsets** (Phase 8). Lead times are fixed. §12 forbids
  inventing options beyond the documented MVP, and the documentation defines
  none, so "remind me 30 minutes before instead" is a product decision rather
  than an omission.
- **A notification inbox** (Phase 8). docs/03 defines a `Notification` entity
  with `read_at`, which is an in-app list of things that were sent. Nothing is
  sent from the server yet, so the table would have no rows and the screen no
  content. It belongs with push.
- **Server-side background jobs** (Phase 8). None were added. With reminders
  scheduled on the device from a plan computed on request, there is nothing for
  a worker to do, and §30 asks for the minimum. The first genuine need for one
  is push delivery retries.

## After the MVP

The MVP is complete and work stops here. Nothing below has been started, and
nothing below should be started without an explicit decision.

Two environment items are the only things standing between this and a running
product, and neither needs code: confirming a Supabase account (or disabling
email confirmation) so the real sign-in round trip can be verified, and putting
the database password in place so the migrations reach the hosted project. Both
are in Blockers with the exact steps.

The most valuable next piece of engineering is **push notifications**. It is
already scaffolded: `TASK_MISSED` and `MEDICATION_MISSED` sit in the care-event
vocabulary deliberately unemitted, `notification_devices` holds the tokens, and
adding a sender would also close the residual revocation window recorded in
Blockers. docs/08's escalation flow — reminder, grace period, overdue, notify
the assignee, optionally notify the family — is the shape it should take.

Everything else in the product documentation (messaging, notes, an activity
inbox, the Next.js web app) is post-MVP by the roadmap's own ordering.

## Running It

```bash
pnpm install

# Database
pnpm db:up                       # PostgreSQL on localhost:55432
cp apps/api/.env.example apps/api/.env
pnpm api:migrate

# API
pnpm api:run                     # http://localhost:8080

# Mobile
cp apps/mobile/.env.example apps/mobile/.env
pnpm mobile

# Checks
pnpm api:test                    # integration tests included via apps/api/.env
pnpm typecheck && pnpm test
```
