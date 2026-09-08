import { QueryClientProvider } from '@tanstack/react-query';
import {
  Inter_400Regular,
  Inter_600SemiBold,
  Inter_700Bold,
  useFonts,
} from '@expo-google-fonts/inter';
import { Stack } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { useState } from 'react';
import { GestureHandlerRootView } from 'react-native-gesture-handler';
import { SafeAreaProvider } from 'react-native-safe-area-context';

import { SessionProvider, useSession } from '@/features/auth/session-provider';
import {
  usePendingDestination,
  usePendingMedicationNotificationAction,
  useReminderSync,
  useReminderTaps,
} from '@/features/notifications/use-reminder-sync';
import { createQueryClient } from '@/lib/query-client';
import { ThemeProvider, useTheme } from '@/theme';

export default function RootLayout() {
  const [fontsLoaded, fontError] = useFonts({
    Inter_400Regular,
    Inter_600SemiBold,
    Inter_700Bold,
  });
  // One client per app instance; created lazily so it survives Fast Refresh.
  const [queryClient] = useState(createQueryClient);

  if (!fontsLoaded && !fontError) return null;

  return (
    <GestureHandlerRootView style={{ flex: 1 }}>
      <SafeAreaProvider>
        <QueryClientProvider client={queryClient}>
          <ThemeProvider>
            <SessionProvider>
              <ThemedStatusBar />
              <Reminders />
              <ThemedStack />
            </SessionProvider>
          </ThemeProvider>
        </QueryClientProvider>
      </SafeAreaProvider>
    </GestureHandlerRootView>
  );
}

/**
 * Keeps this device's scheduled reminders in step with the server, and opens
 * the right screen when one is tapped.
 *
 * Mounted once, at the root, and renders nothing. It has to outlive every
 * screen: a notification tapped from the lock screen arrives before any screen
 * has mounted (plans/phase8.md §15).
 */
function Reminders() {
  const { isSignedIn, isRestoring } = useSession();

  useReminderSync(isSignedIn, isRestoring);
  useReminderTaps(isSignedIn, isRestoring);
  usePendingDestination(isSignedIn, isRestoring);
  usePendingMedicationNotificationAction(isSignedIn, isRestoring);

  return null;
}

/**
 * Header styling for every screen, declared once.
 *
 * Screens set only their title. Twenty-four of them previously repeated the
 * whole options object — `settings/notifications.tsx` three times over, once
 * per render branch — which is how headers drifted apart from one another and
 * from the theme.
 *
 * `headerShown` stays false by default because screens opt in individually
 * today, and flipping it here would put a bar on sign-in and onboarding.
 */
function ThemedStack() {
  const theme = useTheme();

  return (
    <Stack
      screenOptions={{
        headerShown: false,
        headerStyle: { backgroundColor: theme.colors.background },
        headerTintColor: theme.colors.primary,
        headerTitleStyle: {
          color: theme.colors.textPrimary,
          fontFamily: 'Inter_600SemiBold',
          fontSize: 17,
        },
        headerShadowVisible: false,
        headerBackButtonDisplayMode: 'minimal',
        contentStyle: { backgroundColor: theme.colors.background },
      }}
    />
  );
}

function ThemedStatusBar() {
  const theme = useTheme();
  return <StatusBar style={theme.isDark ? 'light' : 'dark'} />;
}
