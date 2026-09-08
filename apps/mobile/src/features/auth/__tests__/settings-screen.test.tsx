import { render, screen } from '@testing-library/react-native';
import type { ReactNode } from 'react';
import { SafeAreaProvider } from 'react-native-safe-area-context';

import SettingsScreen from '@/app/(tabs)/settings/index';
import { ThemeProvider } from '@/theme';

/**
 * Settings owns the only action in the app that can refuse. A refusal it does
 * not show is indistinguishable from a dead button — the person taps, nothing
 * moves, and there is nothing on screen to act on.
 *
 * This coverage moved here from Home along with the button itself, which spent
 * its life stranded mid-screen because docs/13's Settings screens were
 * specified and never built.
 */

const mockAuthActions = jest.fn();

jest.mock('@/features/auth/use-auth-actions', () => ({
  useAuthActions: () => mockAuthActions(),
}));

jest.mock('expo-router', () => ({
  router: { push: jest.fn() },
  Stack: { Screen: () => null },
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

  return render(<SettingsScreen />, { wrapper: Wrapper });
}

it('offers sign out', () => {
  mockAuthActions.mockReturnValue(actions());
  renderScreen();

  expect(screen.getByRole('button', { name: 'Sign out' })).toBeTruthy();
});

it('leads to profile and notification settings', () => {
  mockAuthActions.mockReturnValue(actions());
  renderScreen();

  expect(screen.getByRole('button', { name: 'Profile, Your name and details' })).toBeTruthy();
  expect(screen.getByRole('button', { name: 'Notifications, Reminders and alerts' })).toBeTruthy();
});

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
