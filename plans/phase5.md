# GenXcare — Phase 5: Medication Management

Phase 4 is complete and merged.

Implement **Phase 5 only**.

Do not start Phase 6 or implement unrelated features.

## Objective

Build GenXcare's medication management system.

The system must support:

- medication records
- dosage information
- medication schedules
- recurring medication schedules
- medication instances
- medication status
- marking medication as taken
- marking medication as skipped
- missed medication detection
- medication history
- caregiver/family visibility
- solo self-care
- appropriate medication permissions
- medication reminders foundation

Medication must be implemented as its own domain.

Do not model medication as a normal task.

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

Also inspect the complete Phase 4 implementation before making changes.

---

# 1. Medication Domain

Medication is a separate domain from tasks.

Use the documented entities:

```text
Medication
    ↓
Medication Schedule
    ↓
Medication Instance
```

Conceptually:

```text
Medication

"Metformin"
500 mg
Tablet

        ↓

Schedule

Every day
08:00

        ↓

Instances

Aug 17 08:00
Aug 18 08:00
Aug 19 08:00
...
```

Follow the exact domain terminology already defined in the documentation.

Do not create a second medication model.

---

# 2. Medication Record

Implement the medication information required by the MVP specification.

Only collect fields explicitly required by the product/domain specification.

Potential information includes:

- medication name
- dosage
- dosage unit
- form
- instructions
- notes
- prescribing information where explicitly required
- active/inactive state

Do not add unnecessary clinical information.

Do not attempt to create a medical-record system.

GenXcare is a care coordination application, not a clinical EMR.

---

# 3. Medication Schedule

Implement medication schedules.

A schedule must support the recurrence patterns required by the specification.

Examples:

```text
Every day at 08:00
```

```text
Every day at 08:00 and 20:00
```

```text
Monday, Wednesday and Friday at 09:00
```

```text
Every 12 hours
```

Only implement recurrence patterns explicitly supported by the MVP specification.

Do not build an unnecessarily complex clinical scheduling engine.

---

# 4. Medication Instances

A medication schedule must produce concrete medication instances.

Example:

```text
Medication:
Metformin

Schedule:
500 mg
Every day at 08:00

Instances:

Aug 17 — 08:00 — pending
Aug 18 — 08:00 — pending
Aug 19 — 08:00 — pending
```

Each instance must have its own status.

Completing today's medication must not modify tomorrow's instance.

Historical medication instances must remain immutable except for explicitly supported corrections.

---

# 5. Medication Status

Use the status vocabulary defined in the domain specification.

At minimum support the concepts of:

- pending
- taken
- skipped
- missed

Do not invent conflicting status names.

The backend must validate status transitions.

Example:

```text
Pending
   ↓
Taken
```

or:

```text
Pending
   ↓
Skipped
```

or after the scheduled window has passed:

```text
Pending
   ↓
Missed
```

Do not allow arbitrary status changes from the client.

---

# 6. Mark Medication as Taken

Implement the primary medication action:

```text
Mark as Taken
```

When a user marks medication as taken:

- verify authentication
- verify senior access
- verify medication permission
- verify medication instance
- record authenticated actor
- record completion timestamp
- update the medication instance

Never accept the actor/user ID from the client.

Use the authenticated user.

---

# 7. Skipping Medication

Implement skipping where required by the domain specification.

When medication is skipped, preserve:

- actor
- timestamp
- medication instance
- resulting state

Do not delete skipped medication instances.

Skipped medication is part of the care history.

---

# 8. Missed Medication

A medication becomes missed when its scheduled time/window has passed without:

- being taken
- being skipped

Do not depend on a continuously running JavaScript timer.

The backend should be authoritative.

The mobile application may calculate/display a provisional state for responsiveness, but the server remains authoritative.

Do not require a background cleanup process for correctness.

---

# 9. Medication Permissions

Use the existing relationship-based permission model.

Do not create a separate medication authorization system.

At minimum distinguish the documented permissions for:

- viewing medication
- creating medication
- editing medication
- managing schedules
- recording medication as taken/skipped

Use the exact permission vocabulary from:

`docs/02-permissions-and-authorization.md`

A caregiver must not automatically receive every medication permission merely because they have a caregiver role.

Permissions belong to the relationship.

---

# 10. Medication Assignment

Medication belongs to a senior.

Do not assign medication directly to arbitrary users.

The medication action is performed by an authorized member of the senior's Care Circle.

Example:

```text
Senior
  ↓
Medication
  ↓
Authorized Care Circle Member
  ↓
Taken
```

The backend must verify that the actor has the required relationship and permission.

---

# 11. Medication Creation

Implement medication creation.

The authorized user should be able to:

1. Enter medication name.
2. Enter dosage.
3. Select medication form if supported.
4. Add instructions where supported.
5. Configure schedule.
6. Review.
7. Save.

