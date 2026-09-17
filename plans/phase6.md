# GenxCare — Phase 6: Appointments & Care Schedule

Phase 5 is complete and merged.

Implement **Phase 6 only**.

Do not start Phase 7 or implement unrelated features.

## Objective

Build the appointment and care-schedule system.

GenxCare must allow authorized users to:

- create appointments
- view appointments
- edit appointments
- cancel appointments
- assign/associate appointments with a senior
- track upcoming appointments
- view appointment history
- record relevant appointment information

The system must support:

- Solo self-care
- Family care
- Professional caregivers
- Mixed family + professional care

Use the existing CareRelationship and permission architecture.

Do not model appointments as tasks.

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

Also inspect the complete Phase 5 implementation before making changes.

---

# 1. Appointment Domain

Implement the appointment domain defined in the existing specification.

An appointment belongs to a senior.

Conceptually:

```text
Senior
  ↓
Appointment
```

An appointment may contain:

- title
- date
- start time
- end time where applicable
- location
- provider/doctor name where applicable
- notes
- status
- creator
- timestamps

Only implement fields defined by the product/domain specification.

Do not create a clinical record system.

---

# 2. Appointment Types

Support the appointment types required by the MVP specification.

Examples may include:

- doctor visit
- hospital visit
- therapy
- laboratory visit
- family/care meeting
- other care-related appointment

Do not hardcode an unnecessarily large medical taxonomy.

If the specification defines appointment categories, use those values.

---

# 3. Appointment Status

Use the status vocabulary defined in the domain specification.

Support the documented concepts such as:

- scheduled
- completed
- cancelled

Do not invent conflicting status names.

The backend must validate state transitions.

Example:

```text
Scheduled
   ↓
Completed
```

or:

```text
Scheduled
   ↓
Cancelled
```

A cancelled appointment must not later become completed unless explicitly allowed by the domain specification.

---

# 4. Date and Time

Appointments are time-sensitive.

Handle:

- date
- start time
- end time
- timezone

correctly.

The senior's relevant timezone must be respected.

Do not assume device-local time is the server's source of truth.

Use the existing project date/time conventions.

Pay attention to:

- midnight
- timezone boundaries
- daylight-saving changes where applicable
- appointments crossing midnight where supported

Do not implement naive date/time string manipulation.

---

# 5. Upcoming Appointments

Implement the upcoming appointment view.

Example:

```text
Upcoming

Tomorrow

09:30
Dr. Ahmed
City Hospital

Friday

14:00
Blood Test
MedLab
```

Prioritize the nearest appointments.

Do not load unlimited appointment history into memory.

Use appropriate pagination for historical appointments.

---

# 6. Appointment History

Implement appointment history.

Users with appropriate permissions should be able to see:

- appointment
- date/time
- status
- location
- provider
- relevant notes

Historical appointments must remain available where required.

Do not delete historical information simply because an appointment is completed or cancelled.

---

# 7. Create Appointment

Implement appointment creation.

The authorized user should be able to:

1. Enter appointment title.
2. Select date.
3. Select time.
4. Enter location where applicable.
5. Enter provider/doctor where applicable.
6. Add notes where supported.
7. Review.
8. Create.

Use senior-friendly language and controls.

Do not expose internal database terminology.

---

# 8. Edit Appointment

Implement appointment editing according to the documented permissions.

Editing a future appointment is allowed when the user has the required permission.

Historical appointments should not be rewritten in a way that destroys the original history.

If the specification requires preserving changes, use the documented approach.

Do not introduce a separate audit system during this phase.

---

# 9. Cancel Appointment

Implement cancellation according to the documented permission model.

Cancelling an appointment must preserve the appointment record.

Do not delete it.

Example:

```text
Scheduled
    ↓
Cancelled
```

The cancelled state must remain visible in history.

---

# 10. Complete Appointment

If supported by the domain specification, allow authorized users to mark an appointment as completed.

Example:

```text
Scheduled
    ↓
Completed
```

Record:

- status
- relevant timestamp
- authenticated actor

Never accept the actor ID from the client.

---

# 11. Appointment Permissions

Use the existing relationship permission system.

Do not create a separate appointment authorization system.

Use the exact permission vocabulary defined in:

`docs/02-permissions-and-authorization.md`

The backend must enforce permissions for:

