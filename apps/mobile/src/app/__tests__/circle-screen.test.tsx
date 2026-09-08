import { render, screen } from '@testing-library/react-native';
import type { ReactNode } from 'react';
import { SafeAreaProvider } from 'react-native-safe-area-context';

import { ThemeProvider } from '@/theme';

import CircleScreen from '../(tabs)/circle';

const mockSeniors = jest.fn();

jest.mock('@/features/seniors/use-seniors', () => ({ useSeniors: () => mockSeniors() }));
jest.mock('@/features/auth/session-provider', () => ({
  useSession: () => ({ isSignedIn: true, isRestoring: false }),
}));
jest.mock('expo-router', () => ({ router: { push: jest.fn() }, Stack: { Screen: () => null } }));

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

  return render(<CircleScreen />, { wrapper: Wrapper });
}

it('lists everyone you care for, by your relationship to them', () => {
  mockSeniors.mockReturnValue({
    data: [
      { id: '1', displayName: 'James Miller', isSelf: false, role: 'family_member' },
      { id: '2', displayName: 'Ada Lovelace', isSelf: true, role: 'family_member' },
    ],
    isPending: false,
    isError: false,
    refetch: jest.fn(),
  });
  renderScreen();

  expect(screen.getByRole('button', { name: 'James Miller, Family care' })).toBeTruthy();
  expect(screen.getByRole('button', { name: 'Ada Lovelace, Your own care' })).toBeTruthy();
});

/**
 * An invited caregiver arrives with nobody, and so does a brand-new user. Both
 * need the two ways in, which is why they sit on this screen rather than being
 * buried behind an empty list.
 */
it('offers both ways to grow an empty circle', () => {
  mockSeniors.mockReturnValue({ data: [], isPending: false, isError: false, refetch: jest.fn() });
  renderScreen();

  expect(screen.getByRole('button', { name: 'Add another person' })).toBeTruthy();
  expect(screen.getByRole('button', { name: 'Join a care circle' })).toBeTruthy();
});
