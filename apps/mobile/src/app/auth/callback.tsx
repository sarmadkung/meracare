import { Redirect } from 'expo-router';
import { ActivityIndicator, View } from 'react-native';

import { Screen, Text } from '@/components/ui';
import { useSession } from '@/features/auth/session-provider';
import { useTheme } from '@/theme';

/**
 * Landing route for the OAuth callback.
 *
 * On web the browser returns here with the authorization code in the URL and
 * the Supabase client exchanges it (`detectSessionInUrl`), so this screen waits
 * for the session to appear and then hands over to the app. On native the
 * in-app browser session consumes the deep link before the router sees it, so
 * this route is only reached if the system opens the link directly — in which
 * case sending the person to sign-in is the correct recovery.
 */
export default function AuthCallbackScreen() {
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
            Signing you in…
          </Text>
        </View>
      </Screen>
    );
  }

  return <Redirect href={isSignedIn ? '/home' : '/sign-in'} />;
}
