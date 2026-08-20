import * as Linking from 'expo-linking';
import * as WebBrowser from 'expo-web-browser';

import { supabase } from '@/lib/supabase';

import { signInWithGoogle } from '../google';

jest.mock('@/lib/supabase', () => ({
  supabase: {
    auth: {
      signInWithOAuth: jest.fn(),
      exchangeCodeForSession: jest.fn(),
    },
  },
}));

jest.mock('expo-web-browser', () => ({ openAuthSessionAsync: jest.fn() }));

// `createURL` reads the Expo manifest, which does not exist under Jest. The
// scheme it would produce is `meracare` (app.json), so it is stubbed; `parse`
// stays real, because reading the callback back is what these tests check.
jest.mock('expo-linking', () => ({
  ...jest.requireActual('expo-linking'),
  createURL: (path: string) => `meracare://${path.replace(/^\//, '')}`,
}));

const signInWithOAuth = supabase.auth.signInWithOAuth as jest.Mock;
const exchangeCodeForSession = supabase.auth.exchangeCodeForSession as jest.Mock;
const openAuthSessionAsync = WebBrowser.openAuthSessionAsync as jest.Mock;

const AUTH_URL = 'https://project.supabase.co/auth/v1/authorize?provider=google';

beforeEach(() => {
  jest.clearAllMocks();
  signInWithOAuth.mockResolvedValue({ data: { url: AUTH_URL }, error: null });
  exchangeCodeForSession.mockResolvedValue({ error: null });
});

/** The redirect the app registers, so assertions can match what it receives. */
function redirectUri(): string {
  return Linking.createURL('/auth/callback');
}

describe('signInWithGoogle (native)', () => {
  it('exchanges the returned code for a session', async () => {
    openAuthSessionAsync.mockResolvedValue({
      type: 'success',
      url: `${redirectUri()}?code=auth-code-123`,
    });

    await expect(signInWithGoogle()).resolves.toEqual({ status: 'success' });

    expect(signInWithOAuth).toHaveBeenCalledWith({
      provider: 'google',
      options: { redirectTo: redirectUri(), skipBrowserRedirect: true },
    });
    expect(openAuthSessionAsync).toHaveBeenCalledWith(AUTH_URL, redirectUri());
    expect(exchangeCodeForSession).toHaveBeenCalledWith('auth-code-123');
  });

  it('reports dismissing the browser as a cancellation, not an error', async () => {
    openAuthSessionAsync.mockResolvedValue({ type: 'dismiss' });

    await expect(signInWithGoogle()).resolves.toEqual({ status: 'cancelled' });
    expect(exchangeCodeForSession).not.toHaveBeenCalled();
  });

  it('reports a declined consent screen as a cancellation', async () => {
    openAuthSessionAsync.mockResolvedValue({
      type: 'success',
      url: `${redirectUri()}?error=access_denied&error_description=The+user+declined`,
    });

    await expect(signInWithGoogle()).resolves.toEqual({ status: 'cancelled' });
    expect(exchangeCodeForSession).not.toHaveBeenCalled();
  });

  it('surfaces a provider error from the callback', async () => {
    openAuthSessionAsync.mockResolvedValue({
      type: 'success',
      url: `${redirectUri()}?error=server_error&error_description=Provider+is+not+enabled`,
    });

    await expect(signInWithGoogle()).resolves.toEqual({
      status: 'error',
      message: 'Provider is not enabled',
    });
  });

  it('fails when the callback carries neither a code nor an error', async () => {
    openAuthSessionAsync.mockResolvedValue({ type: 'success', url: redirectUri() });

    const result = await signInWithGoogle();

    expect(result.status).toBe('error');
    expect(exchangeCodeForSession).not.toHaveBeenCalled();
  });

  it('surfaces a failure to start the flow', async () => {
    signInWithOAuth.mockResolvedValue({ data: null, error: { message: 'Unsupported provider' } });

    await expect(signInWithGoogle()).resolves.toEqual({
      status: 'error',
      message: 'Unsupported provider',
    });
    expect(openAuthSessionAsync).not.toHaveBeenCalled();
  });

  it('surfaces a failed code exchange', async () => {
    openAuthSessionAsync.mockResolvedValue({
      type: 'success',
      url: `${redirectUri()}?code=auth-code-123`,
    });
    exchangeCodeForSession.mockResolvedValue({ error: { message: 'Invalid code verifier' } });

    await expect(signInWithGoogle()).resolves.toEqual({
      status: 'error',
      message: 'Invalid code verifier',
    });
  });
});
