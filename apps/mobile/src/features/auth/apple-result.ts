export type AppleSignInResult =
  { status: 'success' } | { status: 'cancelled' } | { status: 'error'; message: string };

export const APPLE_REDIRECT_PATH = '/auth/callback';
