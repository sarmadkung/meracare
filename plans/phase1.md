You are the lead software engineer responsible for building the GenxCare MVP.

GenxCare is a Senior Care and Family Coordination application.

The repository already contains the product and engineering specifications under:

/docs

These documents are the source of truth for this project.

==================================================
1. YOUR FIRST RESPONSIBILITY
==================================================

Before writing significant application code, read and understand ALL documentation under /docs.

Start with:

1. /docs/17-architecture-decision-record.md
2. /docs/00-product-overview.md
3. /docs/01-roles-and-care-model.md
4. /docs/02-permissions-and-authorization.md
5. /docs/03-domain-model.md
6. /docs/04-care-events-and-workflows.md
7. /docs/05-api-and-backend-spec.md
8. /docs/06-mobile-architecture.md
9. /docs/07-database-and-sync.md
10. /docs/08-notifications-and-background.md
11. /docs/09-security-privacy.md
12. /docs/10-testing-and-quality.md
13. /docs/11-performance-requirements.md
14. /docs/12-tech-stack.md
15. /docs/13-mvp-screen-map.md
16. /docs/14-mvp-roadmap-and-tasks.md
17. /docs/15-v2-v3-v4-roadmap.md
18. /docs/16-agent-development-guide.md
19. /docs/18-visual-theme-and-illustrations.md

Do not start by inventing your own architecture.

Do not replace technologies specified in the documents.

Do not add major dependencies without a concrete reason.

==================================================
2. LOCKED ARCHITECTURE
==================================================

The following decisions are LOCKED for MVP.

Mobile:

- React Native
- Expo
- TypeScript
- Expo Router
- TanStack Query
- Small Zustand store
- Native React Native StyleSheet/native styling
- expo-sqlite
- expo-secure-store

Backend:

- Go
- REST API
- Modular monolith
- No microservices

Database:

- PostgreSQL
- Hosted through Supabase

Authentication:

- Supabase Auth
- Apple / Google initially
- Email authentication as defined during implementation

Storage:

- Supabase Storage where appropriate

Repository:

- pnpm workspace

Web:

- NOT required for the initial MVP
- Future web will use Next.js + TypeScript
- Future web will use the same Go API and Supabase Auth

Testing:

- Go tests for backend
- TypeScript/mobile tests as appropriate
- Playwright when a web application exists

Do NOT switch React Native to Flutter.
Do NOT switch React Native to native iOS/Android.
Do NOT switch Go to Node.js.
Do NOT switch PostgreSQL to MongoDB.
Do NOT introduce Redux.
Do NOT introduce a styling framework.
Do NOT introduce microservices.

If you encounter a genuine architectural blocker, stop and explain it before changing the architecture.

==================================================
3. PRODUCT MODES
==================================================

GenxCare is ONE application.

Do NOT build separate applications for:

- family members
- professional caregivers
- seniors

Instead, the user's relationship to a senior determines permissions and available functionality.

The MVP supports four modes:

1. Solo self-care
2. Family care
3. Professional caregiver
4. Mixed family + professional care

A user can start completely alone.

Example:

User
  ↓
Self / Senior
  ↓
Tasks
Medication
Appointments

Later:

User
  ↓
Senior
  ├── Daughter
  ├── Son
  └── Professional Caregiver

A professional caregiver can manage multiple seniors.

Example:

Professional Caregiver
  ├── Senior A
  ├── Senior B
  ├── Senior C
  └── Senior D

A family member can invite a professional caregiver.

A professional caregiver can be connected to multiple seniors.

The same senior can have both family and professional caregivers.

==================================================
4. CORE MVP
==================================================

The MVP must eventually support:

Authentication

- Sign up
- Sign in
- Session restoration
- Logout

Solo mode

- Create own senior profile
- Manage own care
- Tasks
- Medication
- Appointments
- Notes
- Activity

Family care

- Create senior
- Invite family members
- View senior dashboard
- View care activity
- View tasks
- View medication
- View appointments

Professional caregiver

- Manage multiple seniors
- View today's workload
- View assigned tasks
- Complete tasks
- Record medication completion
- Add notes

Mixed care

- Family + professional caregivers working around the same senior

