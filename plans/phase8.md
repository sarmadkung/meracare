Yes. **Phase 7 is complete, so the next phase is Phase 8 — Notifications & Reminders.**

Before starting it, **merge PR #7**. The Supabase real-auth verification is still a documented environment/product-configuration blocker, not a reason to alter the Phase 7 architecture.

For Phase 8, the important distinction is that we now have three things that can generate reminders:

* Tasks
* Medications
* Appointments

The notification system should be **generic infrastructure**, while those domains remain the source of truth.

Save this as `plans/phase8.md`:

````md
# GenxCare — Phase 8: Notifications & Reminders

Phase 7 is complete and merged.

Implement **Phase 8 only**.

Do not start Phase 9 or implement unrelated features.

## Objective

Build the notification and reminder infrastructure for GenxCare.

The system must provide reliable reminders for:

- Medication doses
- Care tasks
- Upcoming appointments

The notification system must be generic infrastructure.

Tasks, medications, and appointments remain the source of truth for what should be reminded.

Do not duplicate scheduling/business logic inside the notification system.

The system must support:

- push notifications
- local notifications where appropriate
- notification preferences
- reminder scheduling
- cancellation/rescheduling
- notification state
- deep linking into the relevant GenxCare screen
- retry-safe notification processing

The architecture must work for:

- Solo self-care
- Family care
- Professional caregivers
- Mixed family + professional care

---

# Before Starting

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

Also inspect:

- Phase 4 task implementation
- Phase 5 medication implementation
- Phase 6 appointment implementation
- Phase 7 CareEvent implementation
- existing mobile/background infrastructure

Do not assume the current implementation exactly matches the documentation.

---

# 1. Notification Architecture

Notifications are infrastructure.

They must not become the source of truth for care data.

Conceptually:

```text
Task
   ↓
Task schedule
   ↓
Reminder
   ↓
Notification

Medication
   ↓
Medication schedule
   ↓
Reminder
   ↓
Notification

Appointment
   ↓
Appointment time
   ↓
Reminder
   ↓
Notification
````

The notification system must not create or own:

* tasks
* medications
* appointments

It only schedules and delivers reminders about them.

---

# 2. Notification Types

Use the vocabulary defined in:

`docs/08-notifications-and-background.md`

Support the notification categories required by the MVP.

At minimum the architecture must support:

```text
TASK_REMINDER
MEDICATION_REMINDER
APPOINTMENT_REMINDER
```

If the existing documentation defines additional types, use those exact definitions.

Do not invent a parallel notification vocabulary.

---

# 3. Notification Preferences

Users must be able to control notification preferences.

Preferences should be per user, not globally per senior.

Conceptually:

```text
User
├── Task reminders
├── Medication reminders
└── Appointment reminders
```

Follow the exact preference model in the documentation.

Do not create a separate preference system for each domain.

---

# 4. Senior vs User Preferences

Be careful about whose notification is being controlled.

Example:

```text
Senior
    ↓
Medication
    ↓
Caregiver A
    ↓
Medication reminder
```

Caregiver A's notification preferences determine whether that caregiver receives the reminder.

Another caregiver may have different preferences.

Do not assume that all Care Circle members receive every notification.

---

# 5. Authorization

A notification must never expose information to a user who does not currently have access to the underlying senior/care information.

Before scheduling or delivering a notification, respect the existing CareRelationship model.

Conceptually:

```text
User
 ↓
Active CareRelationship
 ↓
Required permission/access
 ↓
Notification
```

A revoked caregiver must not continue receiving reminders for the senior.

---

# 6. Notification Permission

Do not assume that application-level permission means the device has granted notification permission.

There are two separate concepts:

```text
GenxCare preference
        +