Use clear, senior-friendly language.

Do not expose internal database terminology.

---

# 12. Medication Editing

Implement medication editing according to the documented permissions.

Be careful with historical medication instances.

Changing a medication's future schedule must not rewrite historical records.

Example:

```text
Medication
    ↓
Historical instances
    ↓
Preserved

Future schedule
    ↓
Updated
```

Do not retroactively modify completed medication history.

---

# 13. Medication Deactivation

Do not permanently delete medication records when a medication is no longer active unless the specification explicitly requires deletion.

Prefer the documented active/inactive or archive behavior.

Historical medication instances must remain available where required.

Example:

```text
Medication
    ↓
Inactive

Historical doses
    ↓
Preserved
```

---

# 14. Medication List

Implement the medication list required by the screen specification.

The list should prioritize today's medication.

Example:

```text
Today's Medication

08:00
Metformin
500 mg
[Take]

13:00
Vitamin D
1 tablet
[Take]

20:00
Metformin
500 mg
[Take]
```

The exact UI should follow the screen specification.

Do not make the screen visually dense.

---

# 15. Medication Detail

Implement the medication detail screen where required.

Display relevant information such as:

- medication name
- dosage
- instructions
- schedule
- current status
- next scheduled dose
- relevant history

Only display information the current user has permission to view.

---

# 16. Medication History

Implement medication history appropriate for the MVP.

A user should be able to understand:

- what medication was scheduled
- when it was scheduled
- whether it was taken
- whether it was skipped
- whether it was missed
- who recorded the action

Use pagination for large histories.

Do not load unlimited medication history into memory.

---

# 17. Quick Medication Action

"Mark as Taken" is a high-frequency action.

Optimize the flow:

```text
Tap "Take"
    ↓
Immediate UI update
    ↓
Mutation
    ↓
Server confirmation
```

Use TanStack Query optimistic updates where safe.

If the server rejects the mutation:

- roll back the optimistic state
- show an appropriate error
- do not leave the medication falsely marked as taken

---

# 18. TanStack Query

Use TanStack Query for:

- medications
- medication schedules
- today's medication
- medication history
- medication mutations

Use appropriate:

- query keys
- cache invalidation
- optimistic updates
- mutation rollback

Do not store medication server state in Zustand.

---

# 19. Zustand

Use Zustand only for local/UI state if necessary.

Examples:

- selected date
- medication filters
- temporary form state where appropriate

Do not use Zustand as the medication database.

---

# 20. Offline Medication Actions

Medication actions are important care actions and should support the existing offline architecture.

Support offline:

- viewing cached medication
- marking medication as taken
- skipping medication

Conceptually:

```text
User
 ↓
Mark Taken
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

The mutation must be safe to retry.

Do not create a separate synchronization system from Phase 4.

Reuse the existing task synchronization infrastructure where appropriate.

---

# 21. Idempotency

Medication actions must be idempotent.

If the same "mark taken" request is delivered twice, it must not create duplicate medication actions.

Use the existing idempotency mechanism from Phase 4.

Do not rely only on the client.

---

# 22. Conflict Handling

Consider:

```text
Caregiver A
    ↓
marks medication Taken

Caregiver B
    ↓
marks same medication Skipped
```

The backend must enforce valid state transitions.

The server is authoritative.

Do not allow arbitrary overwrites.

Document the selected conflict behavior.

---

# 23. Date and Time

Medication scheduling is particularly sensitive to timezones.

The senior's relevant timezone must be respected when calculating:

- scheduled medication time
- today's medication
- missed medication
- next dose

Do not assume device-local time is the server's source of truth.

Use the existing date/time conventions.

Pay attention to:

- midnight
- timezone boundaries
- daylight-saving changes where applicable
- recurring schedules

---

# 24. API

Implement the medication endpoints defined by:

`docs/05-api-and-backend-spec.md`

Likely areas include:

```text
GET /v1/seniors/{id}/medications
POST /v1/seniors/{id}/medications

GET /v1/medications/{id}
PATCH /v1/medications/{id}

GET /v1/medications/{id}/instances
POST /v1/medication-instances/{id}/take
POST /v1/medication-instances/{id}/skip
```

Use the documented API specification as the final authority on exact routes.

Do not create duplicate endpoints.

---

# 25. Authorization

Every medication operation must follow:

```text
Authenticated User
        ↓
CareRelationship
        ↓
Senior
        ↓
Medication
        ↓
