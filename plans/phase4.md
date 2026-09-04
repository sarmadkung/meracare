# GenXcare — Phase 4: Tasks & Daily Care

Phase 3 is complete and merged.

Implement **Phase 4 only**.

Do not start Phase 5 or implement unrelated features.

## Objective

Build the core task and daily-care system.

A senior's care circle must be able to create, assign, schedule, complete, skip, and monitor care tasks.

The system must support:

- one-time tasks
- recurring tasks
- task assignment
- task completion
- task skipping
- overdue tasks
- task status
- daily task views
- caregiver task workflows

The implementation must work for:

- Solo self-care
- Family care
- Professional caregivers
- Mixed family + professional care

Use the existing CareRelationship and permission model from Phases 2 and 3.

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
- `docs/08-notifications-and-background.md`
- `docs/09-security-privacy.md`
- `docs/10-testing-and-quality.md`
- `docs/11-performance-requirements.md`
- `docs/12-tech-stack.md`
- `docs/13-mvp-screen-map.md`
- `docs/14-mvp-roadmap-and-tasks.md`
- `docs/18-visual-theme-and-illustrations.md`

Also inspect the complete Phase 3 implementation before making changes.

Do not assume the implementation matches the documentation perfectly.

---

# 1. Task Domain Model

Implement the task model defined by the domain specification.

The architecture should distinguish between:

```text
Task Template
      ↓
Task Instance
```

A task template represents the definition of a recurring or reusable care task.

A task instance represents a concrete occurrence that can be completed or skipped.

Example:

```text
Task Template

"Take morning walk"
Every day at 09:00
        ↓
Task Instances

Aug 17 — 09:00
Aug 18 — 09:00
Aug 19 — 09:00
...
```

Follow the exact domain terminology already defined in the documentation.

Do not introduce a second task model if the existing specification already defines one.

---

# 2. Task Types

Support the task types required by the MVP specification.

At minimum, the architecture must support:

- one-time tasks
- recurring tasks

Examples:

```text
Take blood pressure
```

```text
Take a 20-minute walk
Every day
```

```text
Call doctor
August 20 at 14:00
```

Do not implement medication as a task.

Medication has its own domain and will be implemented in a later phase.

---

# 3. Task Assignment

A task may be assigned to an appropriate Care Circle member.

Examples:

```text
Senior
  ↓
Task
  ↓
Daughter
```

or:

```text
Senior
  ↓
Task
  ↓
Professional Caregiver
```

For solo mode:

```text
Senior / Self
  ↓
Task
  ↓
Self
```

The task assignment must reference the appropriate relationship/user according to the domain model.

Do not create a separate caregiver assignment architecture.

---

# 4. Task Permissions

Use the existing relationship permissions.

A user must only be able to perform task operations they are authorized to perform.

At minimum distinguish between permissions for:

- viewing tasks
- creating tasks
- editing tasks
- assigning tasks
- completing tasks

Use the exact permission vocabulary from `docs/02-permissions-and-authorization.md`.

Do not invent a second permission system.

---

# 5. Task Creation

Implement task creation.

A task should support the fields defined by the specification, such as:

- title
- description where applicable
- senior
- creator
- assignee
- schedule
- recurrence
- due time/date
- status
- timestamps

Only implement fields required by the domain specification.

Do not add speculative medical fields.

---

# 6. One-Time Tasks

Support one-time tasks.

Example:

```text
Task:
Call Dr. Ahmed

Date:
August 20

Time:
14:00
```

The system should create a concrete task instance for the specified occurrence.

The task should transition through the appropriate lifecycle.

---

# 7. Recurring Tasks

Support recurring tasks according to the documented recurrence model.

Examples:

```text
Every day
```

```text
Every weekday
```

```text
Every Monday
```

```text
Every Monday, Wednesday and Friday
```

```text
Every week
```

Use the recurrence representation defined by the domain specification.

Do not implement an overly complicated recurrence engine if the MVP only requires a limited set of recurrence patterns.

---

# 8. Task Instances

A recurring task must produce concrete task instances.

For example:

