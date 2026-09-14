import type { CarePermission } from '@genxcare/contracts';
import { permissionLabelsByGroup, roleLabel } from '@genxcare/contracts';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import { useEffect, useMemo, useState } from 'react';
import { ActivityIndicator, View } from 'react-native';

import { Button, Card, Screen, Text } from '@/components/ui';
import { PermissionToggle } from '@/components/ui/permission-toggle';
import { useCircleMembers, useUpdateMember } from '@/features/circle/use-circle';
import { useSenior } from '@/features/seniors/use-seniors';
import { ApiError } from '@/lib/api-error';
import { useTheme } from '@/theme';

/**
 * Change what one member of the care circle can do.
 *
 * The editor can only grant what they hold themselves, so anything outside
 * their own permissions is shown disabled rather than hidden — that makes the
 * limit visible instead of mysterious.
 */
export default function EditMemberScreen() {
  const theme = useTheme();
  const { seniorId, relationshipId } = useLocalSearchParams<{
    seniorId: string;
    relationshipId: string;
  }>();

  const senior = useSenior(seniorId ?? null);
  const members = useCircleMembers(seniorId ?? null);
  const updateMember = useUpdateMember(seniorId ?? '');

  const member = members.data?.find((entry) => entry.id === relationshipId);
  const grantable = useMemo(() => senior.data?.permissions ?? [], [senior.data]);
  const sections = useMemo(() => permissionLabelsByGroup(), []);

  const [selected, setSelected] = useState<CarePermission[] | null>(null);

  // Seed once from the stored set, so a refetch cannot discard edits.
  useEffect(() => {
    if (selected === null && member) {
      setSelected(member.permissions);
    }
  }, [member, selected]);

  if (senior.isPending || members.isPending || selected === null) {
    return (
      <Screen>
        <View style={{ alignItems: 'center', flex: 1, justifyContent: 'center' }}>
          <ActivityIndicator color={theme.colors.primary} />
        </View>
      </Screen>
    );
  }

  if (!member) {
    return (
      <Screen scrollable>
        <Stack.Screen options={{ headerShown: true, title: 'Access' }} />
        <Card>
          <Text variant="sectionHeading">This person is no longer in the care circle</Text>
          <Button variant="secondary" label="Back" onPress={() => router.back()} />
        </Card>
      </Screen>
    );
  }

  function toggle(permission: CarePermission) {
    setSelected((current) =>
      (current ?? []).includes(permission)
        ? (current ?? []).filter((entry) => entry !== permission)
        : [...(current ?? []), permission],
    );
  }

  async function handleSave() {
    await updateMember.mutateAsync({
      relationshipId: relationshipId ?? '',
      permissions: selected ?? [],
    });
    router.back();
  }

  return (
    <Screen scrollable>
      <Stack.Screen options={{ headerShown: true, title: 'Access' }} />

      <View style={{ gap: theme.spacing.sm }}>
        <Text variant="pageHeading">{member.displayName}</Text>
        <Text variant="body" color="secondary">
          {roleLabel(member.role)}
        </Text>
      </View>

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
              disabled={!grantable.includes(entry.permission)}
              onToggle={() => toggle(entry.permission)}
            />
          ))}
        </View>
      ))}

      <Text variant="secondary" color="secondary">
        Greyed-out items are things you cannot pass on, because you do not have them yourself.
      </Text>

      {updateMember.isError ? (
        <Text variant="secondary" color="danger">
          {updateMember.error instanceof ApiError
            ? updateMember.error.message
            : 'We could not save those changes.'}
        </Text>
      ) : null}

      <Button label="Save access" onPress={handleSave} loading={updateMember.isPending} />
      <Button variant="ghost" label="Cancel" onPress={() => router.back()} />
    </Screen>
  );
}
