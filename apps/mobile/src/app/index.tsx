import { Redirect } from 'expo-router';
import { ActivityIndicator, View } from 'react-native';

import { Screen, Text } from '@/components/ui';
import { useSession } from '@/features/auth/session-provider';
import { useTheme } from '@/theme';

/**
 * Entry route: waits for the stored session to be restored, then sends the user
 * to the app or to sign-in.
 */
export default function Index() {
  const { isRestoring, isSignedIn } = useSession();
  const theme = useTheme();

  if (isRestoring) {
    return (
      <Screen>
        <View
          style={{ alignItems: 'center', flex: 1, gap: theme.spacing.lg, justifyContent: 'center' }}
        >
          <ActivityIndicator color={theme.colors.primary} size="large" />
          <Text variant="secondary" color="secondary">
            Loading GenxCare…
          </Text>
        </View>
      </Screen>
    );
  }

  return <Redirect href={isSignedIn ? '/home' : '/sign-in'} />;
}