```text
Template
"Morning walk"
Daily at 09:00

Instances
├── Aug 17 — pending
├── Aug 18 — pending
├── Aug 19 — pending
└── Aug 20 — pending
```

Instances should have their own status.

Do not modify the template merely because an individual instance was completed.

Completing today's task must not automatically complete tomorrow's task.

---

# 9. Task Status

Use the status vocabulary defined in the domain specification.

At minimum, support the concepts of:

- pending
- completed
- skipped
- overdue

Do not invent conflicting status names if the domain already defines them.

Status transitions must be validated by the backend.

---

# 10. Task Completion

Implement task completion.

Example:

```text
Pending
   ↓
Completed
```

When a user completes a task:

- verify authorization
- verify the task belongs to an accessible senior
- verify the user has completion permission
- update the task instance
- record completion timestamp
- record the completing user

Do not trust a completion user ID supplied by the client.

The completing user must come from the authenticated session.

---

# 11. Task Skipping

Implement task skipping where supported by the specification.

Example:

```text
Pending
   ↓
Skipped
```

Record:

- who skipped it
- when it was skipped
- appropriate reason if required by the specification

Do not silently delete skipped tasks.

Skipped tasks are part of the care history.

---

# 12. Overdue Tasks

A task becomes overdue when its due time has passed without completion or skipping.

Do not require a continuously running JavaScript process to determine overdue status.

The system should derive overdue state from:

```text
current time
+
task due time
+
task status
```

The backend should be authoritative.

The mobile UI may calculate/display the current state locally for responsiveness, but must not use that as the source of truth.

Do not create a permanent database status transition merely because a screen was opened.

---

# 13. Task Queries

Implement APIs to retrieve:

- today's tasks
- upcoming tasks
- overdue tasks
- completed tasks
- tasks for a senior
- tasks assigned to the current user

Use the API specification for exact endpoint names.

Likely areas:

```text
GET /v1/seniors/{id}/tasks
POST /v1/seniors/{id}/tasks

GET /v1/tasks/{id}
PATCH /v1/tasks/{id}

POST /v1/tasks/{id}/complete
POST /v1/tasks/{id}/skip
```

Do not create duplicate endpoints if the existing API specification defines different routes.

---

# 14. Date and Time

Handle dates and times carefully.

The senior's relevant timezone must be respected when calculating:

- due dates
- recurring task occurrences
- today's tasks
- overdue state

Do not assume UTC is the user's display timezone.

Store timestamps using the database/time conventions established by the project.

Keep timezone information explicit where required.

Do not use device-local time as the sole source of truth for server-side scheduling.

---

# 15. Recurrence and Timezones

Recurring tasks must behave correctly around timezone boundaries.

Do not generate recurrence using naive string manipulation.

Use a well-tested date/time approach.

Pay particular attention to:

- midnight
- daylight-saving transitions where applicable
- timezone changes
- recurring weekly tasks

Do not add a heavy recurrence dependency unless necessary.

If the existing backend/date libraries provide a suitable implementation, prefer them.

---

# 16. Task API Authorization

Every task operation must verify:

```text
Authenticated User
        ↓
CareRelationship
        ↓
Senior
        ↓
Task
        ↓
Required Permission
```

A user must not be able to access a task simply by knowing its ID.

Maintain the existing unauthorized-resource behavior from Phases 2 and 3.

Do not leak whether another senior has a particular task.

---

# 17. Task Assignment Authorization

A user must not be able to assign a task to an arbitrary user.

The assignee must:

1. belong to the senior's Care Circle
2. have an appropriate active relationship
3. be eligible for the task assignment according to the domain rules

A revoked member must not receive new task assignments.

The backend must validate this.

---

# 18. Task Editing

Implement editing according to the documented permissions.

Editing a task template must not accidentally modify historical task instances that have already occurred.

Be careful with recurring tasks.

For example:

```text
Existing:

Every day at 09:00

Historical:
Aug 15
Aug 16

Future:
Aug 17
Aug 18
Aug 19
```

Changing the recurrence should not rewrite historical completed/skipped instances.

Follow the documented behavior for future instances.

---

