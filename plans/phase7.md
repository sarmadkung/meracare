# GenxCare — Phase 7: Care Events & Activity Timeline

Phase 6 is complete and merged into `main`.

Implement **Phase 7 only**.

Do not start Phase 8 or implement unrelated features.

## Objective

Build the unified Care Event and Activity Timeline system.

CareEvents provide a chronological record of meaningful activity within a senior's care.

Events should cover the existing domains:

- Care Circle
- Tasks
- Medications
- Appointments

The Activity Timeline must allow authorized users to understand what has happened in a senior's care.

Do not create separate activity systems for individual domains.

---

## Before Starting

Read:

- `AGENTS.md`
- `docs/00-product-overview.md`
- `docs/01-roles-and-care-model.md`
- `docs/02-permissions-and-authorization.md`
- `docs/03-domain-model.md`
- `docs/04-care-events-and-workflows.md`
- `docs/05-api-and-backend-spec.md`
- `docs/06-mobile-architecture.md`
- `docs/07-database-and-sync.md`
- `docs/09-security-privacy.md`
- `docs/10-testing-and-quality.md`
- `docs/11-performance-requirements.md`
- `docs/12-tech-stack.md`
- `docs/13-mvp-screen-map.md`
- `docs/14-mvp-roadmap-and-tasks.md`
- `docs/18-visual-theme-and-illustrations.md`

Also inspect the implementations from Phases 2–6.

Use the documentation as the product/architecture source of truth and the existing code as the implementation baseline.

---

# 1. CareEvent Domain

Implement the CareEvent domain defined in `docs/04-care-events-and-workflows.md`.

A CareEvent represents a meaningful action that occurred within a senior's care.

Conceptually:

```text
Senior
  ↓
CareEvent
  ├── actor
  ├── event type
  ├── timestamp
  ├── related entity
  └── metadata
```

Each event must belong to a senior/care context.

---

# 2. Event Types

Use the exact event vocabulary defined by the documentation.

At minimum, integrate the meaningful events from existing domains, such as:

```text
MEMBER_INVITED
MEMBER_JOINED
MEMBER_REVOKED

TASK_CREATED
TASK_COMPLETED
TASK_SKIPPED

MEDICATION_CREATED
MEDICATION_TAKEN
MEDICATION_SKIPPED

APPOINTMENT_CREATED
APPOINTMENT_COMPLETED
APPOINTMENT_CANCELLED
```

If the documentation defines additional or different event names, follow the documentation.

Do not invent a parallel event naming system.

Do not create events for every database update.

Only meaningful domain actions should generate events.

---

# 3. Event Structure

An event must contain enough information to answer:

- What happened?
- Who performed it?
- When did it happen?
- Which senior was affected?
- Which entity was involved?
- What information is required to render the activity?

Use structured metadata where necessary.

Do not store presentation-ready sentences as the primary event data.

The mobile application should render human-readable descriptions.

---

# 4. Actor

When an action is performed by a user, record the authenticated actor.

Never accept an actor ID from the client.

For example:

```text
Authenticated User
       ↓
Complete Task
       ↓
CareEvent.actor_id
```

If a system-generated event has no human actor, represent that according to the domain model.

Do not fabricate a user as the actor.

---

# 5. Historical Integrity

CareEvents are historical records.

Once created, they must not normally be edited or deleted.

If the source entity is later:

- edited
- cancelled
- completed
- revoked
- deactivated

the original event remains unchanged.

Do not rewrite historical events.

---

# 6. Event Creation

Domain actions must create events as part of the same logical operation.

Example:

```text
Complete Task
    ↓
Update task
    ↓
Create TASK_COMPLETED event
```

Where both operations use the same database transaction, they must be transactionally consistent.

Do not allow a successful domain action to silently lose its corresponding event.

Do not create events from the mobile client.

---

# 7. Existing Domain Integration

Integrate CareEvents into the existing domains.

## Care Circle

Generate the documented membership events, including where applicable:

```text
MEMBER_INVITED
MEMBER_JOINED
MEMBER_REVOKED
```

Do not create another invitation/member history system.

