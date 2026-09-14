import type {
  CreateMedicationRequest,
  MedicationForm,
  MedicationScheduleInput,
  Weekday,
} from '@genxcare/contracts';
import {
  MEDICATION_RECURRENCE_PRESETS,
  SELECTABLE_FORMS,
  SELECTABLE_WEEKDAYS,
  recurrenceLabel,
} from '@genxcare/contracts';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { Pressable, View } from 'react-native';

import { Button, Card, OptionCard, Screen, Text, TextField } from '@/components/ui';
import { useCreateMedication } from '@/features/medications/use-medications';
import { useSenior } from '@/features/seniors/use-seniors';
import { ApiError } from '@/lib/api-error';
import { useTheme } from '@/theme';

/**
 * Add a medication.
 *
 * Name, then dose, then when — the order somebody reading a label would think
 * about it in (plans/phase5.md §11). The repeat options are worded ("Every
 * weekday"), never expressed as the rule the server stores.
 *
 * Nothing here suggests a dose or judges a medicine: the app records what the
 * family was told (plans/phase5.md §32).
 */
export default function NewMedicationScreen() {
  const theme = useTheme();
  const { seniorId } = useLocalSearchParams<{ seniorId: string }>();

  const senior = useSenior(seniorId ?? null);
  const createMedication = useCreateMedication(seniorId ?? '');

  const [name, setName] = useState('');
  const [dosage, setDosage] = useState('');
  const [form, setForm] = useState<MedicationForm | null>(null);
  const [instructions, setInstructions] = useState('');

  const [preset, setPreset] = useState('daily');
  const [weekdays, setWeekdays] = useState<Weekday[]>([]);
  // Most medicines are taken once a day; a second time is offered rather than
  // assumed, so the common case stays one field.
  const [firstTime, setFirstTime] = useState('08:00');
  const [secondTime, setSecondTime] = useState('');

  const chosen = MEDICATION_RECURRENCE_PRESETS.find((option) => option.key === preset);
  const custom = preset === 'custom';

  const fieldErrors =
    createMedication.error instanceof ApiError ? (createMedication.error.details ?? {}) : {};

  const recurrence = custom
    ? ({ frequency: 'weekly', weekdays } as const)
    : (chosen?.recurrence ?? { frequency: 'daily' as const, weekdays: [] });

  const incomplete =
    name.trim().length === 0 || (custom && weekdays.length === 0) || firstTime.trim() === '';

  function toggleWeekday(weekday: Weekday) {
    setWeekdays((current) =>
      current.includes(weekday) ? current.filter((day) => day !== weekday) : [...current, weekday],
    );
  }

  async function handleCreate() {
    // Two times of day are two schedules, which is what lets the evening dose
    // be stopped later without touching the morning one.
    const schedules: MedicationScheduleInput[] = [{ recurrence, scheduledTime: firstTime.trim() }];
    if (secondTime.trim() !== '') {
      schedules.push({ recurrence, scheduledTime: secondTime.trim() });
    }

    const body: CreateMedicationRequest = {
      name: name.trim(),
      dosage: dosage.trim(),
      form,
      instructions: instructions.trim(),
      schedules,
    };

    await createMedication.mutateAsync(body);
    router.back();
  }

  return (
    <Screen scrollable>
      <Stack.Screen options={{ headerShown: true, title: 'Add a medication' }} />

      <View style={{ gap: theme.spacing.sm }}>
        <Text variant="pageHeading">Add a medication</Text>
        <Text variant="body" color="secondary">
          Something {senior.data?.displayName ?? 'this person'} takes.
        </Text>
      </View>

      <Card>
        <TextField
          label="What is it called?"
          value={name}
          onChangeText={setName}
          placeholder="Metformin"
          error={fieldErrors.name}
        />
        <TextField
          label="How much? (optional)"
          value={dosage}
          onChangeText={setDosage}
          placeholder="500 mg"
          error={fieldErrors.dosage}
        />
        <TextField
          label="Anything to remember? (optional)"
          value={instructions}
          onChangeText={setInstructions}
          placeholder="With food"
          multiline
          error={fieldErrors.instructions}
        />
      </Card>

      <View style={{ gap: theme.spacing.md }}>
        <Text variant="sectionHeading">What kind is it?</Text>
        <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: theme.spacing.sm }}>
          {SELECTABLE_FORMS.map((option) => {
            const selected = form === option.form;
            return (
              <Pressable
                key={option.form}
                accessibilityRole="radio"
                accessibilityState={{ selected }}
                accessibilityLabel={option.label}
                onPress={() => setForm(selected ? null : option.form)}
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

      <View style={{ gap: theme.spacing.md }}>
        <Text variant="sectionHeading">How often?</Text>
        {MEDICATION_RECURRENCE_PRESETS.map((option) => (
          <OptionCard
            key={option.key}
            title={option.label}
            description={option.description}
            selected={option.key === preset}
            onPress={() => setPreset(option.key)}
          />
        ))}
      </View>

      {custom ? (
        <Card>
          <Text variant="bodyStrong">Which days?</Text>
          <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: theme.spacing.sm }}>
            {SELECTABLE_WEEKDAYS.map((day) => {
              const selected = weekdays.includes(day.weekday);
              return (
                <Pressable
                  key={day.weekday}
                  accessibilityRole="checkbox"
                  accessibilityState={{ checked: selected }}
                  accessibilityLabel={day.label}
                  onPress={() => toggleWeekday(day.weekday)}
                  style={{
                    alignItems: 'center',
                    backgroundColor: selected ? theme.colors.primary : theme.colors.surfaceMuted,
                    borderRadius: theme.radii.md,
                    justifyContent: 'center',
                    minHeight: 48,
                    minWidth: 64,
                    paddingHorizontal: theme.spacing.md,
                  }}
                >
                  <Text
                    variant="bodyStrong"
                    style={{ color: selected ? theme.colors.onPrimary : theme.colors.textPrimary }}
                  >
                    {day.short}
                  </Text>
                </Pressable>
              );
            })}
          </View>
          {weekdays.length > 0 ? (
            <Text variant="secondary" color="secondary">
              {recurrenceLabel({ frequency: 'weekly', weekdays })}
            </Text>
          ) : null}
        </Card>
      ) : null}

      <Card>
        <TextField
          label="At what time?"
          value={firstTime}
          onChangeText={setFirstTime}
          placeholder="08:00"
          autoCapitalize="none"
          error={fieldErrors.scheduledTime ?? fieldErrors['schedules.0.scheduledTime']}
        />
        <TextField
          label="And a second time? (optional)"
          value={secondTime}
          onChangeText={setSecondTime}
          placeholder="20:00"
          autoCapitalize="none"
          error={fieldErrors['schedules.1.scheduledTime']}
        />
        <Text variant="secondary" color="secondary">
          Times are in {senior.data?.displayName ?? 'this person'}&apos;s own timezone.
        </Text>
      </Card>

      {createMedication.isError && Object.keys(fieldErrors).length === 0 ? (
        <Text variant="secondary" color="danger">
          {createMedication.error instanceof ApiError
            ? createMedication.error.message
            : 'We could not add that medication.'}
        </Text>
      ) : null}

      <Button
        label="Add medication"
        onPress={handleCreate}
        disabled={incomplete}
        loading={createMedication.isPending}
      />
      <Button variant="ghost" label="Cancel" onPress={() => router.back()} />
    </Screen>
  );
}