# 19. Task Deletion

Do not permanently delete historical task information merely because a task is no longer active.

If the specification supports deletion/archive/deactivation:

- use the documented mechanism
- preserve relevant historical records

Do not introduce hard deletion without checking the domain model.

---

# 20. Mobile Task List

Implement the task UI required by the screen specification.

The main task experience should prioritize:

```text
Today's Care
```

Example:

```text
Today's Care

09:00
Take morning walk
Assigned to Sarah
[Complete]

11:00
Check blood pressure
Assigned to Ahmed
[Complete]

14:00
Call doctor
Assigned to Me
[Complete]
```

The exact UI should follow the screen map.

Do not make the interface dense.

---

# 21. Task Creation UI

Implement a task creation flow supporting the MVP fields.

The flow should allow the authorized user to:

1. Enter task title.
2. Add description if supported.
3. Choose date/time.
4. Choose recurrence where applicable.
5. Choose assignee where applicable.
6. Review.
7. Create task.

Use human-readable recurrence options.

Do not expose internal recurrence representations to users.

---

# 22. Task Detail

Implement the task detail screen where required.

Display:

- title
- description
- schedule
- recurrence
- assignee
- status
- completion information
- relevant timestamps

Actions should be permission-aware.

---

# 23. Quick Completion

Completing a task is a high-frequency action.

Optimize this flow.

The preferred experience is:

```text
Tap Complete
    ↓
Immediate UI update
    ↓
Mutation
    ↓
Server confirmation
```

Use TanStack Query optimistic updates where safe.

If the server rejects the mutation:

- restore the previous state
- show a clear error
- do not leave the UI falsely showing completion

---

# 24. TanStack Query

Use TanStack Query for:

- today's tasks
- upcoming tasks
- task details
- task templates
- task mutations

Use appropriate:

- query keys
- invalidation
- optimistic updates
- mutation rollback
- stale times

Do not put task server state into Zustand.

---

# 25. Zustand

Only use Zustand for local/UI state if necessary.

Examples:

- task filters
- temporary selected date
- UI preferences

Do not store task lists in Zustand.

---

# 26. Offline

Tasks are the first major care workflow that should use the offline architecture.

Implement offline support for:

- viewing cached tasks
- completing tasks
- skipping tasks

A task completion/skip action should be locally persisted and queued when offline.

Conceptually:

```text
User
 ↓
Complete
 ↓
SQLite
 ↓
Optimistic UI
 ↓
Sync Queue
 ↓
Go API
 ↓
Server
```

When connectivity returns:

- retry queued mutations
- handle successful synchronization
- remove successfully synchronized mutations
- retry transient failures
- surface permanent failures

Do not build a generic sync engine for every future entity.

Build the smallest reusable synchronization foundation required for tasks.

---

# 27. Idempotency

Task completion and skip mutations must be safe to retry.

A mutation should not create duplicate completion records or corrupt task state if the same request is delivered more than once.

Use an idempotency mechanism appropriate to the existing API architecture.

Do not rely only on the client to prevent duplicates.

---

# 28. Conflict Handling

Consider this scenario:

```text
Device A
    ↓
Complete task

Device B
    ↓
Skip same task
```

The backend must enforce valid state transitions.

Do not allow an already completed task to become skipped unless the domain specification explicitly permits it.

The server is authoritative.

Document the chosen conflict behavior.

---

# 29. Database

Create migrations for task-related entities according to the domain model.

Use:

- UUIDs
- foreign keys
- constraints
- indexes
- timestamps
- appropriate status constraints

Add indexes for common access patterns such as:

- senior
- assignee
- due time
- status
- active task templates
- recurring tasks

Do not add indexes without a query/use case.

---

# 30. Care Events

Do not implement the full CareEvent system if it is scheduled for Phase 7.

However, task completion and skip actions should preserve enough information to support future care events.

At minimum retain:

- actor
- timestamp
- task
- senior
- resulting state

Do not create a parallel event system.

---

# 31. Notifications

Do not implement the complete notification system in this phase.

Task scheduling should be designed so Phase 8 can later add reminders without changing the task domain.