OS notification permission
```

The application preference controls what GenxCare wants to send.

The operating system controls whether the device allows notifications.

Handle both correctly.

---

# 7. Device Registration

Implement device registration required for push notifications.

A user may have multiple devices.

Example:

```text
User
├── iPhone
├── Android phone
└── Tablet
```

Do not store only one push token per user.

Device registration should support:

* device identifier
* push token
* platform
* app/version information where useful
* active/inactive state
* timestamps

Use the exact fields defined by the documentation.

---

# 8. Push Token Security

Push tokens are sensitive identifiers.

Do not expose them unnecessarily through APIs.

Do not log push tokens.

Do not include tokens in CareEvents.

Do not expose another user's device registrations.

---

# 9. Device Lifecycle

Support:

* registration
* token update
* device deactivation
* logout/unregister where appropriate

If a push token becomes invalid, mark/remove it according to the documented lifecycle.

Do not allow an invalid device registration to cause permanent notification failures.

---

# 10. Local vs Remote Notifications

Use the appropriate mechanism for each reminder.

The architecture should distinguish:

```text
Local notification
    ↓
Scheduled directly on device
```

and:

```text
Remote push notification
    ↓
Server
    ↓
Push provider
    ↓
Device
```

Use local notifications where they provide a reliable experience for user-specific scheduled reminders.

Use remote notifications where server-side state or multi-user coordination requires them.

Do not introduce unnecessary server polling.

---

# 11. Medication Reminders

Medication reminders are the highest-priority reminder workflow.

A medication reminder should be generated from the existing medication schedule.

Example:

```text
Medication
Metformin
500 mg

Schedule
08:00 daily

        ↓

Reminder
08:00

        ↓

Notification
"Time to take your medication"
```

Do not create a separate medication schedule inside notifications.

---

# 12. Medication Reminder Timing

Use the reminder timing defined by the product specification.

If configurable reminder offsets are supported, use the documented values.

For example:

```text
Medication time
08:00

Reminder
07:45
```

Do not invent additional reminder options beyond the documented MVP.

---

# 13. Task Reminders

Tasks may have reminders where supported by the task model.

Example:

```text
Task
Morning walk

Due
09:00

Reminder
08:45
```

Use the existing task schedule.

Do not create another recurrence engine.

The shared recurrence implementation from Phase 5 remains the source of truth.

---

# 14. Appointment Reminders

Appointments may generate reminders before the appointment.

Example:

```text
Appointment
Doctor — 14:00

Reminder
13:00
```

Use the appointment's existing date/time/timezone.

Do not create another appointment scheduling system.

---

# 15. Deep Linking

Every notification must know where it should take the user.

Examples:

```text
Medication reminder
    ↓
Medication detail / today's medication
```

```text
Task reminder
    ↓
Task detail / today's care
```

```text
Appointment reminder
    ↓
Appointment detail
```

Use the existing React Native navigation architecture.

Do not introduce a second navigation system.

---

# 16. Notification Payload

Notification payloads should contain only the information necessary for delivery and navigation.

Prefer:

```json
{
  "type": "MEDICATION_REMINDER",
  "seniorId": "...",
  "entityId": "..."
}
```

rather than sending complete medical information in the push payload.

Do not include unnecessary:

* medication details
* dosage information
* private notes
* medical history

in remote notification payloads.

The application can fetch authorized information after opening the relevant screen.

---

# 17. Privacy

Notifications can appear on a locked device.

Do not expose sensitive information by default.

Avoid notification text such as:

```text
"Take your 500mg Metformin for diabetes."
```

Prefer privacy-conscious wording such as:

```text
"Medication reminder"
```

or the exact wording defined by the product specification.

Do not include sensitive medical information unless explicitly required by the documented notification UX.

---

# 18. Notification Preferences UI

Implement the notification settings UI required by the screen specification.

Users should be able to control the documented notification categories.

Example:

```text
Notifications

