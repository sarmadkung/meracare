import { permissionLabel, roleLabel } from '@meracare/contracts';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import { ActivityIndicator, View } from 'react-native';

import { Button, Card, Screen, Text } from '@/components/ui';
import { useSession } from '@/features/auth/session-provider';
import { useAcceptInvitation, useInvitationPreview } from '@/features/circle/use-circle';
import { ApiError } from '@/lib/api-error';
import { useTheme } from '@/theme';

/**
 * Accept an invitation.
 *
 * Readable before signing in, so somebody with no account can see what they are
 * being asked to join and then create one. Accepting itself requires a session,
 * because the membership has to attach to a user.
 */
export default function AcceptInvitationScreen() {
  const theme = useTheme();
  const { token } = useLocalSearchParams<{ token: string }>();
  const { isSignedIn, isRestoring } = useSession();
  const preview = useInvitationPreview(token ?? null);
  const accept = useAcceptInvitation();

  if (isRestoring || preview.isPending) {
    return (
      <Screen>
        <View style={{ alignItems: 'center', flex: 1, justifyContent: 'center' }}>
          <ActivityIndicator color={theme.colors.primary} />
        </View>
      </Screen>
    );
  }

  if (preview.isError) {
    return (
      <Screen scrollable>
        <Stack.Screen options={{ headerShown: true, title: 'Invitation' }} />
        <Card>
          <Text variant="sectionHeading">This invitation cannot be used</Text>
          <Text variant="body" color="secondary">
            {preview.error instanceof ApiError
              ? preview.error.message
              : 'The link may be incorrect, or the invitation may have been cancelled.'}
          </Text>
          <Button variant="secondary" label="Go to MeraCare" onPress={() => router.replace('/')} />
        </Card>
      </Screen>
    );
  }

  const invitation = preview.data;
  const usable = invitation.status === 'pending';

  async function handleAccept() {
    const result = await accept.mutateAsync(token ?? '');
    router.replace({
      pathname: '/seniors/[seniorId]',
      params: { seniorId: result.seniorId },
    });
  }

  return (
    <Screen scrollable>
      <Stack.Screen options={{ headerShown: true, title: 'Invitation' }} />

      <View style={{ gap: theme.spacing.sm }}>
        <Text variant="pageHeading">You have been invited</Text>
        <Text variant="body" color="secondary">
          {invitation.inviterName} has invited you to help with {invitation.seniorName}&apos;s care.
        </Text>
      </View>

      <Card>
        <Text variant="sectionHeading">Your role</Text>
        <Text variant="body">{roleLabel(invitation.role)}</Text>
      </Card>

      <Card>
        <Text variant="sectionHeading">What you will be able to do</Text>
        {invitation.permissions.map((permission) => {
          const described = permissionLabel(permission);
          return (
            <View key={permission} style={{ gap: theme.spacing.xs }}>
              <Text variant="body">{described.label}</Text>
              <Text variant="secondary" color="secondary">
                {described.description}
              </Text>
            </View>
          );
        })}
      </Card>

      {!usable ? (
        <Card>
          <Text variant="bodyStrong" color="warning">
            {invitation.status === 'expired'
              ? 'This invitation has expired.'
              : 'This invitation is no longer available.'}
          </Text>
          <Text variant="secondary" color="secondary">
            Ask {invitation.inviterName} to send a new one.
          </Text>
        </Card>
      ) : !isSignedIn ? (
        <Card>
          <Text variant="body">
            Sign in as {invitation.inviteeEmail} to accept. If you do not have an account yet, you
            can create one with that address.
          </Text>
          <Button
            label="Sign in or create an account"
            // Carry the invitation across sign-in. Without this the person
            // lands on the dashboard afterwards and the invitation they came
            // to accept is gone.
            onPress={() =>
              router.push({
                pathname: '/sign-in',
                params: { next: `/invitations/${token ?? ''}` },
              })
            }
          />
        </Card>
      ) : (
        <View style={{ gap: theme.spacing.md }}>
          {accept.isError ? (
            <Text variant="secondary" color="danger">
              {accept.error instanceof ApiError
                ? accept.error.message
                : 'We could not accept that invitation.'}
            </Text>
          ) : null}

          <Button label="Accept invitation" onPress={handleAccept} loading={accept.isPending} />
          <Button variant="ghost" label="Not now" onPress={() => router.replace('/')} />
        </View>
      )}
    </Screen>
  );
}
