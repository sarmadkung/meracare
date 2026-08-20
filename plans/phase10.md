# MeraCare — Phase 10: Google Social Authentication

Implement **Phase 10 only**.

The goal is to add **Continue with Google** authentication to MeraCare while
preserving the existing authentication architecture.

This phase must support:

- React Native iOS
- React Native Android
- React Native Web / mobile web
- Existing email/password authentication
- Existing Supabase Auth sessions
- Existing Go API JWT verification

Do not implement GitHub, Apple, Facebook, Microsoft, or other social providers in
this phase.

## Objective

Add `Continue with Google` to the existing authentication UI:

```text
MeraCare → Continue with Google → Google → Supabase Auth → Supabase session
        → existing Go API → authenticated MeraCare user
```

The Go API must continue treating the Supabase JWT as the source of
authentication. Do not implement Google token verification inside the Go API.

## Architecture

```text
                    ┌── Email / Password
MeraCare Client ────┼── Google
                    └── Future providers
                            ↓
                       Supabase Auth → Supabase session → Go API JWT → MeraCare
```

The application should not need to know which provider was used.

## Requirements

1. **Google Cloud.** Document the OAuth client configuration required for web,
   Android, and iOS. Never commit client secrets or credential files. The client
   secret must never ship inside the mobile application.
2. **Supabase.** Enable Authentication → Providers → Google with the client ID
   and secret, using the callback URL Supabase provides. Document the redirect
   configuration; do not hardcode credentials in the repository.
3. **Environment.** Add only variables the client genuinely needs. Client
   variables must never contain the Google client secret, the Supabase service
   role key, or database credentials.
4. **Supabase client.** Use the existing client. Preserve session persistence,
   token refresh, auth state listeners, logout, and initialisation.
5. **Authentication service.** Add Google behind the existing abstraction
   (`auth.signInWithGoogle()`); do not scatter `supabase.auth.signInWithOAuth`
   through UI components.
6. **Platforms.** Distinguish web, iOS, and Android only where the platform
   genuinely differs. The public authentication interface stays identical.
7. **Mobile OAuth.** Use the supported Supabase flow with native
   redirect/deep-link handling. Do not implement a custom OAuth protocol.
8. **Deep linking.** Use the project's existing URL scheme; configure the
   matching redirect URL in Supabase.
9. **Web.** Use the existing web Supabase client and configure the deployed and
   development origins. Do not hardcode localhost into production configuration.
10. **Redirect handling.** Authentication state updates without a manual restart.
11. **Session restoration.** Verify on iOS, Android, and web using the existing
    persistence architecture. Do not create a separate Google session store.
12. **Go API.** No Google-specific logic. Verify a Google-authenticated user can
    call `GET /v1/me`.
13. **User creation.** First Google sign-in flows through the existing
    application-user provisioning. No duplicate application users.
14. **Existing user with Google.** Signing in with Google using the email of an
    existing account must not silently create a second MeraCare account. If
    linking is required, use the supported Supabase mechanism. Do not match users
    by email inside the Go API.
15. **Account linking.** One person → one MeraCare user with both identities.
    Do not create custom identity-linking tables. Document the final behaviour.
16. **New Google user.** Enters the existing MeraCare onboarding; no
    Google-specific onboarding.
17. **Existing care data.** A correctly linked identity keeps its senior
    profile, care circles, tasks, medications, appointments, activity, and
    notification preferences. Nothing is copied or recreated.
18. **UI.** Add the button to the existing authentication screen following the
    existing theme and component patterns; do not redesign the screen.
19. **Button.** Clearly identifies Google with an accessible label, pressed and
    loading states, duplicate-request prevention, and a meaningful error. Use the
    official Google mark, never a lookalike.
20. **Cancellation.** Never presented as a server error; no application user is
    created and the screen does not stay in a loading state.
21. **Errors.** Handle cancellation, network failure, invalid OAuth
    configuration, redirect failure, Supabase errors, expired sessions, and
    unknown provider errors. Never expose secrets, JWTs, internal errors, or
    stack traces.
22. **Authentication state.** `SIGNED_IN` behaves exactly as it does for email.
    No separate `GOOGLE_SIGNED_IN` state.
23. **Logout.** Provider-independent.
24. **Expired sessions.** Normal Supabase refresh; a failed refresh falls back to
    re-authentication through the existing architecture.
25. **Authorization regression.** Senior ownership, care circle membership,
    permissions, caregiver access, revoked access, and invitation permissions are
    unchanged. Authentication identifies; authorization decides.
26. **Security.** The Google client secret and Supabase service role key never
    reach the app. Access tokens, refresh tokens, authorization codes, ID tokens,
    and client secrets are never logged. Use authorization code + PKCE.
27. **Testing.** Unit tests for the authentication abstraction (success,
    cancellation, failure, loading, duplicate-click prevention, authenticated
    state, logout) with external boundaries mocked; integration testing where
    practical; at least one genuine end-to-end sign-in with a real Google account
    reaching `GET /v1/me`. Record a per-platform matrix and document any platform
    that cannot be tested rather than claiming success.
28. **Regression.** Email/password, Google, logout, and session restoration all
    work, and the full existing suite passes. Do not remove existing tests.
29. **Commands.** Use the repository's existing scripts, including
    `go test -race -count=1 ./...`, the mobile tests, `pnpm typecheck`, and
    `pnpm lint`.
30. **Documentation.** Update `docs/IMPLEMENTATION_STATUS.md` and add operational
    setup instructions.

## Out of Scope

No GitHub, Apple, Facebook, Microsoft, Twitter/X, or LinkedIn. No Google JWT
verification or OAuth endpoints in Go, no Google access tokens in Postgres, no
replacement for Supabase Auth. No `google_users` / `social_users` /
`google_accounts` tables — the application user stays keyed to the Supabase user
ID.

## Definition of Done

Google provider enabled in Supabase; Google Cloud OAuth configuration
documented; `Continue with Google` in the login UI; Google login working on web,
Android, and iOS (or the exact environment limitation documented); redirects
returning correctly; sessions persisting; logout working; email/password
unchanged; new Google users entering normal onboarding; linked users retaining
their data; account-linking behaviour tested and documented; the Go API
accepting the resulting JWT with `/v1/me` verified against a genuine
Google-authenticated session; authorization unchanged; no secrets committed; no
OAuth tokens logged; unit, integration, backend, and mobile tests passing;
typecheck, lint, and formatting passing; documentation updated.

## Stop Condition

When Phase 10 is complete, stop. Apple authentication, GitHub authentication,
social account management, MFA, passkeys, and enterprise SSO are separate future
phases.
