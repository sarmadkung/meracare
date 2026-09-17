````md
# GenxCare — Phase 3: Care Circle & Invitations

Phase 2 is complete and merged.

Implement **Phase 3 only**.

Do not start Phase 4 or implement unrelated features.

## Objective

Build the Care Circle collaboration layer.

Users must be able to invite and manage:

- Family members
- Professional caregivers

around a senior.

The same senior can have multiple family members and professional caregivers.

A professional caregiver can be connected to multiple seniors.

The Phase 2 relationship-based authorization model must remain the foundation.

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
- `docs/09-security-privacy.md`
- `docs/10-testing-and-quality.md`
- `docs/12-tech-stack.md`
- `docs/13-mvp-screen-map.md`
- `docs/14-mvp-roadmap-and-tasks.md`
- `docs/18-visual-theme-and-illustrations.md`

Inspect the complete Phase 2 implementation before making changes.

## 1. Care Circle

A Care Circle belongs to a senior.

Example:

```text
Senior
├── Family Member
├── Family Member
└── Professional Caregiver
````

Use the existing `CareRelationship` model.

Do not create separate relationship systems for family and professional caregivers.

A professional caregiver must be able to have relationships with multiple seniors.

## 2. Invitations

Implement invitations for adding a user to a senior's Care Circle.

An invitation should support:

* senior
* inviter
* recipient
* intended role
* permissions
* secure token
* status
* expiration
* timestamps

Follow the existing domain specification for exact fields and vocabulary.

The client must not be allowed to grant arbitrary permissions.

The backend must validate all requested permissions.

## 3. Roles

Support invitations for:

### Family Member

A family member can participate in the senior's care.

### Professional Caregiver

A professional caregiver can participate in the senior's care and can later manage multiple seniors.

Do not create separate applications for these roles.

## 4. Permissions

Continue using the Phase 2 model:

```text
CareRelationship
├── role
└── permissions[]
```

Permissions are stored per relationship.

Do not derive all permissions dynamically from the role.

The inviter may only grant permissions they are authorized to grant.

The backend must validate permissions.

Never trust a permission array supplied by the client.

## 5. Permission Escalation

Prevent users from granting permissions they are not allowed to delegate.

Examples that must fail:

```text
Caregiver
→ grants themselves OWNER
```

```text
Caregiver with VIEW_TASKS
→ grants another user MANAGE_MEMBERS
```

The backend must enforce permission delegation.

## 6. Invitation Security

Invitation tokens must be:

* cryptographically random
* sufficiently long
* single-purpose
* time-limited
* invalidated after acceptance/revocation

Do not use predictable values such as:

* user IDs
* senior IDs
* timestamps
* sequential IDs

Prefer storing a secure hash of the invitation token rather than the raw token where practical.

## 7. Invitation Lifecycle

Support the statuses defined in the domain specification.

At minimum:

```text
PENDING
ACCEPTED
REVOKED
EXPIRED
```

Rules:

* Pending invitations can be accepted.
* Accepted invitations cannot be reused.
* Revoked invitations cannot be accepted.
* Expired invitations cannot be accepted.
* Tokens cannot be reused after successful acceptance.

Expiration must be checked by the API itself. Do not rely only on background cleanup.

## 8. Accept Invitation

Support:

```text
Invitation
    ↓
Authentication
    ↓
Validation
    ↓
Accept
    ↓
CareRelationship
```

If the recipient does not have an account:

```text
Invitation
    ↓
Create account
    ↓
Accept invitation
    ↓
CareRelationship
```

Do not create the relationship before acceptance.

Do not create duplicate application users.

## 9. API

Implement the invitation and Care Circle endpoints defined in the API specification.

Likely areas include:

```text
POST /v1/seniors/{id}/invitations
GET /v1/seniors/{id}/invitations
GET /v1/invitations/{token}
POST /v1/invitations/{token}/accept
POST /v1/invitations/{id}/revoke

GET /v1/seniors/{id}/members
PATCH /v1/seniors/{id}/members/{relationshipId}
DELETE /v1/seniors/{id}/members/{relationshipId}
```

Use the existing API specification as the final authority on exact routes and request/response formats.

Do not create duplicate endpoints if equivalent endpoints already exist.

## 10. Authorization

Every Care Circle operation must use the Phase 2 authorization system.

Examples:

* Viewing members requires appropriate permission.
* Creating invitations requires invitation permission.
* Changing relationship permissions requires permission-management rights.
* Revoking members requires member-management permission.
* Accepting an invitation requires satisfying the invitation recipient rules.

Never trust IDs supplied by the client without authorization checks.

Maintain Phase 2's unauthorized-resource behavior.

## 11. Relationship Revocation

When a member is removed:

* Preserve the relationship record where required.
* Mark it revoked/inactive.
* Immediately remove active access.
* Preserve historical authorship.

A revoked caregiver must no longer be able to access the senior.

Do not delete historical records simply because a relationship is revoked.

## 12. Multiple Seniors

A professional caregiver must support multiple relationships:

```text
Professional Caregiver
├── Senior A
├── Senior B
└── Senior C
```

Do not add a single `caregiver_senior_id` field to the user.

Use `CareRelationship`.

Do not build the full professional caregiver dashboard yet.

## 13. Multiple Family Members

A senior must support multiple family relationships:

```text
Senior
├── Son
├── Daughter
├── Daughter
└── Spouse
```

Do not store family membership directly on the senior profile.

Use `CareRelationship`.

## 14. Care Circle UI

Implement the Care Circle screen according to the existing screen specification.

Display:

* senior
* active members
* role
* relevant relationship information
* available actions based on permissions

Management controls must only be shown to users who can perform those actions.

## 15. Invite Member UI

Implement an invitation flow:

1. Select role.
2. Enter recipient information.
3. Select allowed permissions where applicable.
4. Review.
5. Send invitation.

Do not expose raw permission identifiers.

Use human-readable labels.

For example:

```text
Can view care information
Can manage tasks
Can complete tasks
Can view medications
Can view appointments
```

Use only permissions defined by the domain specification.

## 16. Pending Invitations

Authorized users should be able to:

* view pending invitations
* revoke pending invitations

Unauthorized users must not be able to manage invitations.

## 17. Accept Invitation UI

Display:

* inviter
* senior
* role
* relevant permissions
* accept/reject actions

After acceptance:

```text
Invitation
    ↓