Tasks

- Create task
- Assign task
- Recurring task
- Complete task
- Skip task
- Overdue state

Medication

- Create medication
- Schedule medication
- Reminder
- Mark medication taken
- Missed medication state

Appointments

- Create appointment
- Assign appointment
- Reminder
- Complete/cancel

Care Circle

- Members
- Roles
- Permissions
- Invitations
- Accept invitation
- Remove/revoke member

Activity

- Care events
- Activity timeline
- Authorized members can see relevant activity

Notifications

- Local medication/task reminders
- Push notifications
- Invitations
- Relevant activity
- Overdue/escalation notifications

Messaging

- Senior-scoped care-circle conversation

Offline

- Cache important data
- Complete core actions while offline
- Queue mutations
- Sync when connection returns

==================================================
5. VISUAL IDENTITY
==================================================

The product name is:

GenxCare

Domain:

GenxCare.app

Use the approved visual system in:

/docs/18-visual-theme-and-illustrations.md

Primary brand color:

#0F766E

Deep Teal

The overall direction is:

GREEN
with a slight
BLUE / TEAL
bias.

Supporting palette is defined in the visual documentation.

Do not invent another primary color.

Typography direction:

Inter

Visual principles:

- calm
- trustworthy
- warm
- modern
- accessible
- dignified
- human

This product is for elderly people and their families.

Do not make the UI childish.

Do not make it look like a hospital administration system.

Do not make it look like generic corporate SaaS.

Prioritize readability and simplicity.

Use large touch targets.

Use semantic theme tokens rather than hardcoded colors.

Illustration sources:

1. unDraw — primary
2. Storyset — secondary

Do not introduce another illustration library without approval.

Do not hotlink third-party illustration assets in production.

When adding an illustration:

- record source
- record asset name
- record source URL
- record license/attribution requirements

==================================================
6. ARCHITECTURE
==================================================

Use this general structure:

apps/
  mobile/
  api/

packages/
  contracts/
  config/

Keep the architecture simple.

The backend is a modular monolith.

The mobile application communicates with the Go API.

The mobile application should NOT perform business-critical database writes directly against Supabase/PostgreSQL.

Architecture:

React Native
    ↓
Supabase Auth
    ↓
JWT
    ↓
Go API
    ↓
PostgreSQL / Supabase

For server state:

React Native
    ↓
TanStack Query

For local/UI state:

React Native
    ↓
Zustand

For durable offline data:

React Native
    ↓
SQLite

==================================================
7. AUTHENTICATION
==================================================

Use Supabase Auth.

The mobile application authenticates through Supabase.

The Go backend validates the Supabase-issued JWT.

Do not trust:

- user IDs from request bodies
- role values from the client
- senior IDs without authorization
- permission values from the client

The backend determines the authenticated user from the validated token.

Keep:

Supabase auth.users

separate from:

application users table.

==================================================
8. AUTHORIZATION
==================================================

Authorization is relationship-based.

Do NOT use:

isCaregiver = true

as the primary authorization model.

A user can be:

- family member for Senior A
- professional caregiver for Senior B
- senior/self for Senior C

Every protected request must verify:

1. authenticated user
2. relationship to senior
3. permission for requested operation

Client-side UI restrictions are NOT security.

Backend authorization is mandatory.

==================================================
9. DATABASE
==================================================

Use PostgreSQL.

Core entities include:

User
SeniorProfile
CareRelationship
Invitation
CareTaskTemplate
CareTaskInstance
Medication
MedicationSchedule
MedicationInstance
Appointment
CareNote
CareEvent
Notification
Conversation
Message

Use:

- UUID identifiers
- foreign keys
- indexes
- constraints
- transactions

Avoid premature database abstraction.

Create migrations.

Do not use MongoDB.

==================================================
10. CARE EVENTS
==================================================

CareEvent is a foundational part of the architecture.

Meaningful actions should produce events.

Examples:

TASK_COMPLETED
TASK_MISSED
MEDICATION_TAKEN
MEDICATION_MISSED
APPOINTMENT_CREATED
NOTE_ADDED
MEMBER_INVITED
MEMBER_JOINED

The activity timeline should be driven by care events rather than UI state.

