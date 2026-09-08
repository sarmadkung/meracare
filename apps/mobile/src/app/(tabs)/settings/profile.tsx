import { Stack } from 'expo-router';
import { useState } from 'react';
import { ActivityIndicator, View } from 'react-native';

import { Avatar, Button, Card, Screen, Text, TextField } from '@/components/ui';
import { useSession } from '@/features/auth/session-provider';
import { useMe, useUpdateMe } from '@/features/profile/use-me';
import { useTheme } from '@/theme';

/**
 * Profile (docs/13-mvp-screen-map.md, screen 24).
 *
 * Your name and phone as everyone else in your circles sees them. The email is
 * shown but not editable here: it belongs to the sign-in identity rather than
 * to this record, and changing it means re-verifying an address, which is an
 * account flow rather than a profile edit.
 */
export default function ProfileScreen() {
  const theme = useTheme();
  const me = useMe();

  return (
    <Screen scrollable variant="detail">
      <Stack.Screen options={{ title: 'Profile' }} />

      {me.isPending ? (
        <View style={{ alignItems: 'center', paddingVertical: theme.spacing.xxl }}>
          <ActivityIndicator color={theme.colors.primary} />
        </View>
      ) : me.isError || me.data === undefined ? (
        <Card>
          <Text variant="bodyStrong">We could not load your profile</Text>
          <Text variant="secondary" color="secondary">
            Something went wrong. Please try again.
          </Text>
          <Button variant="secondary" label="Try again" onPress={() => void me.refetch()} />
        </Card>
      ) : (
        <Details displayName={me.data.displayName} phone={me.data.phone} />
      )}
    </Screen>
  );
}

function Details({ displayName, phone }: { displayName: string; phone: string | null }) {
  const theme = useTheme();
  const { session } = useSession();
  const update = useUpdateMe();

  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(displayName);
  const [phoneDraft, setPhoneDraft] = useState(phone ?? '');
  const [nameError, setNameError] = useState<string | null>(null);

  function startEditing() {
    setName(displayName);
    setPhoneDraft(phone ?? '');
    setNameError(null);
    setEditing(true);
  }

  function save() {
    const trimmed = name.trim();

    // A name is how the rest of the circle recognises you on a task or a note,
    // so a blank one would leave those rows attributed to nobody.
    if (trimmed === '') {
      setNameError('Please enter your name.');
      return;
    }

    const trimmedPhone = phoneDraft.trim();

    update.mutate(
      // An empty field means "remove it", which is null rather than "".
      { displayName: trimmed, phone: trimmedPhone === '' ? null : trimmedPhone },
      { onSuccess: () => setEditing(false) },
    );
  }

  if (editing) {
    return (
      <View style={{ gap: theme.spacing.lg }}>
        <TextField
          label="Your name"
          value={name}
          onChangeText={setName}
          error={nameError ?? undefined}
          autoCapitalize="words"
          textContentType="name"
        />

        <TextField
          label="Phone number"
          value={phoneDraft}
          onChangeText={setPhoneDraft}
          keyboardType="phone-pad"
          textContentType="telephoneNumber"
          placeholder="Optional"
        />

        {update.isError ? (
          <Text accessibilityRole="alert" variant="secondary" color="danger">
            We could not save your profile. Please try again.
          </Text>
        ) : null}

        <Button label="Save" onPress={save} loading={update.isPending} />
        <Button variant="ghost" label="Cancel" onPress={() => setEditing(false)} />
      </View>
    );
  }

  return (
    <View style={{ gap: theme.spacing.lg }}>
      <Card>
        <View style={{ alignItems: 'center', flexDirection: 'row', gap: theme.spacing.lg }}>
          <Avatar name={displayName} size={56} />
          <View style={{ flex: 1, gap: theme.spacing.xs }}>
            <Text variant="sectionHeading">{displayName}</Text>
            <Text variant="secondary" color="secondary">
              {phone ?? 'No phone number'}
            </Text>
          </View>
        </View>
      </Card>

      <Button variant="secondary" label="Edit profile" onPress={startEditing} />

      <Card>
        <Text variant="label" color="secondary">
          SIGNED IN AS
        </Text>
        <Text variant="body">{session?.user.email ?? 'Unknown'}</Text>
        <Text variant="secondary" color="secondary">
          Changing this means verifying a new address, so it lives with your account rather than
          here.
        </Text>
      </Card>
    </View>
  );
}
