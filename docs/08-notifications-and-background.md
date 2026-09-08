# Notifications and Background Processing

## Principle

Do not keep JavaScript running continuously just to check time.

Use OS-native scheduling and push notifications.

## Local Notifications

Use for:

-   medication reminders
-   scheduled care tasks
-   appointments

The schedule should be synced to the device.

Example:

``` text
Server schedule
  ↓
Mobile sync
  ↓
OS local notification
  ↓
User action
```

Medication notifications offer three OS actions:

-   **Taken** records the dose immediately after authentication; offline actions
    enter the durable sync queue.
-   **Skip** opens the app and requires confirmation before recording.
-   **Remind in 10 min** schedules one privacy-preserving local follow-up.

These are notification actions, not a persistent full-screen alarm. The sound
and presentation remain controlled by the operating system.

## Remote Push

Use push notifications for:

-   missed tasks
-   missed medication doses
-   caregiver activity
-   invitations
-   messages
-   relevant care-circle events

Architecture:

``` text
Domain event
  ↓
Notification decision
  ↓
Push provider
  ↓
APNs / FCM
  ↓
User device
```

## Background Work

Use background execution only when justified.

Potential MVP jobs:

-   sync pending mutations
-   refresh relevant cached data
-   reconcile notification schedules

Do not run high-frequency timers.

Avoid:

``` text
setInterval(..., 1000)
```

for care logic.

## Escalation

MVP should support configurable escalation.

Example:

``` text
Task due
  ↓
Reminder
  ↓
Grace period
  ↓
Overdue
  ↓
Notify assigned caregiver
  ↓
Optional family notification
```

Medication escalation uses the medication domain's existing two-hour grace
period. If a dose is still pending when that period expires, notify every
active care-circle member who can view medications and has missed-medication
alerts enabled. This includes the senior in solo self-care.

The escalation system should not imply medical emergency unless a
separately implemented emergency feature exists.

## Notification Preferences

Users should control:

-   task reminders
-   medication reminders
-   missed medication alerts
-   activity notifications
-   messages
-   invitations
-   escalation alerts

Respect OS notification permissions.
