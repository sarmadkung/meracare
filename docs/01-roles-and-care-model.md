# Roles, Relationships, and Care Model

## Core Concepts

### User

A person with an account.

A user can have multiple relationships with multiple seniors and can
hold different roles in different care circles.

### Senior

The person receiving or independently managing care.

A senior is also allowed to be the primary user of their own care data.

### Care Circle

The group of users connected to a senior for care coordination.

Examples:

-   Senior + son
-   Senior + daughter + son
-   Senior + professional caregiver + daughter
-   Senior alone in Solo Mode
-   Senior + multiple professional caregivers

### Care Relationship

A relationship between a user and a senior.

Minimum fields:

-   user_id
-   senior_id
-   role
-   permissions
-   status
-   created_at
-   updated_at

Recommended roles:

-   `senior`
-   `family_member`
-   `professional_caregiver`
-   `care_coordinator` (future)
-   `viewer` (future)

## Solo Mode

Solo Mode is an explicit MVP requirement.

A user can create a senior profile representing themselves and use:

-   Medication
-   Tasks
-   Appointments
-   Health notes/measurements where supported
-   Activity timeline
-   Notifications
-   Personal care dashboard

No caregiver is required.

The user should not be forced to invite anyone.

The UX should communicate:

> "You can use Senior Care on your own and invite others whenever you
> need help."

A Solo user can later convert the same profile into a shared care circle
by inviting family or professional caregivers.

## Family Care

A family member can create a senior and invite other family members.

Example:

Mom - Ahmed --- son - Sara --- daughter

## Professional Care

A professional caregiver can manage many seniors.

When a professional creates a client profile, they are that care circle's
initial coordinator and receive the setup permissions needed to edit the
profile, manage tasks, medications and appointments, and invite/manage members.
Professionals invited later keep the narrower defaults granted by the inviter.

Example:

Maria - Mrs. Khan - Mr. Ahmed - Mrs. Ali

The caregiver's home view should aggregate assigned seniors and today's
workload.

## Mixed Care

Example:

Mom - Ahmed --- son - Sara --- daughter - Maria --- professional
caregiver

All operate in the same care circle with role-specific permissions.

## Ownership

Avoid assuming that the person who creates a senior owns all data
forever.

The MVP should distinguish:

-   creator
-   senior
-   care-circle members
-   permissions

This allows future transfer of coordination responsibilities.

A non-senior caregiver may leave a care circle once another active member can
manage it. An empty managed profile may be deleted as an entry mistake; after
care activity exists it is archived instead. A senior's own linked profile is
not removed through the managed-client flow.
