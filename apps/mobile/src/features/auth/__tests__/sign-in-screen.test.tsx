import { fireEvent, render, screen } from '@testing-library/react-native';
import type { ReactNode } from 'react';
import { SafeAreaProvider } from 'react-native-safe-area-context';

import SignInScreen from '@/app/sign-in';
import { ThemeProvider } from '@/theme';

/**
 * The sign-in screen must offer Google beside email without letting one tap
 * start two flows (plans/phase10.md §§21–23).
 */

const mockAuthActions = jest.fn();

jest.mock('@/features/auth/session-provider', () => ({
  useSession: () => ({ isSignedIn: false, isRestoring: false }),
}));

jest.mock('@/features/auth/use-auth-actions', () => ({
  useAuthActions: () => mockAuthActions(),
}));

jest.mock('expo-router', () => ({
  Redirect: () => null,
}));

function actions(overrides: Record<string, unknown> = {}) {
  return {
    signIn: jest.fn(),
    signUp: jest.fn(),
    signInWithGoogle: jest.fn(),
    pending: null,
    isSubmitting: false,
    error: null,
    clearError: jest.fn(),
    ...overrides,
  };
}

function renderScreen() {
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <SafeAreaProvider
        initialMetrics={{
          frame: { x: 0, y: 0, width: 390, height: 844 },
          insets: { top: 47, left: 0, right: 0, bottom: 34 },
        }}
      >
        <ThemeProvider>{children}</ThemeProvider>
      </SafeAreaProvider>
    );
  }

  return render(<SignInScreen />, { wrapper: Wrapper });
}

it('offers Continue with Google', () => {
  mockAuthActions.mockReturnValue(actions());
  renderScreen();

  expect(screen.getByLabelText('Continue with Google')).toBeTruthy();
});

it('starts Google sign-in when pressed', () => {
  const signInWithGoogle = jest.fn().mockResolvedValue(true);
  mockAuthActions.mockReturnValue(actions({ signInWithGoogle }));
  renderScreen();

  fireEvent.press(screen.getByLabelText('Continue with Google'));

  expect(signInWithGoogle).toHaveBeenCalledTimes(1);
});

it('disables the button while Google sign-in is in flight', () => {
  const signInWithGoogle = jest.fn();
  mockAuthActions.mockReturnValue(
    actions({ signInWithGoogle, pending: 'google', isSubmitting: true }),
  );
  renderScreen();

  const button = screen.getByLabelText('Continue with Google');
  expect(button.props.accessibilityState).toMatchObject({ disabled: true, busy: true });

  fireEvent.press(button);
  expect(signInWithGoogle).not.toHaveBeenCalled();
});

it('disables Google while an email sign-in is in flight', () => {
  mockAuthActions.mockReturnValue(actions({ pending: 'email', isSubmitting: true }));
  renderScreen();

  expect(screen.getByLabelText('Continue with Google').props.accessibilityState).toMatchObject({
    disabled: true,
    busy: false,
  });
});

it('shows a Google failure on the screen', () => {
  mockAuthActions.mockReturnValue(actions({ error: 'Provider is not enabled' }));
  renderScreen();

  expect(screen.getByText('Provider is not enabled')).toBeTruthy();
});
