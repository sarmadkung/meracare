# Security and Privacy Requirements

## Sensitive Domain

The product will handle potentially sensitive health and caregiving
information.

Treat all care and health-related data as sensitive even where a
specific regulatory classification is not yet established.

## MVP Principles

-   Encrypt data in transit.
-   Use TLS everywhere.
-   Use secure authentication.
-   Never store auth tokens in plain local storage.
-   Use SecureStore/Keychain/Keystore-backed mechanisms as appropriate.
-   Least-privilege authorization.
-   Server-side authorization for every protected resource.
-   Audit security-sensitive actions.
-   Avoid logging health data.
-   Avoid logging access tokens.
-   Minimize collected data.

### Browser session storage

Browsers do not expose Keychain/Keystore to application JavaScript. The Expo web
build therefore keeps the Supabase session in `sessionStorage`, never
`localStorage`: it survives a reload in the same tab and is removed when the tab
closes. The installation identifier is not a credential and may use
`localStorage`. A future Next.js application should prefer secure, HTTP-only
cookies where its server architecture permits them.

### Sign-out cleanup

Sign-out must deactivate the current notification device while the access token
still exists, clear OS-scheduled reminders, and remove locally cached care data.
It must first synchronize the offline mutation queue and refuse to erase care
that has not reached the server.

## Authentication

Supabase Auth handles authentication.

The Go API must validate access tokens and establish the authenticated
application user.

Do not trust role claims supplied by the client.

## Authorization

Every protected resource must verify:

1.  authenticated user;
2.  relationship to senior;
3.  action permission.

## Data Access

Do not allow a user to query arbitrary senior IDs and rely on UI
restrictions.

Backend authorization must reject unauthorized access.

Care notes and messages are protected by the same active senior relationship
checks as other care data. Notes require `notes.view` or `notes.create` as
appropriate; messages and personal read state require `messages.participate`.
An author may edit their own note only while still authorized.

## Files

Medical/care documents should use private object storage.

Use signed, short-lived URLs where necessary.

## Audit

Record:

-   permission changes
-   invitations
-   member removal
-   sensitive record access where appropriate
-   data export
-   account deletion

## Data Retention

Define retention policies before production.

MVP should avoid collecting data that is not required for the current
feature.

## Compliance

Before commercial launch in specific jurisdictions, perform a
legal/privacy review for applicable requirements such as HIPAA, GDPR, UK
GDPR, local privacy laws, consent requirements, and health-data
regulations.

Do not claim regulatory compliance merely because Supabase or another
vendor provides security features.

`privacy-policy-draft.md` is a product/legal input draft, not an approved policy.
The operator identity, contact details, retention periods, jurisdictions,
deletion process, and hosting disclosures must be supplied and legally reviewed
before it is published or linked from an app store listing. The complete release
gate is tracked in `21-release-readiness.md`.