## Tasks

Generate documented events such as:

```text
TASK_CREATED
TASK_COMPLETED
TASK_SKIPPED
```

Do not change existing task behavior unnecessarily.

## Medications

Generate documented medication events such as:

```text
MEDICATION_CREATED
MEDICATION_TAKEN
MEDICATION_SKIPPED
```

Do not replace medication history with CareEvents.

## Appointments

Generate documented appointment events such as:

```text
APPOINTMENT_CREATED
APPOINTMENT_COMPLETED
APPOINTMENT_CANCELLED
```

Do not create an appointment-specific activity timeline.

---

# 8. Event vs Domain History

CareEvents do not replace domain-specific history.

For example:

```text
Medication History
    ↓
Detailed medication information

CareEvent
    ↓
Cross-domain activity summary
```

Likewise:

```text
Task History
    ↓
Task-specific information

CareEvent
    ↓
Unified activity timeline
```

Keep existing domain history intact.

---

# 9. Event Metadata

Metadata must be structured and minimal.

For example:

```json
{
  "taskTitle": "Morning walk"
}
```

or:

```json
{
  "medicationName": "Metformin",
  "dosage": "500 mg"
}
```

Only include information required for the activity experience.

Do not copy entire database records into metadata.

Never store:

- passwords
- authentication tokens
- credentials
- unnecessary sensitive information

---

# 10. Sensitive Information

CareEvents may contain sensitive care information.

Activity access must follow the existing authorization model.

Do not create a generic "everyone can see everything" rule.

If `docs/02-permissions-and-authorization.md` defines a specific activity permission, use it.

If it does not, use the documented senior/care access rules rather than inventing a new permission without approval.

Do not expose sensitive information through event metadata unnecessarily.

---

# 11. Activity API

Implement the activity endpoint defined by:

`docs/05-api-and-backend-spec.md`

If the documentation defines a route such as:

```text
GET /v1/seniors/{id}/activity
```

use that route.

Do not create separate activity endpoints for tasks, medications, appointments, and members.

The Activity API should support:

- senior
- cursor pagination
- stable ordering
- newest-first results

---

# 12. Pagination

Use the existing cursor/keyset pagination implementation from `internal/paging`.

Do not introduce offset pagination for the main activity feed.

Support:

- page size
- next cursor
- stable ordering
- deterministic results

Use timestamp plus a unique stable identifier for ordering so events with identical timestamps cannot cause duplicates or gaps.

---

# 13. Event Ordering

Return newest events first.

Example:

```text
Today

10:42
Sarah completed Morning Walk

09:00
Ahmed marked medication as taken

08:30
Appointment was cancelled

Yesterday

18:15
Ahmed completed Evening Care
```

Use the senior/user's relevant timezone for date grouping.

Do not group dates using UTC dates alone.

---

# 14. Activity Timeline UI

Implement the Activity Timeline according to the screen specification.

Each event should communicate:

- what happened
- who performed it
- when it happened

Do not display raw event identifiers such as:

```text
TASK_COMPLETED
MEDICATION_TAKEN
```

Use human-readable labels.

---

# 15. Event Rendering

Create a centralized mobile event-rendering system.

Conceptually:

```text
Event Type
    ↓
Icon
Label
Description
Metadata
```

Keep event rendering separate from backend event creation.

Do not duplicate event descriptions across multiple screens.

Use the existing contract/label patterns from previous phases.

---

# 16. Date Grouping

Group events by date where appropriate:

```text
Today
Yesterday
Monday, August 17
Sunday, August 16
```

Use the application's relevant timezone.

Ensure events around midnight are grouped correctly.

---

# 17. Empty State

Provide a clean empty activity state.

Example:

```text
No activity yet

Care activity will appear here as you
and your care circle work together.
```

Use an approved unDraw or Storyset illustration only if it improves the empty state.

Do not add illustrations to every event.

---

# 18. TanStack Query

Use TanStack Query for:

- activity timeline
- paginated activity
- loading additional activity

Use the project's existing infinite/cursor query patterns.

Do not store activity server state in Zustand.

---

