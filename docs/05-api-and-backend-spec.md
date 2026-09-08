# API and Backend Specification

## Backend

-   Go
-   REST API for MVP
-   PostgreSQL
-   Supabase Auth
-   Supabase Storage where appropriate
-   Background jobs for reminders/escalations
-   Structured logging
-   OpenAPI documentation

The maintained machine-readable contract is [`openapi.yaml`](openapi.yaml).

## API Principles

-   Version APIs: `/v1/...`
-   JSON request/response.
-   UUID primary identifiers.
-   ISO-8601 timestamps.
-   Explicit validation.
-   Relationship-based authorization.
-   Semantic idempotency for retryable terminal actions: repeating the same
    task, dose, or appointment outcome returns the original result and writes
    no duplicate care event. Introduce stored HTTP idempotency keys only for a
    future mutation whose result cannot be derived safely from domain state.

## Core Endpoint Groups

### Auth/Profile

The mobile client obtains the Supabase access token.

Backend validates the JWT and maps the Supabase user to the application
user.

### Seniors

``` text
GET    /v1/seniors
POST   /v1/seniors
GET    /v1/seniors/{id}
PATCH  /v1/seniors/{id}
DELETE /v1/seniors/{id}
```

### Care Circle

``` text
GET    /v1/seniors/{id}/members
DELETE /v1/seniors/{id}/members/me
POST   /v1/seniors/{id}/invitations
DELETE /v1/seniors/{id}/members/{memberId}
PATCH  /v1/seniors/{id}/members/{memberId}
```

### Tasks

``` text
GET    /v1/seniors/{id}/tasks
POST   /v1/seniors/{id}/tasks
GET    /v1/tasks/{id}
PATCH  /v1/tasks/{id}
POST   /v1/tasks/{id}/complete
POST   /v1/tasks/{id}/skip
```

### Medications

``` text
GET    /v1/seniors/{id}/medications
POST   /v1/seniors/{id}/medications
PATCH  /v1/medications/{id}
DELETE /v1/medications/{id}
POST   /v1/medications/{id}/instances/{instanceId}/take
POST   /v1/medications/{id}/instances/{instanceId}/skip
POST   /v1/medications/instances/{instanceId}/take
POST   /v1/medications/instances/{instanceId}/skip
```

The direct instance routes support privacy-preserving notification actions: the
notification carries a dose identifier but no medication details. The API
resolves the dose and requires `medications.record` for its senior; unauthorized
and unknown identifiers return the same 404. All four action routes share the
same idempotent transition.

Medication deletion returns `204` only before a taken/skipped dose exists; it
returns `409` once history requires the medication to be stopped instead.
Senior deletion returns `{ "disposition": "deleted" | "archived" }`.

### Appointments

``` text
GET    /v1/seniors/{id}/appointments
POST   /v1/seniors/{id}/appointments
PATCH  /v1/appointments/{id}
```

### Notes

``` text
GET    /v1/seniors/{id}/notes
POST   /v1/seniors/{id}/notes
PATCH  /v1/notes/{id}
```

### Activity

``` text
GET /v1/seniors/{id}/activity?cursor=...
```

### Messages

``` text
GET  /v1/seniors/{id}/messages?cursor=...
POST /v1/seniors/{id}/messages
POST /v1/seniors/{id}/messages/read
```

## Pagination

Use cursor-based pagination for activity, messages, and potentially task
histories.

Avoid offset pagination for large event timelines.

## Error Format

Use a consistent structure:

``` json
{
  "error": {
    "code": "TASK_NOT_ASSIGNED",
    "message": "You are not allowed to complete this task."
  }
}
```

Do not expose internal database or stack-trace details.

## Backend Package Structure

Recommended:

``` text
cmd/
  api/

internal/
  auth/
  users/
  seniors/
  relationships/
  invitations/
  tasks/
  medications/
  appointments/
  notes/
  events/
  notifications/
  messages/
  storage/

pkg/
  http/
  validation/
  logging/
```

Avoid premature microservices.
