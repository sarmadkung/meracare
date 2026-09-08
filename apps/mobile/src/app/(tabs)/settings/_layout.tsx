import { Stack } from 'expo-router';

import { headerOptions, useTheme } from '@/theme';

/**
 * Settings is a stack so Profile and Notification settings push over its index
 * with a back button, rather than replacing the tab wholesale.
 *
 * It restates the header styling because a nested navigator inherits nothing
 * from the stack above it. Screens here still declare only a title.
 */
export default function SettingsLayout() {
  const theme = useTheme();

  return (
    <Stack
      screenOptions={{
        headerShown: true,
        headerBackButtonDisplayMode: 'minimal',
        ...headerOptions(theme),
      }}
    />
  );
}
