# Google Authentication — Setup and Behaviour

How "Continue with Google" is wired, and the console configuration it needs.
Implemented in Phase 10 (`plans/phase10.md`).

Nothing in this document is a secret. The Google client secret exists in exactly
one place — the Supabase dashboard — and must never appear in this repository,
in an environment file, or in a chat message.

## Shape

```text
MeraCare → Google → Supabase Auth → Supabase session → Go API → MeraCare user
```

Supabase is the authentication authority. It holds the Google client secret and
performs the OAuth code exchange with Google, so the app never sees either. The
Go API is unchanged: it verifies the Supabase JWT exactly as it does for an
email sign-in, and `app_metadata.provider` is recorded but never gated on
(`apps/api/internal/auth/token.go`).

There is no `google_users` table, no Google token in Postgres, and no Google
verification in Go.

## Client Configuration Required

Both consoles have to be configured before the button can work. Neither is in
version control, and there is no way to do it from code.

### 1. Google Cloud

In a Google Cloud project, configure the OAuth consent screen and create
credentials under **APIs & Services → Credentials**:

| Item                | Value                                                   |
| ------------------- | ------------------------------------------------------- |
| Application type    | Web application                                         |
| Authorised origin   | `https://<project-ref>.supabase.co`                     |
| Authorised redirect | `https://<project-ref>.supabase.co/auth/v1/callback`    |

One **Web application** client covers all three platforms. iOS and Android use
the same client because the OAuth exchange happens on Supabase, not on the
device — the app only opens a browser and receives a deep link back. Separate
Android and iOS OAuth clients are needed only for the native Google Sign-In SDKs
(`@react-native-google-signin/google-signin`), which MeraCare deliberately does
not use: they would require the app to handle Google credentials directly and
add a dependency outside the locked stack in `AGENTS.md`.

The consent screen needs the `email`, `profile`, and `openid` scopes — nothing
more. MeraCare asks Google for identity only, never for Gmail, Calendar, Drive,
or Contacts access.

Record in the project's own notes (not here): the Google Cloud project id, the
client id, and which account owns them. Never the client secret.

### 2. Supabase

**Authentication → Providers → Google**:

| Setting              | Value                                                |
| -------------------- | ---------------------------------------------------- |
| Enabled              | Yes                                                  |
| Client ID (for OAuth)| The Google web client id                             |
| Client Secret        | The Google web client secret                         |
| Callback URL         | Supplied by Supabase; paste into Google Cloud above  |

**Authentication → URL Configuration → Redirect URLs** must allow-list every
origin the app returns to. A redirect that is not listed is rejected and the
person lands back on a Supabase error page instead of in MeraCare:

```text
meracare://auth/callback          # iOS and Android (the app.json scheme)
http://localhost:8081/auth/callback   # Expo web dev server
https://<deployed-web-host>/auth/callback
```

Add the deployed host when the web app is deployed; do not leave development
origins in a production project beyond what is needed.

Verify the provider is live without opening the dashboard:

```bash
curl -s "$SUPABASE_URL/auth/v1/settings" -H "apikey: $SUPABASE_ANON_KEY" \
  | python3 -c 'import sys,json; print(json.load(sys.stdin)["external"]["google"])'
```

`False` means the app will show *"Unsupported provider: provider is not
enabled"* — that message comes from Supabase and is the expected symptom of an
unconfigured project, not a bug in the client.

## Environment Variables

**None were added.** The flow needs only the Supabase URL and anon key the app
already has. There is deliberately no `EXPO_PUBLIC_GOOGLE_CLIENT_ID`: the
browser-based flow never needs one on the device, and every value the app could
hold would be one more thing to keep in step across three platforms.

## Implementation

| File                                          | Role                                        |
| --------------------------------------------- | ------------------------------------------- |
| `src/features/auth/google-result.ts`          | The shared result type and callback path    |
| `src/features/auth/google.ts`                 | iOS and Android                             |
| `src/features/auth/google.web.ts`             | Web                                         |
| `src/features/auth/use-auth-actions.ts`       | `signInWithGoogle()` beside the email actions |
| `src/components/ui/google-button.tsx`         | The button, with Google's own mark          |
| `src/app/auth/callback.tsx`                   | Where the OAuth redirect lands              |

Screens call `useAuthActions()`. Nothing outside `features/auth` touches
`supabase.auth.signInWithOAuth`, and nothing outside these files knows which
provider signed a person in — `SessionProvider` sees the same `SIGNED_IN` event
either way, and sign-out is provider-independent.

