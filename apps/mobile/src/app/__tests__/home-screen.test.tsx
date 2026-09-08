import { render, screen } from '@testing-library/react-native';
import type { ReactNode } from 'react';
import { SafeAreaProvider } from 'react-native-safe-area-context';

import { ThemeProvider } from '@/theme';

import HomeScreen from '../(tabs)/home';

const mockTasks = jest.fn();

jest.mock('@/features/auth/session-provider', () => ({
  useSession: () => ({ isSignedIn: true, isRestoring: false }),
}));
jest.mock('@/features/seniors/use-seniors', () => ({
  useSeniors: () => ({ data: [], isPending: false, isError: false, refetch: jest.fn() }),
}));
jest.mock('@/features/tasks/use-tasks', () => ({ useMyTasks: () => mockTasks() }));
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

/** "Today" promised a day and showed a menu of buttons. It has to say which day. */
it("names today's date", () => {
  mockTasks.mockReturnValue({ data: [] });
  renderScreen();

  const today = new Date().toLocaleDateString(undefined, {
    weekday: 'long',
    day: 'numeric',
    month: 'long',
  });

  expect(screen.getByText(today)).toBeTruthy();
});

it('lists your own round', () => {
  mockTasks.mockReturnValue({
    data: [
      {
        id: 't1',
        seniorId: '1',
        title: 'Morning walk',
        status: 'pending',
        scheduledFor: '2026-09-08T09:00:00.000Z',
      },
    ],
  });
  renderScreen();

  expect(screen.getByText('Morning walk')).toBeTruthy();
});

/** Nothing to do is worth saying plainly, rather than showing an empty screen. */
it('says so when nothing needs you', () => {
  mockTasks.mockReturnValue({ data: [] });
  renderScreen();

  expect(screen.getByText('Nothing needs you')).toBeTruthy();
});

/** Sign-out moved to Settings and must not drift back onto Home. */
it('does not offer sign out', () => {
  mockTasks.mockReturnValue({ data: [] });
  renderScreen();

  expect(screen.queryByRole('button', { name: 'Sign out' })).toBeNull();
});
