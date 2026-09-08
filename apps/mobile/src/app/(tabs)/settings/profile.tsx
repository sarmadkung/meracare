import { Stack } from 'expo-router';

import { Card, Screen, Text } from '@/components/ui';
import { useSession } from '@/features/auth/session-provider';

/**
 * Profile (docs/13-mvp-screen-map.md, screen 24).
 *
 * Read-only for now: it answers the question people actually open this screen
 * with, which is who the app currently thinks they are. Editing belongs with
 * the account settings work and is deliberately not stubbed here — a disabled
 * field that never becomes enabled is worse than an honest absence.
 */
export default function ProfileScreen() {
  const { session } = useSession();

  return (
    <Screen scrollable variant="detail">
      <Stack.Screen options={{ title: 'Profile' }} />

      <Card>
        <Text variant="label" color="secondary">
          SIGNED IN AS
        </Text>
        <Text variant="bodyStrong">{session?.user.email ?? 'Unknown'}</Text>
      </Card>
    </Screen>
  );
}
