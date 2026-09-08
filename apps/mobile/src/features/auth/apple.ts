import * as Linking from 'expo-linking';
import * as WebBrowser from 'expo-web-browser';

import { supabase } from '@/lib/supabase';

import { APPLE_REDIRECT_PATH, type AppleSignInResult } from './apple-result';

/** Apple OAuth through Supabase; secrets remain in the provider dashboards. */
export async function signInWithApple(): Promise<AppleSignInResult> {
  const redirectTo = Linking.createURL(APPLE_REDIRECT_PATH);
  const { data, error } = await supabase.auth.signInWithOAuth({
    provider: 'apple',
    options: { redirectTo, skipBrowserRedirect: true },
  });
  if (error) return { status: 'error', message: error.message };
  if (!data?.url) return { status: 'error', message: 'Apple sign-in is not available right now.' };

  const result = await WebBrowser.openAuthSessionAsync(data.url, redirectTo);
  if (result.type !== 'success') return { status: 'cancelled' };

  const params = callbackParams(result.url);
  if (params.get('error') === 'access_denied') return { status: 'cancelled' };
  if (params.get('error')) {
    return { status: 'error', message: params.get('error_description') ?? 'Apple sign-in failed.' };
  }
  const code = params.get('code');
  if (!code)
    return { status: 'error', message: 'Apple sign-in did not complete. Please try again.' };

  const { error: exchangeError } = await supabase.auth.exchangeCodeForSession(code);
  if (exchangeError) return { status: 'error', message: exchangeError.message };
  return { status: 'success' };
}

function callbackParams(url: string): URLSearchParams {
  const start = url.search(/[?#]/);
  if (start === -1) return new URLSearchParams();
  return new URLSearchParams(url.slice(start + 1).replace('#', '&'));
}