Do not introduce continuous polling or JavaScript timers.

If the existing architecture requires task scheduling metadata, implement only that foundation.

---

# 32. Backend Tests

Add comprehensive tests for:

### Creation

- authorized user can create a task
- unauthorized user cannot create a task
- invalid assignee is rejected
- revoked member cannot be assigned

### Retrieval

- authorized user can view tasks
- unauthorized user cannot view another senior's tasks
- today's tasks are correctly returned
- upcoming tasks are correctly returned
- overdue tasks are correctly determined

### Completion

- authorized user can complete task
- unauthorized user cannot complete task
- completion records correct actor
- completion timestamp is correct
- duplicate completion is safe
- invalid state transition is rejected

### Skip

- authorized user can skip task
- unauthorized user cannot skip task
- skipped task retains history
- duplicate skip is safe
- invalid state transition is rejected

### Recurrence

- daily recurrence
- weekly recurrence
- selected weekdays where supported
- future instances
- historical instances remain unchanged

### Assignment

- valid Care Circle member can be assigned
- revoked member cannot be assigned
- unrelated user cannot be assigned

### Authorization

Test every task permission boundary.

---

# 33. Mobile Tests

Test:

1. Today's tasks load.
2. Empty task state works.
3. Loading state works.
4. Error state works.
5. Authorized user can create a task.
6. User can complete a task.
7. User can skip a task.
8. Completion updates the UI immediately.
9. Failed optimistic mutation rolls back.
10. Offline completion is queued.
11. Queued completion synchronizes.
12. Task detail displays correctly.
13. Unauthorized actions are hidden.

---

# 34. Visual Requirements

Follow:

`docs/18-visual-theme-and-illustrations.md`

Primary color:

```text
#0F766E
```

Use semantic design tokens.

Task UI should prioritize:

- large readable task titles
- clear due times
- obvious completion action
- clear status
- accessible touch targets
- minimal visual clutter

A task's completion action should be easy for an older adult to understand.

Do not rely on color alone to communicate status.

---

# 35. Performance

The task list may become one of the most frequently used screens.

Ensure:

- efficient list rendering
- stable keys
- minimal rerenders
- paginated history where appropriate
- cached today's tasks
- optimistic completion

Do not load a senior's entire task history into memory when only today's tasks are required.

Use virtualization for long lists.

---

# 36. Do Not Implement Yet

Do NOT implement:

- medication
- medication schedules
- medication reminders
- appointments
- messaging
- full care-event timeline
- advanced push notifications
- professional caregiver dashboard
- payroll
- billing
- AI
- wearable monitoring

Those belong to later phases.

Only implement the task/daily-care foundation.

---

# 37. Documentation

Update:

`docs/IMPLEMENTATION_STATUS.md`

Record:

- Phase 4 started
- task architecture
- database changes
- recurrence decisions
- authorization decisions
- offline/sync decisions
- completed functionality
- tests
- blockers
- deferred functionality
- next phase

If any architecture decision changes, document it and follow `AGENTS.md`.

---

# 38. Definition of Done

Phase 4 is complete when:

- Task templates are implemented where required.
- Task instances are implemented.
- One-time tasks work.
- Recurring tasks work.
- Tasks can be assigned.
- Assignment authorization works.
- Tasks can be viewed.
- Today's tasks work.
- Upcoming tasks work.
- Overdue tasks work.
- Tasks can be completed.
- Tasks can be skipped.
- Completion records the authenticated actor.
- Invalid state transitions are rejected.
- Historical task state is preserved.
- Task completion is idempotent.
- Offline task completion works.
- Offline task skipping works.
- Queued mutations synchronize correctly.
- Permission checks are enforced.
- Mobile task UI works.
- Task creation works.
- Task detail works.
- Optimistic completion works.
- Rollback works on failure.
- Backend tests pass.
- Mobile tests pass.
- Type checks pass.
- Lint passes.
- Database migrations work on a fresh database.
- No Phase 5+ functionality has been unnecessarily implemented.

When Phase 4 is complete, stop.

Do not automatically continue to Phase 5.