==================================================
11. MOBILE STATE
==================================================

Use TanStack Query for:

- API data
- caching
- mutations
- invalidation
- server synchronization

Use Zustand only for small client-side state:

- selected senior
- UI preferences
- filters
- onboarding state
- temporary UI state

Do not put server state into Zustand.

Do not create a giant global store.

==================================================
12. OFFLINE
==================================================

Use expo-sqlite.

At minimum, support offline access to:

- current senior
- care relationships
- today's tasks
- medication instances
- upcoming appointments
- recent activity

Core mutations should be queueable.

Example:

User taps:

"Complete"

Then:

UI updates immediately
    ↓
SQLite transaction
    ↓
Sync queue
    ↓
Go API
    ↓
Server confirmation

Do not build a complex CRDT system for MVP.

==================================================
13. NOTIFICATIONS
==================================================

Do NOT run continuous JavaScript timers.

Do NOT implement:

setInterval(..., 1000)

for care logic.

Use OS notification scheduling for local reminders.

Use push notifications for:

- invitations
- caregiver activity
- messages
- overdue tasks
- relevant escalation

The app should not keep JavaScript running continuously to monitor medication times.

==================================================
14. PERFORMANCE
==================================================

This application should feel fast and native.

Optimize for:

- fast launch
- fast navigation
- immediate task completion
- smooth lists
- low memory usage
- low battery usage

Use:

- virtualization
- pagination
- TanStack Query caching
- SQLite
- optimistic updates
- push notifications
- OS scheduling

Avoid:

- huge datasets in memory
- unnecessary rerenders
- aggressive polling
- continuous JS timers
- unnecessary global state
- large unoptimized images

Do not optimize prematurely.

Measure first when possible.

==================================================
15. API
==================================================

Use versioned REST APIs:

/v1/...

Core areas:

/v1/seniors
/v1/seniors/{id}/members
/v1/seniors/{id}/invitations
/v1/seniors/{id}/tasks
/v1/tasks/{id}
/v1/seniors/{id}/medications
/v1/medications/{id}
/v1/seniors/{id}/appointments
/v1/seniors/{id}/notes
/v1/seniors/{id}/activity
/v1/seniors/{id}/messages

Use consistent error responses.

Use cursor pagination for:

- activity
- messages
- large histories

Use idempotency where mutations can be retried.

==================================================
16. SECURITY
==================================================

This is a sensitive care application.

Never:

- log access tokens
- log passwords
- expose health information in logs
- trust client-side permissions
- expose arbitrary senior records
- expose private storage objects publicly

Use secure token storage.

Use private storage for sensitive files.

Minimize collected data.

Add authorization tests.

==================================================
17. TESTING
==================================================

Critical workflows must have automated tests.

At minimum:

1. User signs up
2. User signs in
3. User creates own profile
4. Family creates senior
5. Family invites caregiver
6. Caregiver accepts invitation
7. Caregiver sees assigned seniors
8. Caregiver completes task
9. Family sees completion
10. Medication is marked taken
11. Unauthorized user is rejected
12. Offline mutation syncs successfully

Backend:

- unit tests
- integration tests
- authorization tests

Mobile:

- component/logic tests where appropriate
- critical workflow tests

==================================================
18. DEVELOPMENT ORDER
==================================================

Do NOT attempt to build the entire application in one step.

Implement incrementally.

PHASE 1 — FOUNDATION

1. Inspect repository.
2. Create/update project structure.
3. Initialize pnpm workspace.
4. Initialize Expo mobile application.
5. Initialize Go API.
6. Configure TypeScript.
7. Configure linting/formatting.
8. Configure environment variables.
9. Configure Supabase.
10. Configure PostgreSQL migrations.
11. Establish database connection.
12. Establish Supabase Auth integration.
13. Establish Go JWT verification.
14. Establish API error handling.
15. Establish logging.
16. Establish basic testing infrastructure.

PHASE 2 — USER + SENIOR

17. User model.
18. SeniorProfile.
19. CareRelationship.
20. Solo mode.
21. Create senior.
22. Edit senior.
23. Senior dashboard.

PHASE 3 — INVITATIONS + CARE CIRCLE

