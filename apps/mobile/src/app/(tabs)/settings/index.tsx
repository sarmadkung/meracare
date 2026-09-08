import { Stack } from 'expo-router';
import { View } from 'react-native';

import { Button, ListRow, Screen, SectionHeader, Text } from '@/components/ui';
import { useAuthActions } from '@/features/auth/use-auth-actions';
import { useTheme } from '@/theme';

/**
 * Settings (docs/13-mvp-screen-map.md, screens 24 and 26).
 *
 * Specified in the screen map and never built, which is why sign-out spent its
 * life stranded in the middle of Home between two unrelated buttons, reading as
 * one more thing you might casually tap.
 */
export default function SettingsScreen() {
  const theme = useTheme();
  const { signOut, isSubmitting, error: signOutError } = useAuthActions();

  return (
    <Screen scrollable variant="list">
      <Stack.Screen options={{ title: 'Settings' }} />

      <SectionHeader title="Account" />

      <ListRow title="Profile" subtitle="Your name and details" href="/settings/profile" />
      <ListRow
        title="Notifications"
        subtitle="Reminders and alerts"
        href="/settings/notifications"
      />
      <ListRow
        title="Appearance"
        subtitle="Light, dark, or match your phone"
        href="/settings/appearance"
      />

      <View style={{ gap: theme.spacing.md, marginTop: theme.spacing.xl }}>
        {/*
          Sign-out can refuse — a queued care update that has not reached the
          server yet, most often. Left unsaid, the button reads as broken, which
          is exactly how this was reported.
        */}
        {signOutError !== null ? (
          <Text accessibilityRole="alert" variant="secondary" color="danger">
            {signOutError}
          </Text>
        ) : null}

        <Button variant="ghost" label="Sign out" onPress={signOut} loading={isSubmitting} />
      </View>
    </Screen>
  );
}