Accepted
    ↓
CareRelationship
    ↓
Senior becomes accessible
```

## 18. Emergency Contact

Do not introduce blanket role-based emergency-contact rules.

Emergency-contact access must follow the existing permission model.

Do not implement:

```text
role = professional caregiver
→ automatically grants emergency-contact access
```

unless explicitly required by the documented permission model.

## 19. Care Events

Do not build a separate event system during this phase.

If the documentation requires invitation/member events but the event infrastructure belongs to a later phase:

* defer event implementation
* document the requirement in `docs/IMPLEMENTATION_STATUS.md`
* do not create a parallel event system

## 20. State Management

Use TanStack Query for:

* Care Circle members
* invitations
* invitation details
* relationship permissions

Use mutations for:

* creating invitations
* revoking invitations
* accepting invitations
* updating relationship permissions
* revoking relationships

Use Zustand only for temporary UI state.

Do not store Care Circle server data in Zustand.

## 21. Offline

Do not implement offline invitation acceptance or permission management.

These operations require server confirmation.

Do not build additional synchronization infrastructure for this phase.

## 22. Database

Create the required invitation migration(s).

Use:

* UUIDs
* foreign keys
* indexes
* unique constraints
* status constraints
* expiration timestamps
* created/updated timestamps

Add appropriate indexes for:

* senior
* inviter
* recipient
* token lookup
* status
* expiration

Use secure token storage.

## 23. Security Tests

Test:

### Invitation security

* invalid token rejected
* expired token rejected
* revoked token rejected
* accepted token cannot be reused
* token cannot be guessed/predicted

### Authorization

* stranger cannot view invitations
* stranger cannot revoke invitations
* unauthorized member cannot invite
* unauthorized member cannot manage members
* caregiver cannot escalate permissions
* caregiver cannot grant unauthorized permissions
* revoked member cannot access senior
* user cannot accept another user's invitation

Maintain the Phase 2 404 behavior for unauthorized senior resources.

## 24. Backend Tests

At minimum test:

1. Create invitation.
2. Create invitation with invalid permission.
3. Create invitation without required permission.
4. Retrieve invitation.
5. Accept invitation.
6. Accept expired invitation.
7. Accept revoked invitation.
8. Accept already accepted invitation.
9. Revoke invitation.
10. List Care Circle members.
11. Update relationship permissions.
12. Prevent permission escalation.
13. Revoke relationship.
14. Revoked member loses access.
15. Historical relationship remains intact.
16. Professional caregiver can belong to multiple seniors.
17. Multiple family members can belong to one senior.

Run:

```bash
go test -race ./...
```

Also run all relevant mobile, TypeScript, lint, and formatting checks.

## 25. Mobile Tests

Test:

1. Care Circle loads.
2. Active members are displayed.
3. Authorized user can open invite flow.
4. Unauthorized user does not see invite controls.
5. Invitation can be sent.
6. Pending invitation appears.
7. Invitation can be revoked.
8. Invitation can be accepted.
9. Accepted invitation provides senior access.
10. Revoked relationship removes access.

## 26. Visual Requirements

Follow:

`docs/18-visual-theme-and-illustrations.md`

Primary color:

```text
#0F766E
```

Use semantic theme tokens.

Maintain:

* large touch targets
* readable typography
* accessible contrast
* clear role labels
* clear permission descriptions
* calm surfaces
* minimal visual clutter

Use unDraw or Storyset only when an illustration improves the experience.

## 27. Do Not Implement

Do not implement:

* tasks
* recurring tasks
* medication schedules
* medication reminders
* appointments
* messaging
* full care-event timeline
* professional caregiver dashboard
* advanced notifications
* complete offline synchronization

Those belong to later phases.

## 28. Documentation

Update:

`docs/IMPLEMENTATION_STATUS.md`

Record:

* Phase 3 status
* completed functionality
* remaining functionality
* database changes
* security decisions
* permission decisions
* tests
* blockers
* next phase

Do not silently change architectural decisions.

## 29. Definition of Done

Phase 3 is complete when:

* Family members can be invited.
* Professional caregivers can be invited.
* Invitation tokens are secure.
* Invitations expire.
* Invitations can be revoked.
* Invitations can be accepted.
* Existing users can accept invitations.
* New users can create an account and accept invitations.
* Care Circle members can be listed.
* Multiple family members can belong to one senior.
* Professional caregivers can belong to multiple seniors.
* Relationship permissions are enforced.
* Permission escalation is prevented.
* Members can be revoked.
* Revoked members immediately lose access.
* Historical authorship is preserved.
* Backend authorization tests pass.
* Mobile Care Circle UI works.
* Invitation UI works.
* Accept/revoke flows work.
* Type checks pass.
* Lint passes.
* Tests pass.

When Phase 3 is complete, stop.

Do not automatically continue to Phase 4.

```
```
