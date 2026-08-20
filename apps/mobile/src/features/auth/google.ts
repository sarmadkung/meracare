import * as Linking from 'expo-linking';
import * as WebBrowser from 'expo-web-browser';

import { supabase } from '@/lib/supabase';

import { GOOGLE_REDIRECT_PATH, type GoogleSignInResult } from './google-result';

/**
 * Google sign-in on iOS and Android.
 *
 * Supabase performs the OAuth exchange, so the Google client secret stays in the
 * Supabase project and never ships inside the app (docs/09-security-privacy.md).
 * The app opens Google in an in-app browser session, receives the authorization
 * code on the `meracare://` deep link, and trades it for a Supabase session
 * using the PKCE verifier the client stored when the flow began.
 *
 * The web build resolves `google.web.ts` instead.
 */
export async function signInWithGoogle(): Promise<GoogleSignInResult> {
  const redirectTo = Linking.createURL(GOOGLE_REDIRECT_PATH);

  const { data, error } = await supabase.auth.signInWithOAuth({
    provider: 'google',
    options: {
      redirectTo,
      // The app owns the browser session so it can read the callback URL back;
      // Supabase must hand over the authorization URL instead of navigating.
      skipBrowserRedirect: true,
    },
  });

  if (error) return { status: 'error', message: error.message };
  if (!data?.url) {
    return { status: 'error', message: 'Google sign-in is not available right now.' };
  }

  const result = await WebBrowser.openAuthSessionAsync(data.url, redirectTo);

  // Anything that is not a completed redirect means the person backed out or
  // dismissed the sheet. That is a normal outcome, not a failure to report.
  if (result.type !== 'success') return { status: 'cancelled' };

  const params = callbackParams(result.url);
  const providerError = params.get('error');

  if (providerError) {
    // `access_denied` is what Google sends when the person declines consent.
    if (providerError === 'access_denied') return { status: 'cancelled' };
    return {
      status: 'error',
      message: params.get('error_description') ?? 'Google sign-in failed.',
    };
  }

  const code = params.get('code');
  if (!code) {
    return { status: 'error', message: 'Google sign-in did not complete. Please try again.' };
  }

  const { error: exchangeError } = await supabase.auth.exchangeCodeForSession(code);
  if (exchangeError) return { status: 'error', message: exchangeError.message };

  return { status: 'success' };
}

/**
 * Reads the parameters off the callback deep link.
 *
 * The query string is parsed directly rather than through `Linking.parse` or
 * `URL`, neither of which handles a custom-scheme URL consistently across
 * platforms. Supabase returns errors in the fragment on some paths, so both
 * halves are read.
 */
function callbackParams(url: string): Map<string, string> {
  const params = new Map<string, string>();

  const start = url.search(/[?#]/);
  if (start === -1) return params;

  // Splitting on all three separators reads the query and the fragment in one
  // pass, whichever order Supabase used.
  for (const pair of url.slice(start + 1).split(/[&#?]/)) {
    const equals = pair.indexOf('=');
    if (equals <= 0) continue;

    const key = decodeURIComponent(pair.slice(0, equals));
    const value = decodeURIComponent(pair.slice(equals + 1).replace(/\+/g, ' '));
    if (value !== '' && !params.has(key)) params.set(key, value);
  }

  return params;
}