Medication reminders        ON
Task reminders              ON
Appointment reminders       ON
```

Use clear controls.

Do not expose internal notification type identifiers.

---

# 19. OS Permission UI

Implement the appropriate experience for requesting notification permission.

Do not immediately request permission without context if the existing UX specification provides a better flow.

The user should understand why notifications are useful before being asked for OS permission where practical.

Respect:

* granted
* denied
* restricted
* provisional/limited states where supported by the platform

Use the capabilities of the chosen React Native notification stack.

---

# 20. Scheduling Model

The notification system must avoid continuously running JavaScript timers.

Do not implement:

```text
setInterval(...)
```

as the primary reminder mechanism.

Do not keep a React Native process alive to check whether a medication is due.

Use:

* OS local notification scheduling
* server-side scheduling where appropriate
* push delivery infrastructure

according to the documented architecture.

---

# 21. Recurrence

Do not duplicate the recurrence engine.

The existing recurrence implementation from Phases 4 and 5 remains authoritative.

Notification scheduling should consume the resulting schedule.

Do not create:

```text
Notification recurrence engine
```

separate from:

```text
Task/Medication recurrence engine
```

---

# 22. Scheduling Changes

If a task, medication schedule, or appointment changes:

```text
Existing reminder
       ↓
Cancel/update
       ↓
New reminder
```

Do not leave stale reminders scheduled.

Examples:

* medication time changes
* medication becomes inactive
* task is deleted/deactivated
* task schedule changes
* appointment is cancelled
* appointment time changes

The notification layer must react appropriately.

---

# 23. Relationship Revocation

If a caregiver loses access to a senior:

```text
CareRelationship
    ↓
Revoked
```

future notifications for that caregiver must stop.

Do not rely only on the mobile application to stop them.

Server-side notification eligibility must respect current authorization.

Already-delivered notifications cannot be recalled; only future delivery must be prevented.

---

# 24. Notification State

Track enough information to safely manage scheduled/delivered notifications.

Where required by the documentation, support states such as:

```text
SCHEDULED
SENT
CANCELLED
FAILED
```

Use the exact vocabulary defined in the documentation.

Do not create unnecessary state transitions.

---

# 25. Idempotency

Notification scheduling must be idempotent.

For example:

```text
Medication schedule updated
    ↓
API retry
    ↓
schedule reminder
    ↓
