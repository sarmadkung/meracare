import type { MedicationDose } from '@genxcare/contracts';
import {
  can,
  doseDateLabel,
  doseStatusLabel,
  doseTimeLabel,
  formLabel,
  nextDoseLabel,
  schedulesLabel,
} from '@genxcare/contracts';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import { ActivityIndicator, View } from 'react-native';

import { Button, Card, Screen, Text } from '@/components/ui';
import { useCircleMembers } from '@/features/circle/use-circle';
import {
  useMedication,
  useMedicationHistory,
  useUpdateMedication,
} from '@/features/medications/use-medications';
import { useSenior } from '@/features/seniors/use-seniors';
import { ApiError } from '@/lib/api-error';
import { useTheme } from '@/theme';

/**
 * One medication in full: what it is, when it is taken, and what has happened.
 *
 * Every action is permission-aware: the API refuses what the caller may not do,
 * and offering a control that will be refused only wastes their time
 * (plans/phase5.md §15).
 */
export default function MedicationDetailScreen() {
  const theme = useTheme();
  const { medicationId } = useLocalSearchParams<{ medicationId: string }>();

  const medication = useMedication(medicationId ?? null);
  const seniorId = medication.data?.seniorId ?? null;

  const senior = useSenior(seniorId);
  const members = useCircleMembers(seniorId);
  const history = useMedicationHistory(medicationId ?? null);
  const update = useUpdateMedication(seniorId ?? '');

  if (medication.isPending) {
    return (
      <Screen>
        <View style={{ alignItems: 'center', flex: 1, justifyContent: 'center' }}>
          <ActivityIndicator color={theme.colors.primary} />
        </View>
      </Screen>
    );
  }

  if (medication.isError) {
    return (
      <Screen scrollable>
        <Stack.Screen options={{ headerShown: true, title: 'Medication' }} />
        <Card>
          <Text variant="sectionHeading">This medication is not available</Text>
          <Text variant="body" color="secondary">
            {medication.error instanceof ApiError
              ? medication.error.message
              : 'It may have been removed, or you may not have access.'}
          </Text>
          <Button variant="secondary" label="Back" onPress={() => router.back()} />
        </Card>
      </Screen>
    );
  }

  const detail = medication.data;
  const timezone = senior.data?.timezone ?? 'UTC';
  const canManage = senior.data ? can(senior.data, 'medications.manage') : false;

  const actorName = (userId: string | null) =>
    members.data?.find((member) => member.userId === userId)?.displayName ?? 'Somebody';

  const doses = history.data?.pages.flatMap((page) => page.items) ?? [];

  return (
    <Screen scrollable>
      <Stack.Screen options={{ headerShown: true, title: 'Medication' }} />

      <View style={{ gap: theme.spacing.sm }}>
        <Text variant="pageHeading">{detail.name}</Text>
        {detail.dosage ? (
          <Text variant="body" color="secondary">
            {detail.dosage}
            {detail.form ? ` · ${formLabel(detail.form)}` : ''}
          </Text>
        ) : null}
        {!detail.active ? (
          <Text variant="bodyStrong" color="secondary">
            No longer taken. Its record is kept.
          </Text>
        ) : null}
      </View>

      {detail.instructions ? (
        <Card>
          <Text variant="sectionHeading">How to take it</Text>
          <Text variant="body">{detail.instructions}</Text>
        </Card>
      ) : null}

      <Card>
        <Text variant="sectionHeading">When</Text>
        <Row label="Schedule" value={schedulesLabel(detail.schedules)} />
        {detail.active ? (
          <Row label="Next dose" value={nextDoseLabel(detail.nextDoseAt, timezone)} />
        ) : null}
        <Text variant="secondary" color="secondary">
          Shown in {senior.data?.displayName ?? 'this person'}&apos;s timezone.
        </Text>
      </Card>

      {detail.notes ? (
        <Card>
          <Text variant="sectionHeading">Notes</Text>
          <Text variant="body">{detail.notes}</Text>
        </Card>
      ) : null}

      <Card>
        <Text variant="sectionHeading">Recent doses</Text>

        {history.isPending ? (
          <ActivityIndicator color={theme.colors.primary} />
        ) : doses.length === 0 ? (
          <Text variant="body" color="secondary">
            Nothing recorded yet.
          </Text>
        ) : (
          doses.map((dose) => (
            <HistoryRow key={dose.id} dose={dose} timezone={timezone} actorName={actorName} />
          ))
        )}

        {history.hasNextPage ? (
          <Button
            variant="secondary"
            label="Show more"
            loading={history.isFetchingNextPage}
            onPress={() => void history.fetchNextPage()}
          />
        ) : null}
      </Card>

      {update.isError ? (
        <Text variant="secondary" color="danger">
          {update.error instanceof ApiError
            ? update.error.message
            : 'That did not save. Please try again.'}
        </Text>
      ) : null}

      {canManage ? (
        <Button
          variant="secondary"
          label="Edit medication"
          onPress={() =>
            router.push({
              pathname: '/medications/[medicationId]/edit',
              params: { medicationId: detail.id },
            })
          }
        />
      ) : null}

      {canManage && detail.active ? (
        <Button
          variant="ghost"
          label="Stop this medication"
          loading={update.isPending}
          onPress={() => update.mutate({ medicationId: detail.id, active: false })}
        />
      ) : null}

      {canManage && !detail.active ? (
        <Button
          variant="secondary"
          label="Start taking it again"
          loading={update.isPending}
          onPress={() => update.mutate({ medicationId: detail.id, active: true })}
        />
      ) : null}
    </Screen>
  );
}

/**
 * One dose in the history.
 *
 * Says who recorded it as well as what happened: a family member reading this
 * next week needs to know it was the visiting caregiver, not a guess
 * (plans/phase5.md §16).
 */
function HistoryRow({
  dose,
  timezone,
  actorName,
}: {
  dose: MedicationDose;
  timezone: string;
  actorName: (userId: string | null) => string;
}) {
  const theme = useTheme();

  const recordedBy =
    dose.status === 'taken'
      ? `Taken by ${actorName(dose.takenBy)}`
      : dose.status === 'skipped'
        ? `Skipped by ${actorName(dose.skippedBy)}`
        : doseStatusLabel(dose.status);

  return (
    <View style={{ gap: theme.spacing.xs }}>
      <Text variant="bodyStrong">
        {doseDateLabel(dose, timezone)} at {doseTimeLabel(dose, timezone)}
      </Text>
      <Text variant="secondary" color="secondary">
        {dose.dosage ? `${dose.dosage} · ` : ''}
        {recordedBy}
      </Text>
      {dose.notes ? (
        <Text variant="secondary" color="secondary">
          {dose.notes}
        </Text>
      ) : null}
    </View>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  const theme = useTheme();

  return (
    <View style={{ gap: theme.spacing.xs }}>
      <Text variant="secondary" color="secondary">
        {label}
      </Text>
      <Text variant="body">{value}</Text>
    </View>
  );
}
