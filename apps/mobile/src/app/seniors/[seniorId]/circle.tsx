import type { CircleMember, Invitation } from '@genxcare/contracts';
import { can, roleLabel } from '@genxcare/contracts';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import { ActivityIndicator, View } from 'react-native';

import { Button, Card, Illustration, Screen, Text } from '@/components/ui';
import {
  useCircleMembers,
  useInvitations,
  useLeaveCareCircle,
  useRevokeInvitation,
  useRevokeMember,
} from '@/features/circle/use-circle';
import { useSenior } from '@/features/seniors/use-seniors';
import { ApiError } from '@/lib/api-error';
import { confirmAction } from '@/lib/dialogs';
import { useTheme } from '@/theme';

/**
 * Care Circle.
 *
 * Shows who is involved and what each person can do. Management controls are
 * rendered only for a reader who holds the matching permission — the API
 * enforces the same rules, so hiding them is courtesy rather than security
 * (docs/02).
 */
export default function CareCircleScreen() {
  const theme = useTheme();
  const { seniorId } = useLocalSearchParams<{ seniorId: string }>();
  const senior = useSenior(seniorId ?? null);
  const members = useCircleMembers(seniorId ?? null);

  const mayInvite = senior.data ? can(senior.data, 'members.invite') : false;
  const mayManage = senior.data ? can(senior.data, 'members.manage') : false;
  const invitations = useInvitations(seniorId ?? null, senior.data !== undefined);
  const leave = useLeaveCareCircle(seniorId ?? '');

  if (senior.isPending || members.isPending) {
    return (
      <Screen>
        <View style={{ alignItems: 'center', flex: 1, justifyContent: 'center' }}>
          <ActivityIndicator color={theme.colors.primary} />
        </View>
      </Screen>
    );
  }

  if (senior.isError || members.isError) {
    const error = senior.error ?? members.error;
    return (
      <Screen scrollable>
        <Stack.Screen options={{ headerShown: true, title: 'Care circle' }} />
        <Card>
          <Text variant="sectionHeading">We could not load the care circle</Text>
          <Text variant="body" color="secondary">
            {error instanceof ApiError ? error.message : 'Please try again.'}
          </Text>
          <Button variant="secondary" label="Try again" onPress={() => void members.refetch()} />
        </Card>
      </Screen>
    );
  }

  const pending = (invitations.data ?? []).filter((invitation) => invitation.status === 'pending');
  const self = members.data.find((member) => member.isSelf);
  const seniorName = senior.data.displayName;

  function confirmLeave() {
    confirmAction({
      title: `Leave ${seniorName}'s care circle?`,
      message:
        'You will lose access immediately. Care you already recorded will stay in the history.',
      confirmLabel: 'Leave',
      onConfirm: () => leave.mutate(undefined, { onSuccess: () => router.replace('/') }),
    });
  }

  return (
    <Screen scrollable>
      <Stack.Screen options={{ headerShown: true, title: 'Care circle' }} />

      <View style={{ gap: theme.spacing.sm }}>
        <Text variant="pageHeading">Care circle</Text>
        <Text variant="body" color="secondary">
          The people involved in {senior.data.displayName}&apos;s care.
        </Text>
      </View>

      {mayInvite ? (
        <Button
          label="Invite someone"
          onPress={() =>
            router.push({
              pathname: '/seniors/[seniorId]/invite',
              params: { seniorId: senior.data.id },
            })
          }
        />
      ) : null}

      {mayInvite && members.data.length <= 1 ? (
        <Card>
          <Illustration name="careCircle" height={150} />
          <Text variant="sectionHeading">Build a trusted care circle</Text>
          <Text variant="body" color="secondary">
            Invite family or professional caregivers when you are ready to coordinate care.
          </Text>
        </Card>
      ) : null}

      <View style={{ gap: theme.spacing.md }}>
        {members.data.map((member) => (
          <MemberCard
            key={member.id}
            member={member}
            seniorId={senior.data.id}
            mayManage={mayManage}
          />
        ))}
      </View>

      {mayInvite && pending.length > 0 ? (
        <View style={{ gap: theme.spacing.md }}>
          <Text variant="sectionHeading">Waiting to be accepted</Text>
          {pending.map((invitation) => (
            <PendingInvitationCard
              key={invitation.id}
              invitation={invitation}
              seniorId={senior.data.id}
            />
          ))}
        </View>
      ) : null}

      {self && !self.isSenior ? (
        <Card>
          <Text variant="sectionHeading">Your access</Text>
          <Text variant="body" color="secondary">
            You can leave when another person can manage this care circle. Your past care records
            will remain attributed to you.
          </Text>
          {leave.isError ? (
            <Text variant="secondary" color="danger">
              {leave.error instanceof ApiError
                ? leave.error.message
                : 'We could not remove your access.'}
            </Text>
          ) : null}
          <Button
            variant="danger"
            label="Leave care circle"
            loading={leave.isPending}
            onPress={confirmLeave}
          />
        </Card>
      ) : null}
    </Screen>
  );
}

