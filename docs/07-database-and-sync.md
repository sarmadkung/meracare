# Database and Sync Strategy

## Locked Decision

The primary database is:

> **PostgreSQL hosted by Supabase.**

The mobile app does not directly perform business-critical writes
against PostgreSQL.

The application architecture is:

``` text
React Native
    ↓
Supabase Auth
    ↓ JWT
Go API
    ↓
PostgreSQL on Supabase
```

## Supabase Responsibilities

Use Supabase for:

-   managed PostgreSQL
-   Supabase Auth
-   social authentication
-   email/password/OTP authentication where enabled
-   private object storage where appropriate
-   database management and migrations/tooling

The Go backend remains the authoritative application/API layer.

This means Supabase is infrastructure, not the business-logic layer.

## Authentication

The mobile application uses the Supabase Auth SDK.

Flow:

``` text
User
  ↓
React Native
  ↓
Supabase Auth
  ↓
Session / JWT
  ↓
Go API
  ↓
JWT validation
  ↓
Application authorization
```

The Go API must validate the Supabase-issued JWT and derive the
authenticated application user from it.

Never trust a user ID supplied by the mobile client.

## Social Authentication

Initial social providers:

-   Apple
-   Google

Additional providers may be enabled later without changing the core user
model.

Supabase Auth supports OAuth/social providers for mobile and web
applications.

## Application User

Keep Supabase `auth.users` separate from the application's `users`
table.

Recommended relationship:

``` text
auth.users.id
     │
     ▼
users.auth_user_id
```

Application-specific profile and role data belongs in application
tables.

## Local Database

Use `expo-sqlite` for the mobile durable local database.

Do not mirror every server table automatically.

MVP local data should focus on:

-   current user/profile
-   seniors the user can access
-   care relationships
-   today's tasks
-   current medication schedules/instances
-   upcoming appointments
-   recent activity
-   pending sync operations

Care notes and care-circle messages remain server-authoritative in MVP and are
refetched through TanStack Query. They are not copied into the durable mutation
queue or SQLite cache; creating or editing them therefore requires a connection.

## Sync Queue

A local mutation queue should contain:

-   operation_id
-   entity_type
-   entity_id
-   operation_type
-   payload
-   created_at
-   retry_count
-   last_error
-   status

Mutation APIs must be idempotent where retries can occur.

Before sign-out, the mobile app drains this queue. Sign-out is refused while an
operation remains pending or failed: deleting it would lose recorded care, and
leaving it for the next account could replay somebody else's action. Once the
queue is empty, cached care data is removed from SQLite.

## Conflict Strategy

For MVP:

-   server authoritative for permissions and relationships
-   server authoritative for final state
-   completion mutations semantically idempotent through their terminal state
-   avoid destructive overwrites
-   preserve care-event history
-   do not silently overwrite important care/health records

A sophisticated CRDT is not required.

## Storage

Private documents and images should use Supabase Storage or another
private object store.

Do not store large files directly in PostgreSQL.

## Database Constraints

Use:

-   foreign keys
-   unique constraints
-   check constraints
-   appropriate indexes
-   transactions

Database constraints must reinforce application authorization and data
integrity.

The current application schema is managed by migrations `0001` through `0010`.
Migration `0010_notes_and_messages.sql` adds `care_notes`, `messages`, and
`message_read_states`, including senior-scoped indexes, content-length checks,
foreign keys, and the read-state persistence used by the API.
