import type { MedicationSchedule } from '@genxcare/contracts';
import { scheduleLabel } from '@genxcare/contracts';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { ActivityIndicator, View } from 'react-native';

import { Button, Card, Screen, Text, TextField } from '@/components/ui';
import {
  useAddMedicationDose,
  useAddMedicationSchedule,
  useMedication,
  useMedicationSchedules,
  useUpdateMedication,
  useUpdateMedicationSchedule,
} from '@/features/medications/use-medications';
import { useSenior } from '@/features/seniors/use-seniors';
import { ApiError } from '@/lib/api-error';
import { instantFor, todayInIso } from '@/lib/timezone';
import { useTheme } from '@/theme';

/**
 * Edit a medication.
 *
 * The two halves are deliberately separate. Changing what the medicine is
 * rewrites nothing that has happened; changing when it is taken only affects
 * doses that have not fallen due. Neither can reach into the history
 * (plans/phase5.md §12) — the server enforces that, and the wording here says
 * so, because somebody correcting a dosage should not have to wonder.
 */
export default function EditMedicationScreen() {
  const theme = useTheme();
  const { medicationId } = useLocalSearchParams<{ medicationId: string }>();

  const medication = useMedication(medicationId ?? null);
  const seniorId = medication.data?.seniorId ?? null;
  const senior = useSenior(seniorId);
  const schedules = useMedicationSchedules(medicationId ?? null);

  const update = useUpdateMedication(seniorId ?? '');
  const addSchedule = useAddMedicationSchedule(seniorId ?? '', medicationId ?? '');
  const updateSchedule = useUpdateMedicationSchedule(seniorId ?? '', medicationId ?? '');
  const addDose = useAddMedicationDose(seniorId ?? '', medicationId ?? '');

  // Seeded once the medication has loaded; null means "not edited yet", so an
  // in-flight refetch cannot overwrite what somebody is typing.
  const [name, setName] = useState<string | null>(null);
  const [dosage, setDosage] = useState<string | null>(null);
  const [instructions, setInstructions] = useState<string | null>(null);
  const [notes, setNotes] = useState<string | null>(null);

  const [newTime, setNewTime] = useState('');
  const [doseDate, setDoseDate] = useState(todayInIso());
  const [doseTime, setDoseTime] = useState('');

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
        <Stack.Screen options={{ headerShown: true, title: 'Edit' }} />
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
  const fieldErrors = update.error instanceof ApiError ? (update.error.details ?? {}) : {};
  const scheduleErrors =
    addSchedule.error instanceof ApiError ? (addSchedule.error.details ?? {}) : {};

  const live = (schedules.data ?? []).filter((schedule) => schedule.active);

  async function handleSaveDetails() {
    await update.mutateAsync({
      medicationId: detail.id,
      ...(name === null ? {} : { name: name.trim() }),
      ...(dosage === null ? {} : { dosage: dosage.trim() }),
      ...(instructions === null ? {} : { instructions: instructions.trim() }),
      ...(notes === null ? {} : { notes: notes.trim() }),
    });
    router.back();
  }

  async function handleAddTime() {
    // A new time inherits the pattern the medicine is already on, so "add
    // 20:00" means what somebody expects without asking the question twice.
    const pattern = live[0]?.recurrence ?? { frequency: 'daily' as const, weekdays: [] };

    await addSchedule.mutateAsync({ recurrence: pattern, scheduledTime: newTime.trim() });
    setNewTime('');
  }

  async function handleAddDose() {
    await addDose.mutateAsync({
      scheduledFor: instantFor(doseDate, doseTime.trim(), timezone),
    });
    setDoseTime('');
  }

  return (
    <Screen scrollable>
      <Stack.Screen options={{ headerShown: true, title: 'Edit medication' }} />

      <Text variant="pageHeading">{detail.name}</Text>

      <Card>
        <Text variant="sectionHeading">What it is</Text>
        <TextField
          label="Name"
          value={name ?? detail.name}
          onChangeText={setName}
          error={fieldErrors.name}
        />
        <TextField
          label="How much?"
          value={dosage ?? detail.dosage}
          onChangeText={setDosage}
          placeholder="500 mg"
          error={fieldErrors.dosage}
        />
        <TextField
          label="Anything to remember?"
          value={instructions ?? detail.instructions ?? ''}
          onChangeText={setInstructions}
          placeholder="With food"
          multiline
          error={fieldErrors.instructions}
        />
        <TextField
          label="Notes"
          value={notes ?? detail.notes ?? ''}
          onChangeText={setNotes}
          multiline
          error={fieldErrors.notes}
        />

        <Text variant="secondary" color="secondary">
          Doses already recorded keep the name and amount they were taken with.
        </Text>

        <Button label="Save" loading={update.isPending} onPress={handleSaveDetails} />
      </Card>

      <Card>
        <Text variant="sectionHeading">When it is taken</Text>

        {live.length === 0 ? (
          <Text variant="body" color="secondary">
            No times set yet.
          </Text>
        ) : (
          live.map((schedule) => (
            <TimeRow
              key={schedule.id}
              schedule={schedule}
              stopping={updateSchedule.isPending}
              onStop={() => updateSchedule.mutate({ scheduleId: schedule.id, active: false })}
            />
          ))
        )}

        <TextField
          label="Add another time"
          value={newTime}
          onChangeText={setNewTime}
          placeholder="20:00"
          autoCapitalize="none"
          error={scheduleErrors.scheduledTime}
        />
        <Button
          variant="secondary"
          label="Add this time"
          disabled={newTime.trim() === ''}
          loading={addSchedule.isPending}
          onPress={handleAddTime}
        />

        <Text variant="secondary" color="secondary">
          Changing the times only affects doses that have not come round yet.
        </Text>
      </Card>

      <Card>
        <Text variant="sectionHeading">A one-off dose</Text>
        <Text variant="body" color="secondary">
          For a dose outside the usual times. It does not change the schedule.
        </Text>

        <TextField
          label="On which day?"
          value={doseDate}
          onChangeText={setDoseDate}
          placeholder="2026-08-20"
          autoCapitalize="none"
        />
        <TextField
          label="At what time?"
          value={doseTime}
          onChangeText={setDoseTime}
          placeholder="22:00"
          autoCapitalize="none"
        />
        <Button
          variant="secondary"
          label="Add this dose"
          disabled={doseTime.trim() === '' || doseDate.trim() === ''}
          loading={addDose.isPending}
          onPress={handleAddDose}
        />
      </Card>

      {update.isError || addSchedule.isError || updateSchedule.isError || addDose.isError ? (
        <Text variant="secondary" color="danger">
          {firstMessage(update.error, addSchedule.error, updateSchedule.error, addDose.error)}
        </Text>
      ) : null}

      <Button variant="ghost" label="Done" onPress={() => router.back()} />
    </Screen>
  );
}

function TimeRow({
  schedule,
  stopping,
  onStop,
}: {
  schedule: MedicationSchedule;
  stopping: boolean;
  onStop: () => void;
}) {
  const theme = useTheme();

  return (
    <View style={{ gap: theme.spacing.xs }}>
      <Text variant="bodyStrong">{scheduleLabel(schedule)}</Text>
      <Button variant="ghost" label="Stop this time" loading={stopping} onPress={onStop} />
    </View>
  );
}

function firstMessage(...errors: unknown[]): string {
  for (const error of errors) {
    if (error instanceof ApiError) return error.message;
    if (error) return 'That did not save. Please try again.';
  }
  return 'That did not save. Please try again.';
}
