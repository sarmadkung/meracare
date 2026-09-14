import { render, screen } from '@testing-library/react-native';
import type { ReactNode } from 'react';

import { ThemeProvider } from '@/theme';

import TabsLayout from '../(tabs)/_layout';

const mockSession = jest.fn();

jest.mock('@/features/auth/session-provider', () => ({ useSession: () => mockSession() }));
jest.mock('@/features/notifications/use-notifications', () => ({ useUnreadCount: () => 3 }));

// Stands in for the real redirect so a test can see where it points without
// mounting a navigator.
jest.mock('expo-router', () => {
  // Required inside the factory: jest hoists this above the imports, so a
  // top-level import of react-native would not exist yet.
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  const { Text } = require('react-native');

  function Redirect({ href }: { href: string }) {
    return <Text>{`redirected to ${href}`}</Text>;
  }

  return { Redirect };
});

// Render the tab configuration rather than a real navigator: this test is about
// which destinations exist and what they are called, not about react-navigation.
jest.mock('expo-router/js-tabs', () => {
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  const { Text, View } = require('react-native');

  function Tabs({ children }: { children: ReactNode }) {
    return <View>{children}</View>;
  }

  function TabScreen({ options }: { options: { title: string; tabBarBadge?: number } }) {
    const { title, tabBarBadge } = options;
    return <Text>{tabBarBadge === undefined ? title : `${title} ${tabBarBadge}`}</Text>;
  }

  Tabs.Screen = TabScreen;

  return { Tabs };
});

function renderTabs() {
  return render(
    <ThemeProvider>
      <TabsLayout />
    </ThemeProvider>,
  );
}

beforeEach(() => {
  mockSession.mockReturnValue({ isSignedIn: true, isRestoring: false });
});

it('offers the four top-level destinations', () => {
  renderTabs();

  expect(screen.getByText('Today')).toBeTruthy();
  expect(screen.getByText('Circle')).toBeTruthy();
  expect(screen.getByText('Settings')).toBeTruthy();
});

/**
 * The unread count used to sit on a button inside Home, which meant you had to
 * already be on Home to learn that anything needed you.
 */
it('carries the unread count on Alerts', () => {
  renderTabs();

  expect(screen.getByText('Alerts 3')).toBeTruthy();
});

/**
 * Signing out happens on Settings, and Settings had no guard of its own — so
 * the account went away while the screen stayed, leaving a signed-out person
 * looking at their own settings. The guard belongs to the layout every tab
 * shares, not to whichever screens remembered it.
 */
it('leaves for sign-in once the session is gone', () => {
  mockSession.mockReturnValue({ isSignedIn: false, isRestoring: false });
  renderTabs();

  expect(screen.getByText('redirected to /sign-in')).toBeTruthy();
  expect(screen.queryByText('Settings')).toBeNull();
});

/**
 * At launch there is briefly no session because none has been read yet. Leaving
 * then would bounce every returning user through sign-in on their way in.
 */
it('waits for the stored session before deciding', () => {
  mockSession.mockReturnValue({ isSignedIn: false, isRestoring: true });
  renderTabs();

  expect(screen.queryByText(/redirected to/)).toBeNull();
  expect(screen.getByText('Today')).toBeTruthy();
});
