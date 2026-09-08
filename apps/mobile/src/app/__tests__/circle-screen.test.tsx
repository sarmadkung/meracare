import { render, screen } from '@testing-library/react-native';
import type { ReactNode } from 'react';
import { SafeAreaProvider } from 'react-native-safe-area-context';

import { ThemeProvider } from '@/theme';

import CircleScreen from '../(tabs)/circle';

const mockSeniors = jest.fn();
const mockSummaries = jest.fn();

jest.mock('@/features/seniors/use-seniors', () => ({ useSeniors: () => mockSeniors() }));
jest.mock('@/features/seniors/use-senior-summaries', () => ({
  useSeniorSummaries: () => mockSummaries(),
}));
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

beforeEach(() => {
  mockSummaries.mockReturnValue({ data: undefined });
});

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

/**
 * The card exists to answer "does anyone need me right now", which a list of
 * names cannot. These are the counts, read off the summary endpoint.
 */
it("shows each person's day on their card", () => {
  mockSeniors.mockReturnValue({
    data: [{ id: '1', displayName: 'James Miller', isSelf: false, role: 'family_member' }],
    isPending: false,
    isError: false,
    refetch: jest.fn(),
  });
  mockSummaries.mockReturnValue({
    data: [
      {
        seniorId: '1',
        medications: { done: 3, total: 4 },
        tasks: { done: 2, total: 4 },
        needsAttention: 1,
      },
    ],
  });
  renderScreen();

  expect(screen.getByLabelText('3/4 Meds')).toBeTruthy();
  expect(screen.getByLabelText('2 Tasks')).toBeTruthy();
  expect(screen.getByLabelText('1 Due')).toBeTruthy();
});

/** Names arrive before counts, and a card with no strip is the honest interim. */
it('shows the person before their counts arrive', () => {
  mockSeniors.mockReturnValue({
    data: [{ id: '1', displayName: 'James Miller', isSelf: false, role: 'family_member' }],
    isPending: false,
    isError: false,
    refetch: jest.fn(),
  });
  mockSummaries.mockReturnValue({ data: undefined });
  renderScreen();

  expect(screen.getByRole('button', { name: 'James Miller, Family care' })).toBeTruthy();
  expect(screen.queryByLabelText(/Meds$/)).toBeNull();
});