function MemberCard({
  member,
  seniorId,
  mayManage,
}: {
  member: CircleMember;
  seniorId: string;
  mayManage: boolean;
}) {
  const theme = useTheme();
  const revokeMember = useRevokeMember(seniorId);

  // The senior's own membership is fixed, and nobody is offered a control to
  // remove themselves by accident.
  const removable = mayManage && !member.isSenior && !member.isSelf;

  function confirmRemove() {
    confirmAction({
      title: `Remove ${member.displayName}?`,
      message:
        'They will lose access straight away. Anything they have already recorded stays in the care history.',
      confirmLabel: 'Remove',
      onConfirm: () => revokeMember.mutate(member.id),
    });
  }

  return (
    <Card>
      <View style={{ gap: theme.spacing.xs }}>
        <Text variant="bodyStrong">
          {member.displayName}
          {member.isSelf ? ' (you)' : ''}
        </Text>
        <Text variant="secondary" color="secondary">
          {roleLabel(member.role)}
        </Text>
        <Text variant="secondary" color="secondary">
          {member.permissions.length} permission{member.permissions.length === 1 ? '' : 's'}
        </Text>
      </View>

      {removable ? (
        <View style={{ flexDirection: 'row', gap: theme.spacing.sm }}>
          <View style={{ flex: 1 }}>
            <Button
              variant="secondary"
              label="Change access"
              onPress={() =>
                router.push({
                  pathname: '/seniors/[seniorId]/members/[relationshipId]',
                  params: { seniorId, relationshipId: member.id },
                })
              }
            />
          </View>
          <View style={{ flex: 1 }}>
            <Button
              variant="danger"
              label="Remove"
              onPress={confirmRemove}
              loading={revokeMember.isPending}
            />
          </View>
        </View>
      ) : null}

      {revokeMember.isError ? (
        <Text variant="secondary" color="danger">
          {revokeMember.error instanceof ApiError
            ? revokeMember.error.message
            : 'We could not remove that person.'}
        </Text>
      ) : null}
    </Card>
  );
}

function PendingInvitationCard({
  invitation,
  seniorId,
}: {
  invitation: Invitation;
  seniorId: string;
}) {
  const theme = useTheme();
  const revokeInvitation = useRevokeInvitation(seniorId);

  return (
    <Card>
      <View style={{ gap: theme.spacing.xs }}>
        <Text variant="bodyStrong">{invitation.inviteeEmail}</Text>
        <Text variant="secondary" color="secondary">
          Invited as {roleLabel(invitation.role).toLowerCase()}
        </Text>
        <Text variant="secondary" color="secondary">
          Expires {new Date(invitation.expiresAt).toLocaleDateString()}
        </Text>
      </View>

      <Button
        variant="secondary"
        label="Cancel invitation"
        onPress={() => revokeInvitation.mutate(invitation.id)}
        loading={revokeInvitation.isPending}
      />

      {revokeInvitation.isError ? (
        <Text variant="secondary" color="danger">
          {revokeInvitation.error instanceof ApiError
            ? revokeInvitation.error.message
            : 'We could not cancel that invitation.'}
        </Text>
      ) : null}
    </Card>
  );
}
