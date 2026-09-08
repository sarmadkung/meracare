import { fireEvent, render, screen } from '@testing-library/react-native';
import type { ReactNode } from 'react';
import { SafeAreaProvider } from 'react-native-safe-area-context';

import ProfileScreen from '@/app/(tabs)/settings/profile';
import { ThemeProvider } from '@/theme';

/**
 * Profile shipped showing an email address and nothing else, while `/v1/me`
 * had carried a display name and phone all along — and a mutation to change
 * them that no screen had ever called.
 */

const mockMe = jest.fn();
const mockMutate = jest.fn();

jest.mock('@/features/profile/use-me', () => ({
  useMe: () => mockMe(),
  useUpdateMe: () => ({ mutate: mockMutate, isPending: false, isError: false }),
}));

jest.mock('@/features/auth/session-provider', () => ({
  useSession: () => ({ session: { user: { email: 'ada@example.com' } } }),
}));

jest.mock('expo-router', () => ({ Stack: { Screen: () => null } }));

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
  return render(<ProfileScreen />, { wrapper: Wrapper });
}

const me = {
  id: 'user-1',
  displayName: 'Ada Lovelace',
  avatarUrl: null,
  phone: '+92 300 1234567',
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
};

beforeEach(() => {
  mockMutate.mockClear();
  mockMe.mockReturnValue({ data: me, isPending: false, isError: false });
});

it('shows who you are, not just how you signed in', () => {
  renderScreen();

  expect(screen.getByText('Ada Lovelace')).toBeTruthy();
  expect(screen.getByText('+92 300 1234567')).toBeTruthy();
});

/** The email belongs to the sign-in identity, not to the profile record. */
it('shows the sign-in email as something you cannot edit here', () => {
  renderScreen();

  expect(screen.getByText('ada@example.com')).toBeTruthy();
});

it('saves a new name', () => {
  renderScreen();

  fireEvent.press(screen.getByRole('button', { name: 'Edit profile' }));
  fireEvent.changeText(screen.getByLabelText('Your name'), 'Ada King');
  fireEvent.press(screen.getByRole('button', { name: 'Save' }));

  expect(mockMutate).toHaveBeenCalledWith(
    expect.objectContaining({ displayName: 'Ada King' }),
    expect.anything(),
  );
});

/** A name is how everyone else in the circle sees you, so it cannot be blank. */
it('refuses to save an empty name', () => {
  renderScreen();

  fireEvent.press(screen.getByRole('button', { name: 'Edit profile' }));
  fireEvent.changeText(screen.getByLabelText('Your name'), '   ');
  fireEvent.press(screen.getByRole('button', { name: 'Save' }));

  expect(mockMutate).not.toHaveBeenCalled();
  expect(screen.getByText('Please enter your name.')).toBeTruthy();
});

/** Clearing a phone number is a real intent, not an empty field to reject. */
it('lets a phone number be removed', () => {
  renderScreen();

  fireEvent.press(screen.getByRole('button', { name: 'Edit profile' }));
  fireEvent.changeText(screen.getByLabelText('Phone number'), '');
  fireEvent.press(screen.getByRole('button', { name: 'Save' }));

  expect(mockMutate).toHaveBeenCalledWith(
    expect.objectContaining({ phone: null }),
    expect.anything(),
  );
});
