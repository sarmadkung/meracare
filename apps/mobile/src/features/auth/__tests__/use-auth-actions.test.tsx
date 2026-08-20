import { act, renderHook, waitFor } from '@testing-library/react-native';

import { signInWithGoogle as startGoogleSignIn } from '../google';
import { useAuthActions } from '../use-auth-actions';

jest.mock('@/lib/supabase', () => ({
  supabase: {
    auth: {
      signInWithPassword: jest.fn().mockResolvedValue({ error: null }),
      signUp: jest.fn().mockResolvedValue({ error: null }),
      signOut: jest.fn().mockResolvedValue({ error: null }),
    },
  },
}));

jest.mock('../google', () => ({ signInWithGoogle: jest.fn() }));

const googleSignIn = startGoogleSignIn as jest.Mock;

beforeEach(() => jest.clearAllMocks());

describe('useAuthActions — Google', () => {
  it('reports success and leaves no error behind', async () => {
    googleSignIn.mockResolvedValue({ status: 'success' });
    const { result } = renderHook(() => useAuthActions());

    let succeeded: boolean | undefined;
    await act(async () => {
      succeeded = await result.current.signInWithGoogle();
    });

    expect(succeeded).toBe(true);
    expect(result.current.error).toBeNull();
    expect(result.current.isSubmitting).toBe(false);
  });

  it('treats cancellation as a non-event: no error, no session', async () => {
    googleSignIn.mockResolvedValue({ status: 'cancelled' });
    const { result } = renderHook(() => useAuthActions());

    let succeeded: boolean | undefined;
    await act(async () => {
      succeeded = await result.current.signInWithGoogle();
    });

    expect(succeeded).toBe(false);
    expect(result.current.error).toBeNull();
  });

  it('surfaces a failure message', async () => {
    googleSignIn.mockResolvedValue({ status: 'error', message: 'Provider is not enabled' });
    const { result } = renderHook(() => useAuthActions());

    await act(async () => {
      await result.current.signInWithGoogle();
    });

    expect(result.current.error).toBe('Provider is not enabled');
  });

  it('replaces an unexpected throw with a message that leaks nothing', async () => {
    googleSignIn.mockRejectedValue(new Error('https://project.supabase.co?code=secret'));
    const { result } = renderHook(() => useAuthActions());

    await act(async () => {
      await result.current.signInWithGoogle();
    });

    expect(result.current.error).toBe('Something went wrong. Please try again.');
    expect(result.current.isSubmitting).toBe(false);
  });

  it('marks only Google as pending while the flow runs, so one tap is one flow', async () => {
    let release: (value: { status: 'cancelled' }) => void = () => {};
    googleSignIn.mockReturnValue(
      new Promise<{ status: 'cancelled' }>((resolve) => {
        release = resolve;
      }),
    );
    const { result } = renderHook(() => useAuthActions());

    let pendingCall: Promise<boolean>;
    act(() => {
      pendingCall = result.current.signInWithGoogle();
    });

    await waitFor(() => expect(result.current.pending).toBe('google'));
    expect(result.current.isSubmitting).toBe(true);

    await act(async () => {
      release({ status: 'cancelled' });
      await pendingCall;
    });

    expect(result.current.pending).toBeNull();
    expect(googleSignIn).toHaveBeenCalledTimes(1);
  });
});
