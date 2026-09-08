# Domain Model

## Core Entities

### User

-   id
-   auth_user_id
-   display_name
-   avatar_url
-   phone
-   created_at
-   updated_at

### SeniorProfile

-   id
-   user_id (nullable for a managed senior without an account)
-   display_name
-   date_of_birth
-   photo_url
-   phone
-   address
-   emergency_contact
-   created_at
-   updated_at

### CareRelationship

-   id
-   senior_id
-   user_id
-   role
-   permissions
-   status
-   created_at
-   updated_at

### Invitation

-   id
-   senior_id
-   inviter_user_id
-   invitee_email or invitee_phone
-   role
-   permissions
-   token/reference
-   expires_at
-   status
-   created_at

### CareTaskTemplate

Defines recurring task rules.

-   id
-   senior_id
-   title
-   description
-   assigned_user_id
-   recurrence_rule
-   due_time
-   active
-   created_at
-   updated_at

### CareTaskInstance

A concrete occurrence of a task.

-   id
-   template_id
-   senior_id
-   assigned_user_id
-   scheduled_for
-   status
-   completed_at
-   completed_by
-   notes
-   created_at
-   updated_at

Statuses:

-   `pending`
-   `completed`
-   `skipped`
-   `overdue`
-   `cancelled`

### Medication

-   id
-   senior_id
-   name
-   dosage
-   instructions
-   active
-   created_at
-   updated_at

A medication entered by mistake may be permanently deleted only while it has
no `taken` or `skipped` instance. The medication, schedules, pending instances,
and creation activity are removed together. Once any dose outcome exists, the
medication is stopped (`active = false`) so its clinical history remains.

### MedicationSchedule

-   id
-   medication_id
-   recurrence_rule
-   scheduled_time
-   active

### MedicationInstance

A scheduled medication occurrence.

-   id
-   medication_id
-   senior_id
-   scheduled_for
-   status
-   completed_at
-   completed_by
-   notes

### Appointment

-   id
-   senior_id
-   title
-   provider_name
-   location
-   scheduled_at
-   assigned_user_id
-   notes
-   status

### CareNote

-   id
-   senior_id
-   author_user_id
-   content
-   created_at
-   updated_at

Care notes are senior-scoped. A member with `notes.create` may add a note and a
member with `notes.view` may read it. Only the original author may edit a note;
the author must still be an active member with the required permission.

### CareEvent

Immutable event describing meaningful activity.

-   id
-   senior_id
-   actor_user_id
-   event_type
-   entity_type
-   entity_id
-   metadata
-   occurred_at

Examples:

-   `TASK_COMPLETED`
-   `TASK_MISSED`
-   `MEDICATION_TAKEN`
-   `MEDICATION_MISSED`
-   `APPOINTMENT_CREATED`
-   `NOTE_ADDED`
-   `MEMBER_INVITED`
-   `MEMBER_JOINED`

### Notification

-   id
-   user_id
-   type
-   title
-   body
-   entity_type
-   entity_id
-   read_at
-   created_at

### Conversation / Message

The MVP has one conversation stream per senior/care circle rather than a
separate conversation record.

-   Message
    -   id
    -   senior_id
    -   sender_user_id
    -   content
    -   created_at
-   MessageReadState
    -   senior_id
    -   user_id
    -   last_read_message_id
    -   last_read_at

Members need `messages.participate` to read, send, or advance their own read
state. Read state is a monotonic high-water mark and never moves backward.

## Data Relationships

``` text
User
  └──< CareRelationship >── SeniorProfile
                              ├──< CareTaskTemplate
                              ├──< CareTaskInstance
                              ├──< Medication
                              ├──< Appointment
                              ├──< CareNote
                              ├──< Message
                              ├──< MessageReadState
                              └──< CareEvent
```

## Design Rule

Use relational foreign keys and database constraints for core
relationships. Do not duplicate authoritative relationship state across
arbitrary JSON blobs.
