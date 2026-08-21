# MeraCare — Phase 11: Notifications & Reminders

Implement **Phase 11 only**.

Establish MeraCare's notification system and deliver the first production-useful
notifications for medication reminders, upcoming appointments, care tasks,
overdue tasks, and important care-circle activity — for family caregivers,
professional caregivers, and seniors using MeraCare themselves.

Do not implement SMS, WhatsApp, email, or marketing notifications in this phase.

## Objective

```text
Medication reminder ─┐
Appointment reminder ─┤
Task reminder ────────┼──→ Notification system ──┬──→ In-app notification
Overdue task ─────────┤                          └──→ Push notification
Care activity ────────┘
```

Additional channels must be addable later without replacing the core model.

## Architecture

```text
Domain event / scheduled event → Notification service → Notification
                                                        ├── in-app
                                                        └── push provider
```

Medication, tasks, and appointments must not send push notifications
themselves; they produce notification-worthy events or jobs. The notification
domain lives in `internal/notifications/` and defines the vocabulary:
`MEDICATION_REMINDER`, `APPOINTMENT_REMINDER`, `TASK_REMINDER`, `TASK_OVERDUE`,
`CARE_ACTIVITY`. Do not add types that are not implemented.

## Requirements

1. **Model.** Persist recipient, senior, type, title, body, related resource,
   scheduled time, delivery status, read status, and creation time. Avoid
   uncontrolled JSON where a typed structure suffices. Keep it extensible.
2. **Recipient.** A notification is addressed to `recipient_user_id`, never to a
   senior alone — several people care for the same person.
3. **Authorization.** Nobody may read, mark read, or delete another user's
   notification, or register a device for another user. Do not expose
   notifications by senior id.
4. **Preferences.** User-level and independently configurable: medication,
   appointment, task, overdue task, care activity. Small, focused, sensible
   defaults.
5. **Every care mode.** A senior using MeraCare alone receives their own
   medication reminders. Family members are notified according to relationship,
   permission, and preference — not everybody about everything. A professional
   caregiver receives notifications for every client they are authorized for,
   through one system rather than one per client.
6. **Medication reminders.** Highest priority. Use the existing schedules,
   respect every schedule on a medication, and do not hardcode one global
   reminder time.
7. **Appointment reminders.** 24 hours before and 1 hour before. Never for
   cancelled or completed appointments, and never duplicated. Document any
   interval the existing model makes impossible rather than changing the model.
8. **Task reminders and overdue alerts.** Use the existing task system and its
   recurrence. Use the existing overdue derivation — do not introduce a second
   definition, and do not persist a `notification_overdue` state.
9. **Missed medication.** Do not add a persistent `MEDICATION_MISSED` state, and
   do not add a background job to create one.
10. **Care activity.** Use the Phase 7 care-event system. Notify only about
    useful activity; a care event is not automatically a push.
11. **Deduplication.** A deterministic idempotency key over recipient, type,
    resource, occurrence, and channel, enforced at the persistence layer — not
    in application memory.
12. **Delivery.** Creation is separate from delivery. No API request waits on a
    push-provider call.
13. **Push provider.** Use the supported Expo push architecture rather than
    building APNs/FCM infrastructure prematurely, behind an application
    interface so the provider can be replaced.
14. **Devices.** Multiple per user. Store user, device, push token, platform,
    active state, and last-seen time, with an appropriate uniqueness constraint.
    Tokens are sensitive: never logged, never returned, never registerable for
    another user. Tolerate rotation, expiry, invalidity, and uninstalls.
    Deactivate on logout rather than necessarily deleting.
15. **Inbox.** An in-app list with read/unread, `GET /v1/notifications`,
    `PATCH /v1/notifications/:id/read`, `POST /v1/notifications/read-all`, and
    the existing keyset pagination — not a second cursor implementation.
16. **Navigation.** Notifications deep-link to the relevant resource, and the
    destination is authorized when it opens. Possession of a notification id is
    never authorization.
17. **Retention.** A sensible MVP period, documented, without an aggressive
    deletion mechanism there is no need for.
18. **Time zones and DST.** Schedules are interpreted in the senior's timezone.
    Use the existing recurrence abstraction; never compute a calendar recurrence
    with fixed 86,400-second days. Document any incompleteness rather than
    inventing a second timezone model.
19. **Scheduling.** Server → push provider → device is the primary architecture.
    No infinite client-side JavaScript timer for production reminders.
20. **Scheduler.** Introduce one only where necessary: find due work, create
    notifications, enqueue delivery, retry failures, avoid duplicates. Explicit
    start/stop/shutdown, no uncontrolled goroutine loop, and a clean exit.
