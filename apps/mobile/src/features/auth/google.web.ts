import { supabase } from '@/lib/supabase';

import { GOOGLE_REDIRECT_PATH, type GoogleSignInResult } from './google-result';

/**
 * Google sign-in on the web build.
 *
 * The browser navigates to Google and comes back to this origin carrying the
 * authorization code. `detectSessionInUrl` is enabled for web in
 * `src/lib/supabase.ts`, so the client exchanges that code and emits
 * `SIGNED_IN` on its own — there is nothing to await here, because a successful
 * call leaves the page.
 */
export async function signInWithGoogle(): Promise<GoogleSignInResult> {
  const { error } = await supabase.auth.signInWithOAuth({
    provider: 'google',
    options: { redirectTo: redirectUrl() },
  });

  if (error) return { status: 'error', message: error.message };
  return { status: 'success' };
}

/**
 * Returns to the origin the app is being served from, so a local dev server and
 * the deployed site both work without a build-time setting. Every origin used
 * still has to be allow-listed in Supabase (docs/19-google-authentication.md).
 */
function redirectUrl(): string | undefined {
  if (typeof window === 'undefined') return undefined;
  return `${window.location.origin}${GOOGLE_REDIRECT_PATH}`;
}