**Native.** The app asks Supabase for the authorization URL
(`skipBrowserRedirect: true`), opens it with
`WebBrowser.openAuthSessionAsync`, and reads the authorization code off the
`meracare://auth/callback` deep link the browser session returns. It then calls
`exchangeCodeForSession`, which pairs the code with the PKCE verifier the client
stored when the flow began.

**Web.** `signInWithOAuth` navigates the page to Google; the browser comes back
to `<origin>/auth/callback` with the code, and `detectSessionInUrl` — already
enabled for web in `src/lib/supabase.ts` — exchanges it and emits `SIGNED_IN`.

**PKCE** is set explicitly (`flowType: 'pkce'`) in `src/lib/supabase.ts`. No
token ever appears in a redirect URL, and an intercepted code is useless without
the verifier held by the client that started the flow.

If the web app is served as a static export, the host must serve
`/auth/callback` — the export produces `auth/callback.html`, so a host that does
not fall back to it needs a rewrite rule.

### Expo Go

The native flow uses `expo-web-browser` and a custom URL scheme, both of which
are in the Expo Go runtime, so it runs in Expo Go **provided the redirect that
Expo Go actually generates is allow-listed**. `Linking.createURL` returns an
`exp://…` URL there rather than `meracare://`, so add whatever the dev client
prints to the Supabase redirect list, or use a development build to get the real
scheme. A development build is the reliable option and the only one that matches
production.

## Accounts and Linking

MeraCare application users are keyed to `auth.users.id`
(`users.auth_user_id`), never to an email address. What happens when someone
uses Google with the address of an existing email/password account is therefore
decided entirely by Supabase:

- **Automatic linking is on by default.** Google returns a verified email, and
  Supabase attaches that identity to the existing `auth.users` row when the
  address matches a confirmed account. The `sub` claim is unchanged, so the Go
  API resolves the same `users.id` and the person keeps their seniors, care
  circles, tasks, medications, appointments, activity, and notification
  preferences. Nothing is copied or recreated.
- **If linking is disabled** in the project, Supabase creates a second
  `auth.users` row and MeraCare gets a second application user with its own
  empty care data. `EnsureByAuthUserID` tolerates the duplicate address (it
  stores the second user without an email rather than refusing the sign-in), but
  the two accounts stay separate. Leave automatic linking enabled unless there is
  a decision to do otherwise.

Deliberately **not** implemented: matching users by email inside the Go API. Two
Supabase identities that happen to share an address are not proof of one person,
and treating them as one in the API would hand someone else's care records to
whoever could register the same address.

A first-time Google user is provisioned by the same `users.EnsureByAuthUserID`
path as an email user and enters the existing onboarding flow. There is no
Google-specific onboarding.

## Security

- The Google client secret lives only in Supabase and never reaches the app.
- The Supabase service role key is not in the mobile app at all.
- Authorization code + PKCE; no token is returned in a redirect URL.
- Sessions stay in the Keychain/Keystore via `expo-secure-store`, unchanged.
- No code path logs an access token, refresh token, authorization code, ID
  token, or client secret. Unexpected throws are replaced with a fixed message
  before reaching the screen, because a raw provider error can carry a callback
  URL.
- Cancellation is a distinct result from failure, so backing out of Google
  produces no error message and no user record.
- Authorization is untouched: the provider decides nothing about what a person
  may do. Relationship-based permissions
  (`docs/02-permissions-and-authorization.md`) apply exactly as before.

## Verifying It

```bash
cd apps/mobile && pnpm test && pnpm typecheck && pnpm lint
cd apps/api && gofmt -l . && go vet ./... && go test -race -count=1 ./...
npx expo export --platform all --output-dir /tmp/mc-export   # web, iOS, Android
```

End to end, once the consoles are configured, on each of iOS, Android, and web:

1. Tap **Continue with Google** and complete the Google screen.
2. Confirm the app returns and lands on the home screen without a restart.
3. Call `GET /v1/me` with the session's access token and confirm a 200 carrying
   the expected user.
4. Close and reopen the app; confirm the session is restored.
5. Sign out; confirm the sign-in screen returns and the care data is gone.
6. Tap **Continue with Google** and back out of the Google screen; confirm the
   app returns to sign-in with no error and no new account.
7. With an existing email/password account, sign in with Google using the same
   address; confirm the existing seniors and care data are still there.