API retry again
```

This must not produce multiple identical reminders.

Use a deterministic notification identity or equivalent idempotency mechanism.

Do not rely only on the client.

---

# 26. Duplicate Prevention

Prevent duplicate notifications caused by:

* app restart
* sync retry
* API retry
* device registration retry
* schedule recalculation
* multiple Care Circle members
* repeated background execution

One intended reminder for one user/device should not become multiple identical reminders.

---

# 27. Multi-Device Behavior

A user may have multiple registered devices.

Follow the documented behavior for notification delivery.

At minimum:

* valid devices may receive the notification
* inactive devices must not
* invalid tokens must be handled
* registration must be idempotent

Do not assume one user equals one device.

---

# 28. Backend Notification Service

Implement a small notification abstraction.

For example:

```text
NotificationService
```

or an equivalent architecture appropriate to the existing codebase.

Keep provider-specific code behind the abstraction.

Do not spread Expo/APNs/FCM-specific logic throughout domain services.

---

# 29. Provider Architecture

The mobile application is React Native.

Use the existing project choice for push/local notifications if documented.

Do not replace the project's mobile architecture without a strong reason.

The backend should not become tightly coupled to a single provider if the existing architecture supports abstraction.

Keep provider-specific implementation isolated.

---

# 30. Background Processing

If the documentation requires server-side background processing:

* implement the minimum worker required
* make jobs retry-safe
* avoid duplicate delivery
* avoid unbounded polling

Do not introduce Kafka, NATS, RabbitMQ, or another distributed messaging platform.

The MVP does not require a distributed event-processing architecture.

---

# 31. Notification Scheduling Source of Truth

Do not make notification records authoritative over domain records.

For example:

```text
Medication inactive
```

must mean:

```text
No future medication reminders
```

even if an old notification record still exists.

Domain state wins.

---

# 32. Timezones

Notification scheduling must use the senior/user's relevant timezone according to the existing product rules.

Do not use server UTC blindly for local reminders.

Pay particular attention to:

* midnight
* timezone changes
* daylight-saving transitions
* recurring schedules

Use the same date/time and recurrence logic established in previous phases.

---

# 33. Notification API

Implement the endpoints defined in:

`docs/05-api-and-backend-spec.md`

Only add endpoints required for:

* device registration
* device removal/update
* notification preferences
* notification state where required

Do not expose internal notification scheduling APIs directly to normal users unless the specification requires them.

---

# 34. Database

Create the required migrations for:

* notification preferences
* device registrations
* notification records/schedules if required by the architecture

Use:

* UUIDs
* foreign keys
* constraints
* indexes
* timestamps
* unique constraints where required

Add indexes based on actual access patterns.

Do not store secrets unnecessarily.

---

# 35. Mobile Data

Use TanStack Query for server-owned notification data:

* notification preferences
* device registration state where required

Use Zustand only for local UI state.

Do not store notification server state entirely in Zustand.

---

# 36. Offline

Notification settings should behave safely offline.

If the user changes a preference while offline:

* follow the existing mutation/sync architecture if supported
* otherwise require connectivity before confirming the change

Do not create a second offline queue.

Notification scheduling must never assume an offline preference mutation has already reached the server.

---

# 37. Error Handling

Handle:

* notification permission denied
* invalid push token
* provider failure
* expired device registration
* scheduling failure
* network failure
* authorization failure

Do not crash the application because notifications are unavailable.

Core care functionality must continue working without notifications.

---

# 38. Notification Failure

A failed notification must not change the underlying care state.

For example:

```text
Medication reminder failed
```

must NOT mean:

```text
Medication missed
```

Likewise:

```text
Appointment notification failed
```

must not change appointment status.

Notifications are delivery infrastructure, not care-state authority.

---

# 39. Care Events

Do not create CareEvents for every notification delivery.

Notification delivery itself is infrastructure.

Do not pollute the Activity Timeline with:

```text
Notification sent
Notification failed
Notification scheduled
```

unless the existing CareEvent specification explicitly requires such events.

---

# 40. Security

Test that:

* users cannot register devices for another user
* users cannot modify another user's notification preferences
* users cannot create notifications for another senior without authorization
* revoked caregivers stop receiving future reminders
* push tokens are not exposed
* sensitive information is not unnecessarily included in notification payloads
* notification endpoints enforce authentication

Never trust:

```text
userId
seniorId
actorId
```

from the client without authorization.

---

# 41. Backend Tests

Test at minimum:

### Device registration

* register device
* update token
* deactivate device
* duplicate registration is safe
* unauthorized device modification is rejected

### Preferences

* retrieve preferences
* update preferences
* unauthorized access is rejected
* defaults are correct

### Scheduling

* medication reminder is scheduled correctly
* task reminder is scheduled correctly
* appointment reminder is scheduled correctly
* correct timezone is used
* duplicate scheduling does not duplicate reminders

### Cancellation

* inactive medication cancels future reminders
* cancelled appointment cancels future reminders
* changed task schedule removes stale reminders
* revoked caregiver loses future reminders

### Authorization

* stranger cannot configure notifications for a senior
* caregiver without access cannot receive new reminders
* revoked caregiver cannot receive future reminders

### Idempotency

* repeated scheduling produces one intended reminder
* repeated cancellation is safe
* retries do not duplicate notifications

Run:

```bash
go test -race -count=1 ./...
```

Also run the complete integration suite against a fresh database.

---

# 42. Mobile Tests

Test:

1. Notification settings load.
2. Notification preferences can be changed.
3. OS notification permission flow works.
4. Permission denial is handled gracefully.
5. Device registration works.
6. Token refresh works.
7. Medication notification deep-links correctly.
8. Task notification deep-links correctly.
9. Appointment notification deep-links correctly.
10. Invalid notification payload does not crash the app.
11. Notification does not expose sensitive information unnecessarily.
12. Notification settings work correctly after app restart.

---

# 43. Real Device Verification

Where possible, verify notifications on:

* iOS
* Android

Do not claim push delivery works based only on unit tests.

If physical-device or provider credentials are unavailable:

* test all available local behavior
* document the limitation
* do not fabricate successful push delivery

---

# 44. Supabase Authentication

The previous Phase 7 verification identified that the live Supabase JWKS works, but a genuine authenticated sign-in round trip remains blocked by email confirmation.

Do not redesign authentication during Phase 8.

If the environment has been configured to allow a real test account:

* obtain a genuine Supabase session
* call `/v1/me`
* verify the application user
* verify authenticated notification APIs
* verify device registration using the real identity

If it remains blocked:

* document it accurately
* do not claim full real-auth verification

---

# 45. Supabase Database

Do not change the database architecture because of the previous direct-connection issue.

The production/backend connection should use the configured Supabase Session Pooler where appropriate.

Do not commit credentials.

Do not commit `.env` secrets.

If hosted migrations are still not applied, document that separately from the Phase 8 implementation.

---

# 46. Performance

Notification infrastructure must not negatively affect the core application.

Avoid:

* continuous polling
* long-running mobile JavaScript loops
* excessive API requests
* loading all future reminders at once
* unnecessary database scans

Use indexed queries and bounded scheduling operations.

---

# 47. Privacy

Notification text must be privacy-conscious.

Do not expose unnecessary medical information on lock screens.

Examples of preferred wording:

```text
Medication reminder
```

```text
Care task reminder
```

```text
Upcoming appointment
```

Do not include diagnosis, medical history, or unnecessary dosage details in push payloads.

---

# 48. Visual Requirements

Follow:

`docs/18-visual-theme-and-illustrations.md`

Primary color:

```text
#0F766E
```

Notification settings should use the existing GenxCare design system.

Keep the interface:

* calm
* clear
* accessible
* simple
* easy to understand

Do not use illustrations unnecessarily in settings screens.

---

# 49. Do Not Implement Yet

Do NOT implement:

* messaging
* chat
* AI
* analytics dashboards
* wearable integrations
* telemedicine
* caregiver marketplace
* billing
* advanced notification campaigns
* marketing notifications
* SMS infrastructure unless explicitly required by the existing MVP documentation

Only implement the notification/reminder foundation and care reminders required by the roadmap.

---

# 50. Documentation

Update:

`docs/IMPLEMENTATION_STATUS.md`

Record:

* Phase 8 status
* notification architecture
* provider choice
* device-registration model
* preference model
* scheduling model
* timezone decisions
* idempotency strategy
* authorization behavior
* privacy decisions
* database changes
* mobile implementation
* real-device verification
* Supabase authentication verification status
* environment limitations
* completed functionality
* tests
* blockers
* deferred functionality
* next phase

Do not silently change architecture.

---

# 51. Definition of Done

Phase 8 is complete when:

* Notification infrastructure exists.
* Notification preferences work.
* Device registration works.
* Multiple devices per user are supported.
* Invalid devices can be deactivated.
* Medication reminders work.
* Task reminders work where specified.
* Appointment reminders work.
* Reminder scheduling respects timezone.
* Stale reminders are cancelled.
* Schedule changes update reminders.
* Revoked caregivers stop receiving future reminders.
* Duplicate reminders are prevented.
* Scheduling is idempotent.
* Notification payloads are privacy-conscious.
* Deep links work.
* OS notification permission is handled.
* Notification failures do not affect care state.
* Core application functionality works without notifications.
* TanStack Query integration works.
* Mobile notification settings work.
* Backend authorization works.
* Security tests pass.
* Backend tests pass.
* Mobile tests pass.
* Database migrations work on a fresh database.
* Type checks pass.
* Lint passes.
* Real-device notification behavior is verified where credentials/devices permit.
* Any unavailable external verification is documented honestly.
* No Phase 9+ functionality has been unnecessarily implemented.

When Phase 8 is complete, stop.

Do not automatically continue to Phase 9.

```
```
