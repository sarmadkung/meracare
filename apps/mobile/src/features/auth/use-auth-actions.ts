import { useState } from 'react';

import { supabase } from '@/lib/supabase';

import { signInWithGoogle as startGoogleSignIn } from './google';

/** Which action is in flight, so only that control shows a spinner. */
export type AuthAction = 'email' | 'google';

/**
 * Email sign-in/sign-up, Google sign-in, and sign-out.
 *
 * Screens call these instead of touching `supabase.auth` directly, so the rest
 * of the app never has to know which provider signed the person in — the
 * session that comes out is the same either way (plans/phase10.md §8).
 *
 * Apple is a documented launch provider (docs/12-tech-stack.md) and slots in
 * beside Google here when its Supabase provider is configured.
 */
export function useAuthActions() {
  const [pending, setPending] = useState<AuthAction | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function run<T>(action: AuthAction, work: () => Promise<T>, fallback: T): Promise<T> {
    setPending(action);
    setError(null);
    try {
      return await work();
    } catch (cause) {
      // Never surface a raw provider or network error: it can carry URLs and
      // token fragments (docs/09-security-privacy.md).
      setError(messageFor(cause));
      return fallback;
    } finally {
      setPending(null);
    }
  }

  async function runEmail(
    action: () => Promise<{ error: { message: string } | null }>,
  ): Promise<boolean> {
    return run(
      'email',
      async () => {
        const result = await action();
        if (result.error) {
          setError(result.error.message);
          return false;
        }
        return true;
      },
      false,
    );
  }

  return {
    pending,
    isSubmitting: pending !== null,
    error,
    clearError: () => setError(null),

    signIn: (email: string, password: string) =>
      runEmail(() => supabase.auth.signInWithPassword({ email: email.trim(), password })),

    signUp: (email: string, password: string) =>
      runEmail(() => supabase.auth.signUp({ email: email.trim(), password })),

    /**
     * Returns false both when Google sign-in fails and when the person backs
     * out; only a failure sets an error message.
     */
    signInWithGoogle: () =>
      run(
        'google',
        async () => {
          const result = await startGoogleSignIn();
          if (result.status === 'error') {
            setError(result.message);
            return false;
          }
          return result.status === 'success';
        },
        false,
      ),

    signOut: () => runEmail(() => supabase.auth.signOut()),
  };
}

function messageFor(cause: unknown): string {
  if (cause instanceof TypeError) {
    return 'Could not reach the network. Check your connection and try again.';
  }
  return 'Something went wrong. Please try again.';
}
