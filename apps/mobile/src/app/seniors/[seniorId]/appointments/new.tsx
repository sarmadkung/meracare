import type { AppointmentKind, CreateAppointmentRequest } from '@genxcare/contracts';
import { SELECTABLE_APPOINTMENT_KINDS } from '@genxcare/contracts';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { Pressable, View } from 'react-native';

import { Button, Card, Screen, Text, TextField } from '@/components/ui';
import { useCreateAppointment } from '@/features/appointments/use-appointments';
import { useSenior } from '@/features/seniors/use-seniors';
import { ApiError } from '@/lib/api-error';
import { instantFor, todayInIso } from '@/lib/timezone';
import { useTheme } from '@/theme';

/**
 * Book an appointment.
 *
 * What, then when, then where — the order somebody reading a hospital letter
 * would think about it in (plans/phase6.md §7). Nothing here suggests an
 * appointment or judges one: the app records what the family arranged
 * (plans/phase6.md §28).
 *
 * The date and time are entered as text and turned into an instant in the
 * senior's own timezone, so "09:30" means half past nine where they live rather
 * than where the phone is (plans/phase6.md §4). Native pickers remain deferred
 * across the app; this matches the care-task and medication forms rather than
 * introducing a third way to enter a time.
 */
export default function NewAppointmentScreen() {
  const theme = useTheme();
  const { seniorId } = useLocalSearchParams<{ seniorId: string }>();

  const senior = useSenior(seniorId ?? null);
  const createAppointment = useCreateAppointment(seniorId ?? '');

  const [title, setTitle] = useState('');
  const [kind, setKind] = useState<AppointmentKind | null>(null);
  const [date, setDate] = useState(todayInIso());
  const [time, setTime] = useState('09:30');
  const [endTime, setEndTime] = useState('');
  const [provider, setProvider] = useState('');
  const [location, setLocation] = useState('');
  const [notes, setNotes] = useState('');

  const timezone = senior.data?.timezone ?? 'UTC';
  const who = senior.data?.displayName ?? 'this person';

  const fieldErrors =
    createAppointment.error instanceof ApiError ? (createAppointment.error.details ?? {}) : {};

  // Caught here as well as by the server, so somebody who mistypes an end time
  // is told before the request goes rather than after it comes back.
  const endsTooEarly = endTime.trim() !== '' && endTime.trim() <= time.trim();

  const incomplete =
    title.trim().length === 0 || date.trim() === '' || time.trim() === '' || endsTooEarly;

  async function handleCreate() {
    const body: CreateAppointmentRequest = {
      title: title.trim(),
      kind,
      providerName: provider.trim(),
      location: location.trim(),
      notes: notes.trim(),
      scheduledAt: instantFor(date.trim(), time.trim(), timezone),
      endsAt: endTime.trim() === '' ? null : instantFor(date.trim(), endTime.trim(), timezone),
    };

    await createAppointment.mutateAsync(body);
    router.back();
  }

  return (
    <Screen scrollable>
      <Stack.Screen options={{ headerShown: true, title: 'Add an appointment' }} />

      <View style={{ gap: theme.spacing.sm }}>
        <Text variant="pageHeading">Add an appointment</Text>
        <Text variant="body" color="secondary">
          Somewhere {who} needs to be.
        </Text>
      </View>

      <Card>
        <TextField
          label="What is it for?"
          value={title}
          onChangeText={setTitle}
          placeholder="Cardiology review"
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
          placeholder="2026-08-20"
          autoCapitalize="none"
          error={fieldErrors.scheduledAt}
        />
        <TextField
          label="At what time?"
          value={time}
          onChangeText={setTime}
          placeholder="09:30"
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
          placeholder="Dr Ahmed"
          error={fieldErrors.providerName}
        />
        <TextField
          label="Where is it? (optional)"
          value={location}
          onChangeText={setLocation}
          placeholder="City Hospital"
          error={fieldErrors.location}
        />
        <TextField
          label="Anything to remember? (optional)"
          value={notes}
          onChangeText={setNotes}
          placeholder="Bring the last blood test"
          multiline
          error={fieldErrors.notes}
        />
      </Card>

      {createAppointment.isError && Object.keys(fieldErrors).length === 0 ? (
        <Text variant="secondary" color="danger">
          {createAppointment.error instanceof ApiError
            ? createAppointment.error.message
            : 'We could not add that appointment.'}
        </Text>
      ) : null}

      <Button
        label="Add appointment"
        onPress={handleCreate}
        disabled={incomplete}
        loading={createAppointment.isPending}
      />
      <Button variant="ghost" label="Cancel" onPress={() => router.back()} />
    </Screen>
  );
}
