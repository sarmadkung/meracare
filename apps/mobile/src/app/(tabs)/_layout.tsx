import { Redirect } from 'expo-router';
import { Tabs } from 'expo-router/js-tabs';
import type { ColorValue } from 'react-native';

import { Icon, type IconName } from '@/components/ui';
import { useSession } from '@/features/auth/session-provider';
import { useUnreadCount } from '@/features/notifications/use-notifications';
import { useTheme } from '@/theme';

/**
 * The four places you can be.
 *
 * Today is time-oriented and Circle is people-oriented, which is the split
 * docs/13 already draws between screens 9 and 10. Keeping them apart is what
 * lets one screen set serve a family with one parent and a professional with
 * six clients, rather than the two separate apps that document forbids.
 *
 * Imported from `expo-router/js-tabs`: the same export from `expo-router` is
 * deprecated in v57.
 */
export default function TabsLayout() {
  const theme = useTheme();
  const { isSignedIn, isRestoring } = useSession();

  // Read off the inbox itself, so the badge and the list cannot disagree
  // (plans/phase11.md §61).
  const unread = useUnreadCount(isSignedIn);

  // Signing out happens on Settings, which had no guard of its own and so left
  // a signed-out person looking at their own settings. One guard on the layout
  // every tab shares beats four screens each remembering to have one.
  if (!isRestoring && !isSignedIn) return <Redirect href="/sign-in" />;

  return (
    <Tabs
      screenOptions={{
        headerShown: false,
        tabBarActiveTintColor: theme.colors.primary,
        tabBarInactiveTintColor: theme.colors.textMuted,
        tabBarStyle: {
          backgroundColor: theme.colors.surface,
          borderTopColor: theme.colors.border,
        },
        tabBarLabelStyle: { fontFamily: 'Inter_600SemiBold' },
      }}
    >
      <Tabs.Screen name="home" options={{ title: 'Today', tabBarIcon: tabIcon('today') }} />
      <Tabs.Screen name="circle" options={{ title: 'Circle', tabBarIcon: tabIcon('people') }} />
      <Tabs.Screen
        name="notifications"
        options={{
          title: 'Alerts',
          tabBarIcon: tabIcon('bell'),
          tabBarBadge: unread > 0 ? unread : undefined,
        }}
      />
      <Tabs.Screen
        name="settings"
        options={{ title: 'Settings', tabBarIcon: tabIcon('settings') }}
      />
    </Tabs>
  );
}

/**
 * Builds the render callback a tab bar wants for its icon.
 *
 * A named declaration rather than an arrow, so React and the devtools have
 * something to call it when it shows up in a tree.
 */
function tabIcon(name: IconName) {
  function TabIcon({ color, size }: { color: ColorValue; size: number }) {
    return <Icon name={name} color={color} size={size} />;
  }

  return TabIcon;
}