Required Permission
```

A user must not access medication by guessing its ID.

Maintain the existing unauthorized-resource behavior.

Do not leak whether medication exists for another senior.

---

# 26. Database

Create the required migrations for:

- medications
- medication schedules
- medication instances

Use:

- UUIDs
- foreign keys
- constraints
- indexes
- timestamps
- appropriate status constraints

Add indexes based on real query patterns such as:

- senior
- medication
- schedule
- instance date/time
- status

Do not add unnecessary indexes.

---

# 27. Notifications

This phase introduces the foundation required for medication reminders.

Do not build a complete general notification platform yet.

The medication domain should expose enough scheduling information for Phase 8 to implement:

- medication reminders
- reminder timing
- notification preferences

Do not use continuous JavaScript timers.

Do not use aggressive polling.

If local notification scheduling is explicitly required by the current documentation, implement only the medication reminder functionality needed for the MVP.

---

# 28. Care Events

Do not implement the complete CareEvent system if it remains scheduled for Phase 7.

Medication actions must preserve enough information for future events:

- actor
- senior
- medication
- instance
- timestamp
- resulting state

Do not create a parallel event system.

---

# 29. Backend Tests

Test at minimum:

### Medication

- authorized user can create medication
- unauthorized user cannot create medication
- medication can be retrieved
- medication can be updated
- medication can be deactivated

### Scheduling

- valid schedule is accepted
- invalid schedule is rejected
- recurring instances are generated correctly
- timezone behavior is correct

### Medication actions

- authorized user can mark taken
- unauthorized user cannot mark taken
- authorized user can skip
- unauthorized user cannot skip
- actor is recorded correctly
- duplicate action is safe
- invalid status transition is rejected

### Missed medication

- pending medication becomes logically missed after its scheduled window
- taken medication is not missed
- skipped medication is not missed

### Authorization

- stranger cannot view medication
- caregiver without medication permission cannot access it
- revoked member cannot access medication
- unauthorized user cannot manipulate another senior's medication

### History

- historical medication instances remain intact
- medication schedule changes do not rewrite historical instances

Run:

```bash
go test -race -count=1 ./...
```

Also run all relevant integration tests against a fresh database.

---

# 30. Mobile Tests

Test:

1. Medication list loads.
2. Empty state works.
3. Loading state works.
4. Error state works.
5. Medication creation works.
6. Medication editing works.
7. Medication detail works.
8. Medication can be marked taken.
9. Medication can be skipped.
10. Optimistic update works.
11. Failed mutation rolls back.
12. Offline medication action is queued.
13. Queued medication action synchronizes.
14. Medication history loads.
15. Unauthorized actions are hidden.

---

# 31. Visual Requirements

Follow:

`docs/18-visual-theme-and-illustrations.md`

Primary brand color:

```text
#0F766E
```

Use semantic design tokens.

Medication UI must be:

- highly readable
- calm
- simple
- accessible
- easy to scan
- clear about dose and time

Medication status must never be communicated only through color.

Use icons/text alongside status colors.

Avoid unnecessary medical imagery.

---

# 32. Safety

GenXcare is a care coordination application.

Do not present the app as a medical professional.

Do not implement medical advice or diagnosis.

Do not infer whether a medication is medically appropriate.

The application records and reminds users about medication information supplied by the user/caregiver.

Do not invent dosage recommendations.

---

# 33. Do Not Implement Yet

Do NOT implement:

- appointments
- messaging
- full care-event timeline
- general notification center
- advanced push notification infrastructure
- caregiver marketplace
- billing
- AI
- wearable monitoring
- clinical decision support

Those belong to later phases.

Only implement medication management and the minimum reminder foundation required by the MVP.

---

# 34. Documentation

Update:

`docs/IMPLEMENTATION_STATUS.md`

Record:

- Phase 5 status
- medication domain decisions
- database changes
- scheduling decisions
- timezone decisions
- offline decisions
- authorization decisions
- notification decisions
- completed functionality
- tests
- blockers
- deferred functionality
- next phase

Do not silently change architecture.

---

# 35. Definition of Done

Phase 5 is complete when:

- Medication records work.
- Medication schedules work.
- Medication instances work.
- One-time medication schedules work where required.
- Recurring medication schedules work.
- Medication can be created.
- Medication can be edited.
- Medication can be deactivated.
- Medication can be viewed.
- Today's medication works.
- Medication history works.
- Medication can be marked taken.
- Medication can be skipped.
- Missed medication is correctly determined.
- Actors are recorded.
- Invalid state transitions are rejected.
- Medication actions are idempotent.
- Offline medication actions work.
- Queued medication actions synchronize.
- Authorization is enforced.
- Permission escalation is impossible.
- Mobile medication UI works.
- Optimistic updates work.
- Rollback works.
- Database migrations work on a fresh database.
- Backend tests pass.
- Mobile tests pass.
- Type checks pass.
- No Phase 6+ functionality has been unnecessarily implemented.

When Phase 5 is complete, stop.

Do not automatically continue to Phase 6.
