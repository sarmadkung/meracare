# MVP Roadmap and Engineering Tasks

## Phase 0 --- Product Foundation

### 1. Define user roles

-   Implement User.
-   Implement SeniorProfile.
-   Implement role constants.

### 2. Define Care Circle

-   Implement senior/member relationship.
-   Define membership lifecycle.

### 3. Define permissions

-   Create permission model.
-   Add server authorization checks.
-   Document default role permissions.

### 4. Define Senior

-   Create/edit/view senior.
-   Support self/solo profile.

### 5. Define Caregiver

-   Professional and family caregiver relationships.
-   Support multiple seniors per caregiver.

### 6. Define Care Task

-   Task template.
-   Task instance.
-   Assignment.
-   Status transitions.

### 7. Define Medication

-   Medication.
-   Schedule.
-   Medication instance.

### 8. Define Appointment

-   Appointment model.
-   Assignment.
-   Status.

### 9. Define Care Event

-   Event schema.
-   Event creation rules.
-   Activity timeline.

### 10. Define notification/escalation rules

-   Reminder.
-   Overdue.
-   Escalation.

## Phase 1 --- Architecture

### 11. Repository structure

-   Create monorepo.
-   Mobile app.
-   Backend.
-   Shared contracts/types where appropriate.

### 12. Database schema

-   PostgreSQL migrations.
-   Constraints.
-   Indexes.

### 13. API design

-   OpenAPI.
-   `/v1` endpoints.
-   Error format.

### 14. Authentication

-   Supabase Auth.
-   Apple/Google/email as selected.
-   Go JWT verification.

### 15. Authorization

-   Relationship-based middleware/service.
-   Permission checks.

### 16. Offline/local database

-   SQLite.
-   Repository layer.
-   Cache strategy.

### 17. Sync strategy

-   Mutation queue.
-   Retry.
-   Idempotency.
-   Conflict policy.

### 18. Notification architecture

-   Local notifications.
-   Push notifications.
-   Token registration.
-   Preferences.

## Phase 2 --- MVP Features

### 19. Authentication

-   Complete sign in/sign up.
-   Session restoration.

### 20. Senior creation

-   Create/edit profile.
-   Solo mode.

### 21. Care Circle

-   Members list.
-   Roles.
-   Permissions.

### 22. Invitations

-   Invite.
-   Accept.
-   Revoke.
-   Expiration.

### 23. Tasks

-   CRUD.
-   Assignment.
-   Completion.

### 24. Recurring tasks

-   Recurrence rules.
-   Instance generation.
-   Time zone handling.

### 25. Medication

-   CRUD.
-   Schedule.
-   Completion.
-   **Implemented:** mistaken-entry deletion before dose history; otherwise
    stop and retain history.

### 26. Appointments

-   CRUD.
-   Assignment.
-   Notifications.

### 27. Notes

-   Create/view notes.
-   Author and timestamp.
-   **Implemented:** senior-scoped API and screen, author-only editing,
    authorization, `NOTE_ADDED` activity, contracts, and automated tests.

### 28. Activity timeline

-   Event-driven timeline.
-   Cursor pagination.

### 29. Notifications

-   Local reminders.
-   Push.
-   Overdue notifications.

### 30. Care chat

-   Senior-scoped conversation.
-   Messages.
-   Read state.
-   **Implemented:** one senior-scoped stream, cursor pagination, monotonic
    per-member read state, authorization, contracts, screen, and automated
    tests.

### 31. Family dashboard

-   Senior status.
-   Care completion.
-   Attention items.

### 32. Professional caregiver dashboard

-   My seniors.
-   Today's workload.
-   Overdue items.

## Phase 3 --- Validation

### 33. Test with families

Interview and observe real family-care workflows.

### 34. Test with professional caregivers

Observe multi-senior workflows.

### 35. Measure task completion

Track: - task completion rate - missed tasks - time to completion

### 36. Measure caregiver engagement

Track: - weekly active caregivers - seniors per caregiver - daily care
interactions

### 37. Identify confusion

Collect: - onboarding drop-offs - failed invitations - incomplete
tasks - support questions

### 38. Fix UX before major expansion

Prioritize: - confusing workflows - excessive notifications - slow
screens - permission problems - offline issues

## Definition of MVP Done

MVP is complete when:

-   A solo user can manage their own care.
-   A family can create a senior.
-   Family can invite caregivers.
-   A professional caregiver can manage multiple seniors.
-   Tasks can be scheduled and completed.
-   Medication can be scheduled and completed.
-   Appointments can be coordinated.
-   Activity is visible to authorized members.
-   Authorized members can create and read senior-scoped care notes.
-   Authorized care-circle members can exchange senior-scoped messages and
    maintain read state.
-   Missed work can trigger notifications.
-   Core workflows work with intermittent connectivity.
-   Authorization prevents unauthorized access.
-   Critical flows have automated tests.
-   Managed profiles can be safely deleted when empty or archived once care
    history exists, and caregivers can leave without orphaning coordination.
