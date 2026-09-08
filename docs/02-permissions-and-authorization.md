# Permissions and Authorization

## Principle

Authorization is relationship-based, not merely role-based.

A user may be a professional caregiver for one senior and a family
member for another.

Never use a global `is_caregiver` flag as the primary authorization
mechanism.

## MVP Permission Groups

### Senior

Can:

-   View own profile.
-   View own tasks.
-   Complete own tasks.
-   View own medications.
-   Record medication completion.
-   View appointments.
-   Add permitted notes.
-   View care-circle members.
-   Invite people if enabled by the care-circle policy.

### Family Member

Default access:

-   View senior overview.
-   View tasks and completion.
-   View medications and adherence status.
-   View appointments.
-   View activity timeline.
-   View care notes.
-   Participate in care-circle chat.
-   Invite additional members if allowed.

### Professional Caregiver

Default access:

-   View assigned seniors.
-   View assigned care tasks.
-   Complete assigned tasks.
-   View relevant medications.
-   Record medication completion.
-   View appointments.
-   Add care notes.
-   View relevant activity.

Professional caregivers must not automatically see unrelated
family/private information.

The professional role defaults apply to invited caregivers. A professional who
creates a client is also the initial coordinator for that specific care circle
and receives `senior.edit`, `tasks.manage`, `medications.manage`,
`appointments.manage`, `members.invite`, and `members.manage`. This is stored on
that relationship; it does not widen the professional role globally.

A caregiver may revoke their own relationship only through the leave action,
and only when another active relationship holds `members.manage`. This prevents
an orphaned care circle while still letting a caregiver end access voluntarily.

### Care Coordinator

Reserved for future organization/agency workflows.

## Least Privilege

Every endpoint must authorize:

1.  authenticated user;
2.  relationship to the senior;
3.  permission for the requested resource/action.

Never rely only on client-side hiding.

## Invitation Rules

An invitation must include:

-   senior_id
-   inviter_user_id
-   invited email/phone
-   proposed role
-   proposed permissions
-   expiration
-   status

The backend must validate the inviter's authority.

## Auditability

Authorization-sensitive events should be recorded:

-   invitation sent
-   invitation accepted
-   member removed
-   permission changed
-   care relationship created
-   care relationship revoked
