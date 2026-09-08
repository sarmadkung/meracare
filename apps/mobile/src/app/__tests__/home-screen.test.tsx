import { render, screen } from '@testing-library/react-native';
import type { ReactNode } from 'react';
import { SafeAreaProvider } from 'react-native-safe-area-context';

import { ThemeProvider } from '@/theme';

import HomeScreen from '../(tabs)/home';

const mockToday = jest.fn();

jest.mock('@/features/auth/session-provider', () => ({
  useSession: () => ({ isSignedIn: true, isRestoring: false }),
}));
jest.mock('@/features/today/use-today', () => ({ useToday: () => mockToday() }));
jest.mock('@/features/sync/use-sync', () => ({ useOfflineSync: jest.fn() }));
jest.mock('expo-router', () => ({
  router: { push: jest.fn() },
  Redirect: () => null,
  Stack: { Screen: () => null },
}));

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

/** "Today" promised a day and showed a menu. It has to say which day it is. */
it("names today's date", () => {
  mockToday.mockReturnValue({ data: [] });
  renderScreen();

  const today = new Date().toLocaleDateString(undefined, {
    weekday: 'long',
    day: 'numeric',
    month: 'long',
  });

  expect(screen.getByText(today)).toBeTruthy();
});

function dose(overrides = {}) {
  return {
    kind: 'dose',
    id: 'dose-1',
    seniorId: 'senior-1',
    seniorName: 'James',
    timezone: 'UTC',
    title: 'Metformin',
    detail: '500 mg',
    scheduledFor: '2026-09-08T09:00:00.000Z',
    status: 'pending',
    assignedToMe: false,
    ...overrides,
  };
}

/**
 * The bug this replaced: Today read only tasks assigned to the reader, so a
 * family member caring alone — who assigns nothing to themselves — saw
 * "Nothing needs you" while their mother had four doses due.
 */
it('shows care that nobody has been assigned', () => {
  mockToday.mockReturnValue({ data: [dose()] });
  renderScreen();

  expect(screen.getByText('Metformin')).toBeTruthy();
});

it("leads with the reader's own work when there is some", () => {
  mockToday.mockReturnValue({
    data: [dose({ id: 'a', title: 'Morning walk', kind: 'task', assignedToMe: true }), dose()],
  });
  renderScreen();

  expect(screen.getByRole('header', { name: 'Yours to do' })).toBeTruthy();
  expect(screen.getByText('Morning walk')).toBeTruthy();
  expect(screen.getByText('Metformin')).toBeTruthy();
});

/** With nothing assigned there is no point in a heading that separates nothing. */
it('shows no assignment heading when nothing is assigned', () => {
  mockToday.mockReturnValue({ data: [dose()] });
  renderScreen();

  expect(screen.queryByRole('header', { name: 'Yours to do' })).toBeNull();
});

/** An empty day is worth saying plainly rather than showing a blank screen. */
it('says so when the day really is clear', () => {
  mockToday.mockReturnValue({ data: [] });
  renderScreen();

  expect(screen.getByText('Nothing needs you')).toBeTruthy();
});

/** Sign-out moved to Settings and must not drift back onto Home. */
it('does not offer sign out', () => {
  mockToday.mockReturnValue({ data: [] });
  renderScreen();

  expect(screen.queryByRole('button', { name: 'Sign out' })).toBeNull();
});
