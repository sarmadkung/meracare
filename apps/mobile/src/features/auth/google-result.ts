/**
 * The result shape both platform implementations of `signInWithGoogle` return.
 *
 * Cancellation is a distinct outcome from failure: backing out of the Google
 * screen must return the person to sign-in without an error message.
 */
export type GoogleSignInResult =
  { status: 'success' } | { status: 'cancelled' } | { status: 'error'; message: string };

/**
 * Path the OAuth callback returns to. On native it becomes
 * `meracare://auth/callback`; on web it is appended to the current origin.
 */
export const GOOGLE_REDIRECT_PATH = '/auth/callback';
