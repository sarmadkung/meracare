import {
  SELECTABLE_APPOINTMENT_KINDS,
  type AppointmentKind,
  type UpdateAppointmentRequest,
} from '@genxcare/contracts';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { ActivityIndicator, Pressable, View } from 'react-native';

import { Button, Card, Screen, Text, TextField } from '@/components/ui';
import { useAppointment, useUpdateAppointment } from '@/features/appointments/use-appointments';
import { useSenior } from '@/features/seniors/use-seniors';
import { ApiError } from '@/lib/api-error';
import { instantFor, offsetAt } from '@/lib/timezone';
import { useTheme } from '@/theme';

/**
 * Edit an appointment.
 *
 * Every field starts at what is already stored, so an edit that only moves the
 * time cannot blank the address by accident (plans/phase6.md §19). The server
 * refuses an edit to an appointment that has already been settled and answers
 * 409; the message it sends is shown as it is.
 */
export default function EditAppointmentScreen() {
  const { appointmentId } = useLocalSearchParams<{ appointmentId: string }>();
  const appointment = useAppointment(appointmentId ?? null);
  const theme = useTheme();

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
        <Stack.Screen options={{ headerShown: true, title: 'Edit appointment' }} />
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

  // Remounted per appointment so the form's initial values come from real data
  // rather than from an effect that has to keep them in step.
  return <EditForm appointment={appointment.data} />;
}

function EditForm({
  appointment,
}: {
  appointment: NonNullable<ReturnType<typeof useAppointment>['data']>;
}) {
  const theme = useTheme();

  const senior = useSenior(appointment.seniorId);
  const update = useUpdateAppointment(appointment.seniorId);

  const timezone = senior.data?.timezone ?? 'UTC';
  const who = senior.data?.displayName ?? 'this person';

  const [title, setTitle] = useState(appointment.title);
  const [kind, setKind] = useState<AppointmentKind | null>(appointment.kind);
  const [date, setDate] = useState(localDate(appointment.scheduledAt, timezone));
  const [time, setTime] = useState(localTime(appointment.scheduledAt, timezone));
  const [endTime, setEndTime] = useState(
    appointment.endsAt === null ? '' : localTime(appointment.endsAt, timezone),
  );
  const [provider, setProvider] = useState(appointment.providerName ?? '');
  const [location, setLocation] = useState(appointment.location ?? '');
  const [notes, setNotes] = useState(appointment.notes ?? '');

  const fieldErrors = update.error instanceof ApiError ? (update.error.details ?? {}) : {};

  const endsTooEarly = endTime.trim() !== '' && endTime.trim() <= time.trim();
  const incomplete =
    title.trim().length === 0 || date.trim() === '' || time.trim() === '' || endsTooEarly;

  async function handleSave() {
    const body: UpdateAppointmentRequest & { appointmentId: string } = {
      appointmentId: appointment.id,
      title: title.trim(),
      providerName: provider.trim(),
      location: location.trim(),
      notes: notes.trim(),
      scheduledAt: instantFor(date.trim(), time.trim(), timezone),
    };

    // A cleared field has to say so: an absent one means "leave it alone", which
    // is not the same thing (plans/phase6.md §19).
    if (kind === null) {
      body.clearKind = true;
    } else {
      body.kind = kind;
    }

    if (endTime.trim() === '') {
      body.clearEndsAt = true;
    } else {
      body.endsAt = instantFor(date.trim(), endTime.trim(), timezone);
    }

    await update.mutateAsync(body);
    router.back();
  }

  return (
    <Screen scrollable>
      <Stack.Screen options={{ headerShown: true, title: 'Edit appointment' }} />

      <View style={{ gap: theme.spacing.sm }}>
        <Text variant="pageHeading">Edit appointment</Text>
        <Text variant="body" color="secondary">
          Change what is booked for {who}.
        </Text>
      </View>

      <Card>
        <TextField
          label="What is it for?"
          value={title}
          onChangeText={setTitle}
          error={fieldErrors.title}
        />
      </Card>

      <View style={{ gap: theme.spacing.md }}>
        <Text variant="sectionHeading">What kind of appointment?</Text>
        <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: theme.spacing.sm }}>
          {SELECTABLE_APPOINTMENT_KINDS.map((option) => {
            const selected = kind === option.kind;
            return (
              <Pressable
                key={option.kind}
                accessibilityRole="radio"
                accessibilityState={{ selected }}
                accessibilityLabel={option.label}
                onPress={() => setKind(selected ? null : option.kind)}
                style={{
                  alignItems: 'center',
                  backgroundColor: selected ? theme.colors.primary : theme.colors.surfaceMuted,
                  borderRadius: theme.radii.md,
                  justifyContent: 'center',
                  minHeight: 48,
                  paddingHorizontal: theme.spacing.lg,
                }}
              >
                <Text
                  variant="bodyStrong"
                  style={{ color: selected ? theme.colors.onPrimary : theme.colors.textPrimary }}
                >
                  {option.label}
                </Text>
              </Pressable>
            );
          })}
        </View>
      </View>

      <Card>
        <TextField
          label="On what day?"
          value={date}
          onChangeText={setDate}
          autoCapitalize="none"
          error={fieldErrors.scheduledAt}
        />
        <TextField
          label="At what time?"
          value={time}
          onChangeText={setTime}
          autoCapitalize="none"
        />
        <TextField
          label="Until when? (optional)"
          value={endTime}
          onChangeText={setEndTime}
          placeholder="10:15"
          autoCapitalize="none"
          error={endsTooEarly ? 'It has to end after it starts.' : fieldErrors.endsAt}
        />
        <Text variant="secondary" color="secondary">
          Times are in {who}&apos;s own timezone.
        </Text>
      </Card>

      <Card>
        <TextField
          label="Who are they seeing? (optional)"
          value={provider}
          onChangeText={setProvider}
          error={fieldErrors.providerName}
        />
        <TextField
          label="Where is it? (optional)"
          value={location}
          onChangeText={setLocation}
          error={fieldErrors.location}
        />
        <TextField
          label="Anything to remember? (optional)"
          value={notes}
          onChangeText={setNotes}
          multiline
          error={fieldErrors.notes}
        />
      </Card>

      {update.isError && Object.keys(fieldErrors).length === 0 ? (
        <Text variant="secondary" color="danger">
          {update.error instanceof ApiError
            ? update.error.message
            : 'We could not save that change.'}
        </Text>
      ) : null}

      <Button
        label="Save changes"
        onPress={handleSave}
        disabled={incomplete}
        loading={update.isPending}
      />
      <Button variant="ghost" label="Cancel" onPress={() => router.back()} />
    </Screen>
  );
}

/**
 * The stored instant as the date and time fields need it: the senior's own
 * wall clock, not the device's.
 *
 * Shifting the instant by the zone's offset and then reading the UTC fields
 * gives the local calendar date and clock time without a date library, using
 * the same offset calculation the create form relies on.
 */
function localParts(instant: string, timezone: string): string {
  const at = new Date(instant);
  return new Date(at.getTime() + offsetAt(at, timezone)).toISOString();
}

function localDate(instant: string, timezone: string): string {
  return localParts(instant, timezone).slice(0, 10);
}

function localTime(instant: string, timezone: string): string {
  return localParts(instant, timezone).slice(11, 16);
}