- viewing appointments
- creating appointments
- editing appointments
- cancelling appointments
- completing appointments

Do not automatically grant appointment management permissions based only on role.

Permissions belong to the relationship.

---

# 12. Authorization

Every appointment operation must follow:

```text
Authenticated User
        ↓
CareRelationship
        ↓
Senior
        ↓
Appointment
        ↓
Required Permission
```

A user must not access an appointment simply by knowing its ID.

Maintain the existing unauthorized-resource behavior from previous phases.

Do not reveal whether another senior has an appointment.

---

# 13. Appointment Creator

The creator must always come from the authenticated session.

Never trust:

```text
createdBy
created_by
userId
```

from the client request.

The backend must derive the creator from authentication.

---

# 14. Appointment API

Implement the endpoints defined by:

`docs/05-api-and-backend-spec.md`

Likely areas include:

```text
GET /v1/seniors/{id}/appointments
POST /v1/seniors/{id}/appointments

GET /v1/appointments/{id}
PATCH /v1/appointments/{id}

POST /v1/appointments/{id}/cancel
POST /v1/appointments/{id}/complete
```

Use the existing API specification as the final authority on exact routes and request/response formats.

Do not create duplicate endpoints.

---

# 15. Database

Create the required appointment migration(s).

Use:

- UUIDs
- foreign keys
- constraints
- indexes
- timestamps
- appropriate status constraints

Add indexes based on actual query patterns, such as:

- senior
- appointment date/time
- status

Do not add unnecessary indexes.

---

# 16. Mobile Appointment List

Implement the appointment UI required by the screen specification.

The primary experience should prioritize upcoming appointments.

Example:

```text
Appointments

Today

09:30
Dr. Ahmed
City Hospital

Thursday

15:00
Physiotherapy
Care Center
```

The UI should be simple and highly readable.

---

# 17. Appointment Detail

Implement the appointment detail screen.

Display relevant information:

- title
- date
- time
- location
- provider
- notes
- status

Display actions based on the user's permissions.

Do not show edit/cancel/complete actions to unauthorized users.

---

# 18. Appointment Creation UI

Implement a simple creation flow.

Use:

- date picker
- time picker
- text fields
- appropriate selectors where required

Do not add a heavy form library unless the existing architecture requires it.

Use native React Native components and existing project patterns.

---

# 19. Appointment Editing UI

Allow authorized users to edit future appointments.

When editing:

- preserve valid existing data
- validate date/time
- prevent invalid states
- handle server errors clearly

Do not allow unauthorized users to modify appointments.

---

# 20. Appointment Cancellation UI

Cancellation should require an explicit user action.

Do not cancel appointments accidentally through navigation or gestures.

Where appropriate, ask for confirmation before cancellation.

After successful cancellation:

```text
Scheduled
    ↓
Cancelled
```

Update the UI immediately after server confirmation or through a safe optimistic mutation.

---

# 21. TanStack Query

Use TanStack Query for:

- upcoming appointments
- appointment history
- appointment details
- appointment mutations

Use appropriate:

- query keys
- cache invalidation
- stale times
- optimistic updates where safe

Do not store appointment server state in Zustand.

---

# 22. Zustand

Use Zustand only for local UI state where necessary.

Examples:

- selected date
- appointment filters
- temporary form/UI state

Do not store appointment lists in Zustand.

---

# 23. Offline

Appointments are less suitable for offline mutation than task completion or medication actions.

For this phase:

- cached appointment viewing should work offline
- creating/editing/cancelling appointments may require connectivity
- do not create a second synchronization queue

Reuse existing infrastructure only where it is appropriate.

Do not queue appointment mutations unless the existing specification explicitly requires it.

The server remains authoritative.

---

# 24. Idempotency

Where appointment actions can be retried, ensure they are safe.

Examples:

```text
Cancel appointment
Complete appointment
```

Repeated requests must not corrupt appointment state.

Use the existing API/idempotency patterns.

Do not create a new idempotency architecture.

---

# 25. Conflict Handling

Consider:

```text
User A
  ↓
Cancel appointment

User B
  ↓
Complete appointment
```

The backend must enforce valid state transitions.

Do not allow arbitrary status overwrites.

The server is authoritative.

Document the selected behavior.

---

# 26. Notifications Foundation

Do not build the complete notification system yet.