# 19. Offline

Do not create a separate offline queue for CareEvents.

Existing domain actions already use the offline architecture.

For example:

```text
Offline task completion
        ↓
Sync
        ↓
Server task mutation
        ↓
CareEvent created
```

The server-generated CareEvent is authoritative.

Do not permanently create fake CareEvents locally before the domain mutation is confirmed.

Cached activity may be displayed offline where appropriate.

---

# 20. Idempotency

CareEvents must not be duplicated when domain mutations are retried.

Example:

```text
Complete Task
    ↓
Network timeout
    ↓
Client retries
```

There must still be only one:

```text
TASK_COMPLETED
```

event.

Reuse the existing idempotency mechanisms.

Do not introduce another idempotency system.

---

# 21. API Security

Do not expose a generic client endpoint such as:

```text
POST /v1/care-events
```

CareEvents must be generated by trusted domain operations.

Clients should call:

```text
POST /v1/tasks/{id}/complete
```

rather than:

```text
POST /v1/care-events
```

This prevents users from fabricating activity.

---

# 22. Authorization

Activity access must follow:

```text
Authenticated User
        ↓
CareRelationship
        ↓
Senior
        ↓
Activity Permission
```

A stranger must not be able to enumerate another senior's activity.

Maintain the existing unauthorized-resource behavior from Phases 2 and 3.

Do not leak whether another senior has activity.

---

# 23. CareEvent Service

Use a small service/abstraction for recording events.

For example:

```text
CareEventRecorder
```

or an equivalent architecture that fits the existing codebase.

Domain services should be able to record events transactionally.

Do not create:

- Kafka
- NATS
- RabbitMQ
- Redis Streams
- microservices
- distributed event infrastructure

The Go modular monolith and PostgreSQL transaction are sufficient for the MVP.

---

# 24. Database

Create the required CareEvent migration.

The table should support:

- UUID
- senior reference
- actor reference where applicable
- event type
- timestamp
- related entity type
- related entity ID
- structured metadata
- created timestamp

Use appropriate foreign keys and constraints according to the existing database architecture.

Add indexes for the actual activity query pattern:

```text
senior + newest events
```

Do not add unnecessary indexes.

---

# 25. Database Integrity

The database must reject event types outside the documented vocabulary.

The client must never be able to directly create arbitrary events.

Do not allow arbitrary event types or metadata structures that bypass domain validation.

---

# 26. Transactional Consistency

For each integrated domain action:

```text
Domain Mutation
      +
CareEvent
```

must succeed or fail together where both are handled in the same database transaction.

Examples:

```text
Complete Task
    ├── task update
    └── TASK_COMPLETED event
```

```text
Take Medication
    ├── dose update
    └── MEDICATION_TAKEN event
```

```text
Cancel Appointment
    ├── appointment update
    └── APPOINTMENT_CANCELLED event
```

If event creation fails, the domain mutation should not silently succeed.

---

# 27. Authorization Tests

Test:

- authorized user can view activity
- unauthorized user cannot view activity
- stranger receives the expected unauthorized response
- revoked member loses access where required
- event data cannot leak across seniors
- client cannot fabricate events
- actor cannot be supplied by the client

---

# 28. Backend Tests

Test at minimum:

### Event creation

- task completion creates event
- task skip creates event
- medication creation creates event where specified
- medication taken creates event
- medication skip creates event
- appointment creation creates event
- appointment completion creates event
- appointment cancellation creates event
- member invitation creates event where specified
- member joining creates event where specified
- member revocation creates event where specified

### Transactionality

- failed domain mutation creates no event
- failed event creation does not leave the domain mutation committed
- retry does not create duplicate events

### Pagination

- first page works
- next cursor works
- no duplicate events
- no missing events
- stable ordering
- identical timestamps are handled correctly

### Integrity

- invalid event type is rejected
- client cannot directly create an event
- actor comes from authentication

Run:

```bash
go test -race -count=1 ./...
```

Also run the complete integration suite against a fresh database.

---

# 29. Mobile Tests

Test:

1. Activity timeline loads.
2. Empty state works.
3. Loading state works.
4. Error state works.
5. Events render correctly.
6. Events are grouped by date.
7. Newest events appear first.
8. Pagination works.
9. Load-more works.
10. No duplicate events appear.
11. Event labels are human-readable.
12. Unauthorized activity is not displayed.
13. Cached activity works where supported.

---

# 30. Performance

The activity timeline can grow indefinitely.

Use:

- cursor pagination
- bounded page sizes
- indexed queries
- efficient response serialization
- virtualized mobile lists

Do not load the entire activity history.

Do not perform N+1 database queries for event rendering.

If actor or related-entity information is required, retrieve it efficiently.

---

# 31. Visual Requirements

Follow:

`docs/18-visual-theme-and-illustrations.md`

Primary color:

```text
#0F766E
```

The activity timeline should be:

- calm
- readable
- lightweight
- accessible
- easy to scan

Do not make every event look like an alert.

Do not communicate event meaning through color alone.

---

# 32. Real Supabase Authentication Check

There is an existing deferred item from earlier phases:

> A genuine Supabase authentication token has not yet been verified end-to-end against `/v1/me`.

Do not redesign authentication as part of Phase 7.

However, before declaring Phase 7 complete:

1. Verify whether the real Supabase authentication round trip can now be tested.
2. If environment/configuration is available, run a real authenticated request through `/v1/me`.
3. Verify the token is accepted by the Go API.
4. Verify the authenticated user resolves to the correct application user.
5. Verify the same authentication context works with an authorized senior/activity request.

If the environment is still unavailable, do not fabricate a successful result.

Record the limitation clearly in:

`docs/IMPLEMENTATION_STATUS.md`

This is a verification task, not a reason to redesign the authentication architecture.

---

# 33. Supabase Database Connection

The current environment has a known issue where `DATABASE_URL` points to the Supabase direct connection and is unreachable from the current machine.

Do not change production/database architecture as part of Phase 7.

If needed for local verification, use the existing local database setup.

If the Supabase session-pooler connection is configured during this phase, verify migrations and API behavior against it.

Do not commit credentials.

Do not commit `.env` secrets.

Record any environment limitation in `docs/IMPLEMENTATION_STATUS.md`.

---

# 34. Do Not Implement Yet

Do NOT implement:

- notification center
- push notification infrastructure
- email notifications
- SMS
- messaging
- AI summaries
- AI recommendations
- analytics dashboards
- caregiver billing
- wearable integrations
- telemedicine

Those belong to later phases.

Only implement CareEvents and the Activity Timeline.

---

# 35. Documentation

Update:

`docs/IMPLEMENTATION_STATUS.md`

Record:

- Phase 7 status
- event model
- event vocabulary
- integrated domains
- transaction strategy
- idempotency strategy
- pagination strategy
- authorization behavior
- database changes
- Supabase verification result
- environment limitations
- completed functionality
- tests
- blockers
- deferred functionality
- next phase

Do not silently change architectural decisions.

---

# 36. Definition of Done

Phase 7 is complete when:

- CareEvent domain exists.
- Event vocabulary follows the documentation.
- CareEvents are created by trusted domain actions.
- Clients cannot fabricate events.
- Events contain the correct senior.
- Events record the authenticated actor where applicable.
- Events preserve historical information.
- Care Circle events work where specified.
- Task events work.
- Medication events work.
- Appointment events work.
- Events are transactionally consistent with domain actions.
- Duplicate events are prevented.
- Activity API works.
- Cursor pagination works.
- Stable event ordering works.
- Activity authorization works.
- Activity timeline UI works.
- Empty/loading/error states work.
- Activity loads incrementally.
- Offline domain mutations produce authoritative server-side events after synchronization.
- Database migrations work on a fresh database.
- Backend tests pass.
- Mobile tests pass.
- Type checks pass.
- Lint passes.
- Real Supabase authentication is verified if the environment permits it.
- Any remaining environment limitation is documented accurately.
- No Phase 8+ functionality has been unnecessarily implemented.

When Phase 7 is complete, stop.

Do not automatically continue to Phase 8.
