# MVP Screen Map

## Visual System

Use the locked visual source of truth in
`18-visual-theme-and-illustrations.md`.

-   Primary direction: green with a slight blue/teal bias.
-   Primary brand color: `#0F766E` Deep Teal.
-   Illustration sources: unDraw (primary) and Storyset (secondary).
-   Do not introduce a new primary color or illustration library for MVP
    without explicit approval.

## Visual Direction

All screens use the visual system in
`18-visual-theme-and-illustrations.md`.

Primary:

`#0F766E` Deep Teal

The visual system should prioritize readability, large touch targets,
calm surfaces, and restrained illustrations.

## Authentication

1.  Welcome
2.  Sign in
3.  Sign up
4.  Verification

## Onboarding

5.  Choose how to use the app
    -   For myself
    -   Care for family
    -   Professional caregiver
6.  Create profile
7.  Create/join care circle
8.  Add senior

Use a welcoming illustration from unDraw or Storyset on onboarding where
appropriate.

## Main Application

9.  Home / Today
10. Seniors
11. Senior dashboard
12. Care tasks
13. Task details
14. Create/edit task
15. Medications
16. Medication details
17. Appointments
18. Appointment details
19. Activity timeline
20. Notes
21. Care circle
22. Invite member
23. Messages

Notes and Messages remain required MVP screens. Both were implemented on
2026-08-24 using the same senior-scoped relationship and permission model as the
rest of the application. The senior dashboard exposes each destination only to
members with its required permission.

Lifecycle actions stay on their existing context screens rather than adding
new screens: medication details offers conditional mistaken-entry deletion;
the Care circle offers caregiver self-removal; and the managed senior profile
card offers delete-or-archive removal. Each uses a destructive confirmation and
permission-aware visibility.

## Settings

24. Profile
25. Notifications
26. Account/settings

## Screen Behavior

The same screens should adapt based on relationship and permissions.

Do not create:

-   Family App screens
-   Professional App screens

as separate application architectures.

Instead:

``` text
User
  ↓
Relationship
  ↓
Permissions
  ↓
UI capabilities
```

## Senior Home

Prioritize:

-   current status
-   next task
-   medication
-   appointment
-   help/contact
-   simple language
-   large touch targets

Use illustrations only where they reinforce understanding and do not
compete with important care information.

## Professional Home

Prioritize:

-   today's assigned seniors
-   overdue tasks
-   next visit/task
-   quick completion
-   quick notes

## Family Home

Prioritize:

-   senior status
-   care completion
-   attention items
-   upcoming appointments
-   caregiver activity

## Solo Home

Prioritize:

-   today's own routine
-   medication
-   tasks
-   appointments
-   health/care notes
-   optional invitation to add helpers

No caregiver should be required.
