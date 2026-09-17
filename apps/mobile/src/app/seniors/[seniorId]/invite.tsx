import type { CarePermission, InvitableRole } from '@genxcare/contracts';
import { permissionLabelsByGroup, roleLabel } from '@genxcare/contracts';
import * as Linking from 'expo-linking';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import { useMemo, useState } from 'react';
import { Share, View } from 'react-native';

import { Button, Card, OptionCard, Screen, Text, TextField } from '@/components/ui';
import { PermissionToggle } from '@/components/ui/permission-toggle';
import { formatInvitationCode } from '@/features/circle/invitation-code';
import { useCreateInvitation } from '@/features/circle/use-circle';
import { useSenior } from '@/features/seniors/use-seniors';
import { ApiError } from '@/lib/api-error';
import { useTheme } from '@/theme';

/**
 * Invite someone into the care circle.
 *
 * Role first, then the person, then exactly what they will be able to do. The
 * permission list is limited to what the inviter holds themselves — the API
 * refuses anything more, so offering it would only produce a rejection.
 */
const ROLES: { role: InvitableRole; description: string }[] = [
  {
    role: 'family_member',
    description: 'A relative helping to coordinate care.',
  },
  {
    role: 'professional_caregiver',
    description: 'A paid caregiver carrying out day-to-day care.',
  },
];

/** Sensible starting selections, so the form is useful without being permissive. */
const ROLE_DEFAULTS: Record<InvitableRole, CarePermission[]> = {
  family_member: [
    'senior.view',
    'tasks.view',
    'tasks.complete',
    'medications.view',
    'appointments.view',
    'notes.view',
    'activity.view',
    'members.view',
    'messages.participate',
  ],
  professional_caregiver: [
    'senior.view',
    'tasks.view',
    'tasks.complete',
    'medications.view',
    'medications.record',
    'appointments.view',
    'notes.view',
    'notes.create',
    'members.view',
  ],
};

export default function InviteScreen() {
  const theme = useTheme();
  const { seniorId } = useLocalSearchParams<{ seniorId: string }>();
  const senior = useSenior(seniorId ?? null);
  const createInvitation = useCreateInvitation(seniorId ?? '');

  const [role, setRole] = useState<InvitableRole>('family_member');
  const [email, setEmail] = useState('');
  const [selected, setSelected] = useState<CarePermission[]>(ROLE_DEFAULTS.family_member);
  const [token, setToken] = useState<string | null>(null);

  // Only permissions the inviter holds can be delegated, so the form offers
  // exactly those.
  const grantable = useMemo(() => senior.data?.permissions ?? [], [senior.data]);
  const sections = useMemo(() => permissionLabelsByGroup(grantable), [grantable]);

  const fieldErrors =
    createInvitation.error instanceof ApiError ? (createInvitation.error.details ?? {}) : {};

  function chooseRole(next: InvitableRole) {
    setRole(next);
    setSelected(ROLE_DEFAULTS[next].filter((permission) => grantable.includes(permission)));
  }

  function toggle(permission: CarePermission) {
    setSelected((current) =>
      current.includes(permission)
        ? current.filter((entry) => entry !== permission)
        : [...current, permission],
    );
  }

  async function handleSend() {
    const created = await createInvitation.mutateAsync({
      email: email.trim(),
      role,
      permissions: selected,
    });
    // The token is shown once and cannot be retrieved again, so it goes on
    // screen for the inviter to pass on. Delivery by email arrives with the
    // notification work in Phase 8.
    setToken(created.token);
  }

  if (token !== null) {
    return (
      <InvitationSent
        token={token}
        email={email.trim()}
        seniorName={senior.data?.displayName ?? 'this person'}
      />
    );
  }

  return (
    <Screen scrollable>
      <Stack.Screen options={{ headerShown: true, title: 'Invite someone' }} />

      <View style={{ gap: theme.spacing.sm }}>
        <Text variant="pageHeading">Invite someone</Text>
        <Text variant="body" color="secondary">
          Choose what they will be able to do for {senior.data?.displayName ?? 'this person'}. You
          can change it later.
        </Text>
      </View>

      <View accessibilityRole="radiogroup" style={{ gap: theme.spacing.md }}>
        {ROLES.map((option) => (
          <OptionCard
            key={option.role}
            title={roleLabel(option.role)}
            description={option.description}
            selected={option.role === role}
            onPress={() => chooseRole(option.role)}
          />
        ))}
      </View>

      <Card>
        <TextField
          label="Their email address"
          value={email}
          onChangeText={setEmail}
          autoCapitalize="none"
          autoComplete="email"
          keyboardType="email-address"
          inputMode="email"
          placeholder="name@example.com"
          error={fieldErrors.email}
        />
      </Card>

      <View style={{ gap: theme.spacing.lg }}>
        <Text variant="sectionHeading">What they can do</Text>

        {sections.map((section) => (
          <View key={section.group} style={{ gap: theme.spacing.sm }}>
            <Text variant="bodyStrong" color="secondary">
              {section.group}
            </Text>
            {section.permissions.map((entry) => (
              <PermissionToggle
                key={entry.permission}
                entry={entry}
                selected={selected.includes(entry.permission)}
                onToggle={() => toggle(entry.permission)}
              />
            ))}
          </View>
        ))}
      </View>

      {createInvitation.isError && Object.keys(fieldErrors).length === 0 ? (
        <Text variant="secondary" color="danger">
          {createInvitation.error instanceof ApiError
            ? createInvitation.error.message
            : 'We could not send that invitation. Please try again.'}
        </Text>
      ) : null}

      <Button
        label="Send invitation"
        onPress={handleSend}
        disabled={email.trim().length === 0 || selected.length === 0}
        loading={createInvitation.isPending}
      />
      <Button variant="ghost" label="Cancel" onPress={() => router.back()} />
    </Screen>
  );
}

/** Shown once, with the token that cannot be retrieved again. */
function InvitationSent({
  token,
  email,
  seniorName,
}: {
  token: string;
  email: string;
  seniorName: string;
}) {
  const theme = useTheme();

  // Resolves to genxcare://invitations/<code> in a build and to the exp://
  // equivalent under Expo Go, so the link works wherever this is running.
  const link = Linking.createURL(`/invitations/${token}`);

  async function handleShare() {
    await Share.share({
      message: [
        `You have been invited to help with ${seniorName}'s care on GenxCare.`,
        '',
        'Your invitation code:',
        formatInvitationCode(token),
        '',
        'Already have the app? Open this link:',
        link,
        '',
        'This code expires in seven days.',
      ].join('\n'),
    });
  }

  return (
    <Screen scrollable>
      <Stack.Screen options={{ headerShown: true, title: 'Invitation sent' }} />

      <Text variant="pageHeading">Invitation ready</Text>

      <Card>
        <Text variant="body">
          Send this invitation code to {email}. They will need it to join the care circle.
        </Text>
        <View
          style={{
            backgroundColor: theme.colors.surfaceMuted,
            borderRadius: theme.radii.md,
            padding: theme.spacing.lg,
          }}
        >
          <Text variant="bodyStrong" selectable>
            {formatInvitationCode(token)}
          </Text>
        </View>
        <Button label="Share invitation" onPress={handleShare} />
        <Text variant="secondary" color="secondary">
          This code is shown only once and expires in seven days. If it is lost, cancel the
          invitation and send a new one.
        </Text>
      </Card>

      <Button variant="ghost" label="Done" onPress={() => router.back()} />
    </Screen>
  );
}
