# Apple Authentication — Setup and Behaviour

GenxCare uses Supabase's Apple OAuth provider. The client contains no Apple
private key or client secret; those remain in Apple Developer and Supabase.

## Repository implementation

- Native: `apps/mobile/src/features/auth/apple.ts`
- Web: `apps/mobile/src/features/auth/apple.web.ts`
- Shared result: `apps/mobile/src/features/auth/apple-result.ts`
- Provider-neutral action: `apps/mobile/src/features/auth/use-auth-actions.ts`
- Sign-in control: `apps/mobile/src/components/ui/apple-button.tsx`

Both native and web return to `/auth/callback`; native uses the `genxcare` URL
scheme. Supabase exchanges the PKCE authorization code into the same session
shape used by email/password and Google.

## External setup

1. In Apple Developer, configure Sign in with Apple for bundle identifier
   `app.genxcare.mobile` and create the Services ID/key required by Supabase.
2. In Supabase Authentication → Providers → Apple, enable Apple and enter the
   Apple client identifiers and secret generated from the Apple key.
3. Allow-list `genxcare://auth/callback`, local web callback URLs, and the real
   production callback URL in Supabase.
4. Test new sign-in, cancellation, returning sign-in, account linking, and
   revoked credentials on physical Apple hardware.

Never commit the Apple private key, generated client secret, or provider
credentials. Record ownership and rotation dates in the team's private system.
