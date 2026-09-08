import { fireEvent, render, screen } from '@testing-library/react-native';
import type { ReactNode } from 'react';
import { SafeAreaProvider } from 'react-native-safe-area-context';

import AppearanceScreen from '../(tabs)/settings/appearance';
import { ThemeProvider } from '@/theme';

/**
 * Dark mode worked before this screen, but only by changing the phone's own
 * settings — which is not where anybody looks for an app's appearance.
 */

const setPreference = jest.fn();
const mockAppearance = jest.fn();

jest.mock('@/theme', () => {
  const actual = jest.requireActual('@/theme');
  return { ...actual, useAppearance: () => mockAppearance() };
});

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
  return render(<AppearanceScreen />, { wrapper: Wrapper });
}

beforeEach(() => {
  setPreference.mockClear();
  mockAppearance.mockReturnValue({ preference: 'system', setPreference });
});

it('offers every appearance, phone-matching included', () => {
  renderScreen();

  expect(screen.getByRole('radio', { name: 'Match my phone' })).toBeTruthy();
  expect(screen.getByRole('radio', { name: 'Light' })).toBeTruthy();
  expect(screen.getByRole('radio', { name: 'Dark' })).toBeTruthy();
});

/** Which one is on has to be announced, not just drawn as a tick. */
it('marks the current choice as selected', () => {
  mockAppearance.mockReturnValue({ preference: 'dark', setPreference });
  renderScreen();

  expect(screen.getByRole('radio', { name: 'Dark' }).props.accessibilityState.checked).toBe(true);
  expect(screen.getByRole('radio', { name: 'Light' }).props.accessibilityState.checked).toBe(false);
});

it('changes the appearance when one is chosen', () => {
  renderScreen();

  fireEvent.press(screen.getByRole('radio', { name: 'Dark' }));

  expect(setPreference).toHaveBeenCalledWith('dark');
});