21. **Concurrency.** Assume more than one API instance. Use database-backed
    claiming so two workers cannot deliver the same notification. No in-memory
    mutex.
22. **Retry.** Distinguish temporary failure, permanent failure, and invalid
    token. Conservative, bounded retries. Deactivate a token the provider
    rejects instead of retrying it.
23. **Mobile permission.** Ask at a point where the user understands why, using
    the platform APIs. If denied, the app keeps working with in-app
    notifications and history, does not nag, and offers a settings path.
24. **Preferences UI.** A simple settings section with the five switches,
    following the existing theme. No complicated scheduling controls.
25. **Professional and family UX.** Notify only for actionable events; keep care
    activity conservative; no advanced batching. Never send private senior
    information to family members outside the authorization model.
26. **Privacy.** Lock-screen text minimises sensitive information — "You have an
    appointment tomorrow at 10:00", never a condition or a medicine. Detail
    comes after the authenticated app is opened. Document the content policy.
27. **Security.** Notifications scoped to authenticated users; device tokens
    user-owned; notification ids cannot bypass authorization; push payloads
    carry no unnecessary medical information; tokens never logged; revoked
    caregivers cannot reach linked resources; service credentials stay
    server-side.
28. **Database.** A new migration. Only the tables actually required, with
    foreign keys, indexes, constraints, timestamps, and uniqueness. Do not
    modify previous migrations.
29. **Transactional consistency.** A notification must never be sent for an
    action that ultimately rolled back.
30. **Offline.** Respect the existing offline architecture; avoid duplicate
    notifications during replay.
31. **Performance.** No polling from every client, no infinite timers, no
    unbounded notification generation, no synchronous push in handlers, and no
    loading a whole notification history into memory.
32. **Mobile UX.** The inbox is reachable from the main navigation with an
    unread badge that cannot drift from the database. It works on iOS, Android,
    and web. Handle taps on cold start, warm start, and while open, and never
    lose the navigation intent during initialisation.
33. **Testing.** Backend: creation, authorization, recipient isolation,
    read/unread, mark-all-read, pagination, deduplication, preference filtering,
    device registration and deactivation, invalid tokens, retries, permission
    revocation, idempotency, scheduler concurrency. Scheduling: every reminder
    type, recurrence, cancelled and completed appointments, timezone behaviour,
    duplicate and concurrent scheduler runs — with injectable clocks rather than
    wall-clock sleeps. Mobile: inbox, unread state, mark read, mark all read,
    navigation, permission denied, push registration, token update, logout,
    preferences, deep links, cold-start navigation.
34. **Real push.** Perform at least one genuine end-to-end push test. If no
    physical device is available, document the limitation and the exact manual
    test — do not claim real-device verification.
35. **Matrix.** Record in-app, push registration, push delivery, and deep link
    per platform. Web must at minimum support the in-app inbox; if browser push
    is not implemented, document that as a deliberate limitation.
36. **Documentation.** Update `docs/IMPLEMENTATION_STATUS.md` and add a
    notifications document covering architecture, types, preferences, provider,
    device registration, scheduling, retry, retention, privacy and lock-screen
    policy, timezone behaviour, platform limitations, and manual setup. No
    secrets.
37. **Environment.** Only required configuration in `.env.example`. No Expo
    tokens, Supabase service-role credentials, or push secrets committed;
    document where production secrets belong.
38. **Regression.** Authentication, Google authentication, care circles,
    invitations, tasks, medications, appointments, care events, permissions,
    offline sync, and the web app all still work. Run the full suite, and apply
    migrations to a fresh database.

## Definition of Done

The notification domain exists and is persisted, with securely scoped
recipients, preferences, and device registration. Medication, appointment, task,
and overdue reminders work; deduplication works; the inbox, read/unread,
mark-all-read, and deep links work, with authorization rechecked on open. Push
delivery is asynchronous, invalid tokens are handled, basic retries exist,
content follows the privacy policy, timezone behaviour is documented and tested,
and scheduler concurrency is safe. iOS and Android push are tested if a device
build is available; web in-app notifications work and web push behaviour is
documented. Real push delivery is tested where possible. Existing
authentication, Google authentication, and care functionality still work.
Backend and mobile tests, typecheck, lint, and formatting pass, and
fresh-database migrations pass. Documentation is updated.

## Stop Condition

When Phase 11 is complete, stop. SMS, WhatsApp, email, voice, AI-generated
notifications, advanced batching, smart prioritisation, emergency response
automation, and wearables are separate future phases.
