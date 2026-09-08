import { render, screen } from '@testing-library/react-native';
import type { ReactNode } from 'react';

import { ThemeProvider } from '@/theme';

import TabsLayout from '../(tabs)/_layout';

jest.mock('@/features/auth/session-provider', () => ({
  useSession: () => ({ isSignedIn: true, isRestoring: false }),
}));
jest.mock('@/features/notifications/use-notifications', () => ({ useUnreadCount: () => 3 }));

// Render the tab configuration rather than a real navigator: this test is about
// which destinations exist and what they are called, not about react-navigation.
jest.mock('expo-router/js-tabs', () => {
  // Required inside the factory: jest hoists this above the imports, so a
  // top-level import of react-native would not exist yet.
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

it('offers the four top-level destinations', () => {
  render(
    <ThemeProvider>
      <TabsLayout />
    </ThemeProvider>,
  );

  expect(screen.getByText('Today')).toBeTruthy();
  expect(screen.getByText('Circle')).toBeTruthy();
  expect(screen.getByText('Settings')).toBeTruthy();
});

/**
 * The unread count used to sit on a button inside Home, which meant you had to
 * already be on Home to learn that anything needed you.
 */
it('carries the unread count on Alerts', () => {
  render(
    <ThemeProvider>
      <TabsLayout />
    </ThemeProvider>,
  );

  expect(screen.getByText('Alerts 3')).toBeTruthy();
});
