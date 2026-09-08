import { supabase } from '@/lib/supabase';

import { APPLE_REDIRECT_PATH, type AppleSignInResult } from './apple-result';

export async function signInWithApple(): Promise<AppleSignInResult> {
  const redirectTo =
    typeof window === 'undefined' ? undefined : `${window.location.origin}${APPLE_REDIRECT_PATH}`;
  const { error } = await supabase.auth.signInWithOAuth({
    provider: 'apple',
    options: { redirectTo },
  });
  return error ? { status: 'error', message: error.message } : { status: 'success' };
}