24. Care-circle members.
25. Invitations.
26. Accept invitation.
27. Remove member.
28. Role/permission enforcement.

PHASE 4 — TASKS

29. Task templates.
30. Task instances.
31. Recurrence.
32. Assignment.
33. Completion.
34. Skip.
35. Overdue.

PHASE 5 — MEDICATION

36. Medication.
37. Medication schedule.
38. Medication instances.
39. Completion.
40. Reminder.

PHASE 6 — APPOINTMENTS

41. Appointment CRUD.
42. Assignment.
43. Reminder.
44. Completion/cancellation.

PHASE 7 — ACTIVITY

45. Care events.
46. Activity timeline.
47. Event visibility.

PHASE 8 — NOTIFICATIONS

48. Local notifications.
49. Push token registration.
50. Push notifications.
51. Overdue notification flow.

PHASE 9 — DASHBOARDS

52. Solo dashboard.
53. Family dashboard.
54. Professional caregiver dashboard.
55. Multi-senior caregiver workflow.

PHASE 10 — MESSAGING

56. Care-circle conversation.
57. Messages.
58. Read state.

PHASE 11 — OFFLINE

59. SQLite repositories.
60. Sync queue.
61. Retry.
62. Conflict handling.
63. Offline UI state.

PHASE 12 — QUALITY

64. Authorization tests.
65. Integration tests.
66. Critical mobile tests.
67. Performance profiling.
68. Accessibility review.
69. Error/empty/loading state review.
70. MVP stabilization.

==================================================
19. IMPORTANT AGENT BEHAVIOR
==================================================

Do not blindly implement every future idea.

The following are OUT OF MVP:

- AI diagnosis
- AI medical advice
- fall detection
- wearable monitoring
- pharmacy integration
- insurance
- telemedicine
- caregiver marketplace
- payroll
- billing
- agency management
- advanced clinical workflows

Do not implement them unless explicitly requested.

==================================================
20. WHEN YOU START
==================================================

Your first response should NOT contain hundreds of lines of implementation code.

First:

1. Inspect the repository.
2. Read all /docs files.
3. Determine what already exists.
4. Compare the repository against the specification.
5. Identify missing foundation.
6. Propose the implementation sequence.
7. Then begin Phase 1.

Create a concise:

/docs/IMPLEMENTATION_STATUS.md

containing:

- current repository state
- completed items
- pending items
- current phase
- blockers
- architectural decisions
- next tasks

Keep this document updated as implementation progresses.

==================================================
21. DO NOT CHANGE REQUIREMENTS SILENTLY
==================================================

If you believe a specification is wrong:

DO NOT silently change it.

Instead:

1. Explain the issue.
2. Explain the consequence.
3. Suggest the smallest alternative.
4. Wait for approval if it affects architecture or product behavior.

For ordinary implementation details that do not change the documented architecture, use your engineering judgment.

==================================================
22. DEFINITION OF DONE
==================================================

The MVP is complete only when:

- Solo users can use GenxCare without a caregiver.
- Families can create/manage a senior.
- Family members can invite caregivers.
- Professional caregivers can manage multiple seniors.
- Family + professional caregivers can work around the same senior.
- Tasks work.
- Recurring tasks work.
- Medication schedules work.
- Medication completion works.
- Appointments work.
- Care events work.
- Activity timeline works.
- Notifications work.
- Care-circle messaging works.
- Core workflows work with intermittent connectivity.
- Offline mutations synchronize correctly.
- Backend authorization is enforced.
- Critical workflows have automated tests.
- App is performant.
- App is accessible.
- Visual theme is consistent.
- Approved illustrations are used consistently.
- No V2/V3 functionality has been unnecessarily introduced.

==================================================
23. FINAL PRINCIPLE
==================================================

Build GenxCare as a real production-quality foundation, not as a disposable prototype.

But do not over-engineer it.

Prefer:

- simple
- explicit
- maintainable
- testable
- secure
- fast
- accessible

over:

- clever
- abstract
- over-engineered
- premature
- dependency-heavy

The goal is to get a solid MVP into users' hands quickly while preserving a strong architecture for future expansion.

Start by inspecting the repository and reading the documentation.
