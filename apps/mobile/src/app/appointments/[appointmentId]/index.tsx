import {
  appointmentDateLabel,
  appointmentKindLabel,
  appointmentStatusLabel,
  appointmentWhenLabel,
  can,
} from '@genxcare/contracts';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { ActivityIndicator, View } from 'react-native';

import { Button, Card, Screen, Text } from '@/components/ui';
import {
  useAppointment,
  useCancelAppointment,
  useCompleteAppointment,
} from '@/features/appointments/use-appointments';
import { useCircleMembers } from '@/features/circle/use-circle';
import { useSenior } from '@/features/seniors/use-seniors';
import { ApiError } from '@/lib/api-error';
import { useTheme } from '@/theme';

/**
 * One appointment in full: what it is, when and where, and what became of it.
 *
 * Every action is permission-aware: the API refuses what the caller may not do,
 * and offering a control that will be refused only wastes their time
 * (plans/phase6.md §17).
 */
export default function AppointmentDetailScreen() {
  const theme = useTheme();
  const { appointmentId } = useLocalSearchParams<{ appointmentId: string }>();

  // Cancelling is deliberately two taps. It is the one action here that cannot
  // be undone, and a single button next to "Edit" is too easy to hit by mistake
  // (plans/phase6.md §20).
  const [confirmingCancel, setConfirmingCancel] = useState(false);

  const appointment = useAppointment(appointmentId ?? null);
  const seniorId = appointment.data?.seniorId ?? null;

  const senior = useSenior(seniorId);
  const members = useCircleMembers(seniorId);
  const cancel = useCancelAppointment(seniorId ?? '');
  const complete = useCompleteAppointment(seniorId ?? '');

  if (appointment.isPending) {
    return (
      <Screen>
        <View style={{ alignItems: 'center', flex: 1, justifyContent: 'center' }}>
          <ActivityIndicator color={theme.colors.primary} />
        </View>
      </Screen>
    );
  }

  if (appointment.isError) {
    return (
      <Screen scrollable>
        <Stack.Screen options={{ headerShown: true, title: 'Appointment' }} />
        <Card>
          <Text variant="sectionHeading">This appointment is not available</Text>
          <Text variant="body" color="secondary">
            {appointment.error instanceof ApiError
              ? appointment.error.message
              : 'It may have been removed, or you may not have access.'}
          </Text>
          <Button variant="secondary" label="Back" onPress={() => router.back()} />
        </Card>
      </Screen>
    );
  }

  const detail = appointment.data;
  const timezone = senior.data?.timezone ?? 'UTC';
  const canManage = senior.data ? can(senior.data, 'appointments.manage') : false;
  const open = detail.status === 'scheduled';

  const memberName = (userId: string | null) =>
    members.data?.find((member) => member.userId === userId)?.displayName ?? 'Somebody';

  const outcome =
    detail.status === 'completed'
      ? `Marked as attended by ${memberName(detail.completedBy)}`
      : detail.status === 'cancelled'
        ? `Cancelled by ${memberName(detail.cancelledBy)}`
        : null;

  const failure = cancel.error ?? complete.error;

  return (
    <Screen scrollable>
      <Stack.Screen options={{ headerShown: true, title: 'Appointment' }} />

      <View style={{ gap: theme.spacing.sm }}>
        <Text variant="pageHeading">{detail.title}</Text>
        {detail.kind ? (
          <Text variant="body" color="secondary">
            {appointmentKindLabel(detail.kind)}
          </Text>
        ) : null}
        {/* Status in words, never colour alone (plans/phase6.md §31). */}
        <Text variant="bodyStrong" color="secondary">
          {appointmentStatusLabel(detail.status)}
        </Text>
      </View>

      <Card>
        <Text variant="sectionHeading">When</Text>
        <Row label="Day" value={appointmentDateLabel(detail, timezone)} />
        <Row label="Time" value={appointmentWhenLabel(detail, timezone)} />
        <Text variant="secondary" color="secondary">
          Shown in {senior.data?.displayName ?? 'this person'}&apos;s timezone.
        </Text>
      </Card>

      {detail.providerName || detail.location ? (
        <Card>
          <Text variant="sectionHeading">Where</Text>
          {detail.providerName ? (
            <Row label="Who they are seeing" value={detail.providerName} />
          ) : null}
          {detail.location ? <Row label="Place" value={detail.location} /> : null}
        </Card>
      ) : null}

      {detail.assignedUserId ? (
        <Card>
          <Text variant="sectionHeading">Who is taking them</Text>
          <Text variant="body">{memberName(detail.assignedUserId)}</Text>
        </Card>
      ) : null}

      {detail.notes ? (
        <Card>
          <Text variant="sectionHeading">Notes</Text>
          <Text variant="body">{detail.notes}</Text>
        </Card>
      ) : null}

      {outcome ? (
        <Card>
          <Text variant="sectionHeading">What happened</Text>
          <Text variant="body">{outcome}</Text>
        </Card>
      ) : null}

      {failure ? (
        <Text variant="secondary" color="danger">
          {failure instanceof ApiError ? failure.message : 'That did not save. Please try again.'}
        </Text>
      ) : null}

      {canManage && open ? (
        <>
          <Button
            variant="secondary"
            label="Edit appointment"
            onPress={() =>
              router.push({
                pathname: '/appointments/[appointmentId]/edit',
                params: { appointmentId: detail.id },
              })
            }
          />

          <Button
            label="Mark as attended"
            loading={complete.isPending}
            onPress={() => complete.mutate({ appointmentId: detail.id })}
          />

          {confirmingCancel ? (
            <Card>
              <Text variant="sectionHeading">Cancel this appointment?</Text>
              <Text variant="body" color="secondary">
                It stays in the record, marked as cancelled. This cannot be undone.
              </Text>
              <Button
                label="Yes, cancel it"
                loading={cancel.isPending}
                onPress={() => {
                  setConfirmingCancel(false);
                  cancel.mutate({ appointmentId: detail.id });
                }}
              />
              <Button
                variant="ghost"
                label="Keep the appointment"
                onPress={() => setConfirmingCancel(false)}
              />
            </Card>
          ) : (
            <Button
              variant="ghost"
              label="Cancel this appointment"
              onPress={() => setConfirmingCancel(true)}
            />
          )}
        </>
      ) : null}
    </Screen>
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