However, appointment data must contain enough information for a later notification phase to support:

- appointment reminders
- upcoming appointment notifications
- cancellation notifications

Do not use JavaScript timers.

Do not implement aggressive polling.

Do not build a general notification center in this phase.

---

# 27. Care Events

Do not implement the full CareEvent system if it remains scheduled for Phase 7.

Appointment actions should retain enough information for future events:

- actor
- senior
- appointment
- timestamp
- resulting state

Do not create a parallel event system.

---

# 28. Safety

GenxCare is a care coordination application.

Do not provide:

- medical diagnosis
- treatment recommendations
- clinical decision support
- claims that an appointment is medically necessary

The application only records and coordinates appointments.

Do not infer medical information from appointment titles.

---

# 29. Backend Tests

Test at minimum:

### Creation

- authorized user can create appointment
- unauthorized user cannot create appointment
- invalid date/time is rejected
- invalid senior access is rejected

### Retrieval

- authorized user can view appointment
- unauthorized user cannot view appointment
- upcoming appointments are correctly ordered
- historical appointments are correctly returned

### Editing

- authorized user can edit appointment
- unauthorized user cannot edit appointment
- historical data is not incorrectly destroyed

### Cancellation

- authorized user can cancel
- unauthorized user cannot cancel
- cancelled appointment remains in history
- cancelled appointment cannot be completed if prohibited

### Completion

- authorized user can complete
- unauthorized user cannot complete
- completion records authenticated actor
- invalid state transitions are rejected

### Authorization

- stranger cannot access appointments
- revoked member cannot access appointments
- member without edit permission cannot edit
- member without cancellation permission cannot cancel

Run:

```bash
go test -race -count=1 ./...
```

Also run the integration suite against a fresh database.

---

# 30. Mobile Tests

Test:

1. Upcoming appointments load.
2. Appointment history loads.
3. Empty state works.
4. Loading state works.
5. Error state works.
6. Authorized user can create appointment.
7. Authorized user can edit appointment.
8. Authorized user can cancel appointment.
9. Authorized user can complete appointment where supported.
10. Unauthorized actions are hidden.
11. Appointment detail displays correctly.
12. Date/time validation works.

---

# 31. Visual Requirements

Follow:

`docs/18-visual-theme-and-illustrations.md`

Primary brand color:

```text
#0F766E
```

Use semantic design tokens.

Appointment UI should be:

- readable
- calm
- simple
- accessible
- easy to scan

Clearly distinguish:

- upcoming
- completed
- cancelled

Do not communicate status through color alone.

---

# 32. Performance

Appointment lists should remain fast with large histories.

Use:

- pagination
- appropriate query limits
- efficient database indexes
- virtualized lists where necessary
- TanStack Query caching

Do not load unlimited historical appointments.

---

# 33. Do Not Implement Yet

Do NOT implement:

- CareEvent system
- notification center
- advanced push notifications
- appointment reminders
- messaging
- AI
- wearable monitoring
- telemedicine
- billing
- caregiver marketplace

Those belong to later phases.

Only implement the appointment foundation.

---

# 34. Documentation

Update:

`docs/IMPLEMENTATION_STATUS.md`

Record:

- Phase 6 status
- appointment domain decisions
- database changes
- timezone decisions
- authorization decisions
- offline decisions
- completed functionality
- tests
- blockers
- deferred functionality
- next phase

Do not silently change architectural decisions.

---

# 35. Definition of Done

Phase 6 is complete when:

- Appointment records work.
- Appointments can be created.
- Appointments can be viewed.
- Upcoming appointments work.
- Appointment history works.
- Appointments can be edited.
- Appointments can be cancelled.
- Appointments can be completed where required.
- Status transitions are validated.
- Authorization is enforced.
- Revoked members lose access.
- Authenticated actors are recorded correctly.
- Date/time handling is correct.
- Timezones are handled correctly.
- Offline cached viewing works.
- TanStack Query integration works.
- Appointment UI works.
- Appointment creation works.
- Appointment detail works.
- Appointment editing works.
- Appointment cancellation works.
- Backend tests pass.
- Mobile tests pass.
- Database migrations work on a fresh database.
- Type checks pass.
- Lint passes.
- No Phase 7+ functionality has been unnecessarily implemented.

When Phase 6 is complete, stop.

Do not automatically continue to Phase 7.
