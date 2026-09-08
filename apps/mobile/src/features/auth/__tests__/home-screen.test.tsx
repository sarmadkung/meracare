import { render, screen } from '@testing-library/react-native';
import type { ReactNode } from 'react';
import { SafeAreaProvider } from 'react-native-safe-area-context';

import HomeScreen from '@/app/home';
import { ThemeProvider } from '@/theme';

/**
 * Home is the only screen with a sign-out button, and sign-out is the one
 * action here that can refuse. A refusal it does not show is indistinguishable
 * from a dead button — the person taps, nothing moves, and there is nothing on
 * the screen to act on.
 */

const mockAuthActions = jest.fn();

jest.mock('@/features/auth/session-provider', () => ({
  useSession: () => ({ isSignedIn: true, isRestoring: false }),
}));

jest.mock('@/features/auth/use-auth-actions', () => ({
  useAuthActions: () => mockAuthActions(),
}));

jest.mock('@/features/seniors/use-seniors', () => ({
  useSeniors: () => ({ data: [], isPending: false, isError: false, refetch: jest.fn() }),
}));

jest.mock('@/features/tasks/use-tasks', () => ({ useMyTasks: () => ({ data: [] }) }));
jest.mock('@/features/notifications/use-notifications', () => ({ useUnreadCount: () => 0 }));
jest.mock('@/features/sync/use-sync', () => ({ useOfflineSync: jest.fn() }));

jest.mock('expo-router', () => ({
  Link: ({ children }: { children: ReactNode }) => children,
  Redirect: () => null,
  router: { push: jest.fn() },
}));

function actions(overrides: Record<string, unknown> = {}) {
  return {
    signOut: jest.fn(),
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

  return render(<HomeScreen />, { wrapper: Wrapper });
}

it('shows why sign-out was refused', () => {
  mockAuthActions.mockReturnValue(
    actions({ error: 'Some offline care updates still need attention.' }),
  );
  renderScreen();

  expect(screen.getByText('Some offline care updates still need attention.')).toBeTruthy();
});

it('says nothing when there is nothing wrong', () => {
  mockAuthActions.mockReturnValue(actions());
  renderScreen();

  expect(screen.queryByRole('alert')).toBeNull();
});
