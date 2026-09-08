import { Stack, router } from 'expo-router';
import { useState } from 'react';
import { View } from 'react-native';

import { Button, Card, Screen, Text, TextField } from '@/components/ui';
import { formatInvitationCode, parseInvitationCode } from '@/features/circle/invitation-code';
import { useTheme } from '@/theme';

/**
 * Enter an invitation code.
 *
 * The way into a care circle you were invited to. Reachable before signing in,
 * because somebody invited as a caregiver may have no account yet and should be
 * able to see what they are joining first — the screen this leads to previews
 * the invitation without a session.
 */
export default function JoinCareCircleScreen() {
  const theme = useTheme();
  const [entry, setEntry] = useState('');
  const [error, setError] = useState<string | null>(null);

  // Whatever was typed or pasted, shown back grouped. A pasted link has no
  // sensible grouped rendering, so it is left as it is until it is submitted.
  const parsed = parseInvitationCode(entry);
  const display = /invitations\//i.test(entry) ? entry : formatInvitationCode(canonicalise(entry));

  function handleChange(next: string) {
    setEntry(next);
    setError(null);
  }

  function handleContinue() {
    const token = parseInvitationCode(entry);
    if (token === null) {
      setError('That code does not look right. Check it against the message you were sent.');
      return;
    }
    router.push({ pathname: '/invitations/[token]', params: { token } });
  }

  return (
    <Screen scrollable>
      <Stack.Screen options={{ headerShown: true, title: 'Join a care circle' }} />

      <View style={{ gap: theme.spacing.sm }}>
        <Text variant="pageHeading">Join a care circle</Text>
        <Text variant="body" color="secondary">
          Enter the invitation code you were sent. You will see who invited you and what you will be
          able to do before anything is confirmed.
        </Text>
      </View>

      <Card>
        <TextField
          label="Invitation code"
          value={display}
          onChangeText={handleChange}
          placeholder="K7M2-9XQP-4WTZ"
          autoCapitalize="characters"
          autoCorrect={false}
          autoComplete="off"
          spellCheck={false}
          error={error ?? undefined}
          onSubmitEditing={handleContinue}
          returnKeyType="go"
        />
        <Text variant="secondary" color="secondary">
          You can paste the whole invitation message — we will find the code in it.
        </Text>
      </Card>

      <Button label="Continue" onPress={handleContinue} disabled={parsed === null} />
    </Screen>
  );
}

/** Strips grouping so the field can re-render it, without judging validity. */
function canonicalise(raw: string): string {
  return raw.replace(/[\s-]/g, '').toUpperCase();
}
