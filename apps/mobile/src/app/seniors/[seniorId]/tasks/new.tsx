import type { CreateTaskRequest, TaskRecurrence, Weekday } from '@genxcare/contracts';
import {
  RECURRENCE_PRESETS,
  SELECTABLE_WEEKDAYS,
  recurrenceLabel,
  roleLabel,
} from '@genxcare/contracts';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { Pressable, View } from 'react-native';

import { Button, Card, OptionCard, Screen, Text, TextField } from '@/components/ui';
import { useCircleMembers } from '@/features/circle/use-circle';
import { useSenior } from '@/features/seniors/use-seniors';
import { useCreateTask } from '@/features/tasks/use-tasks';
import { ApiError } from '@/lib/api-error';
import { instantFor, todayInIso } from '@/lib/timezone';
import { useTheme } from '@/theme';

/**
 * Create a care task.
 *
 * Title, then when, then who — the order somebody would think about it in. The
 * repeat options are worded ("Every weekday"), never expressed as the rule the
 * server stores (plans/phase4.md §21).
 */
export default function NewTaskScreen() {
  const theme = useTheme();
  const { seniorId } = useLocalSearchParams<{ seniorId: string }>();

  const senior = useSenior(seniorId ?? null);
  const members = useCircleMembers(seniorId ?? null);
  const createTask = useCreateTask(seniorId ?? '');

  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [preset, setPreset] = useState('once');
  const [weekdays, setWeekdays] = useState<Weekday[]>([]);
  const [dueTime, setDueTime] = useState('09:00');
  const [dueDate, setDueDate] = useState(todayInIso());
  const [assignee, setAssignee] = useState<string | null>(null);

  const chosen = RECURRENCE_PRESETS.find((option) => option.key === preset);
  const repeats = chosen?.recurrence !== null && chosen !== undefined;
  const custom = preset === 'custom';

  const fieldErrors = createTask.error instanceof ApiError ? (createTask.error.details ?? {}) : {};

  const recurrence: TaskRecurrence | null = !repeats
    ? null
    : custom
      ? { frequency: 'weekly', weekdays }
      : (chosen?.recurrence ?? null);

  const incomplete =
    title.trim().length === 0 || (custom && weekdays.length === 0) || (!repeats && dueDate === '');

  function toggleWeekday(weekday: Weekday) {
    setWeekdays((current) =>
      current.includes(weekday) ? current.filter((day) => day !== weekday) : [...current, weekday],
    );
  }

  async function handleCreate() {
    const body: CreateTaskRequest = {
      title: title.trim(),
      description: description.trim() === '' ? null : description.trim(),
      assignedUserId: assignee,
    };

    if (recurrence !== null) {
      body.recurrence = recurrence;
      body.dueTime = dueTime;
    } else {
      // A one-time task is an instant. The date and time the user chose are
      // read in the senior's timezone, not the device's, so a task set for
      // "tomorrow at 09:00" falls on their morning.
      body.scheduledFor = instantFor(dueDate, dueTime, senior.data?.timezone ?? 'UTC');
    }

    await createTask.mutateAsync(body);
    router.back();
  }

  return (
    <Screen scrollable>
      <Stack.Screen options={{ headerShown: true, title: 'Add a task' }} />

      <View style={{ gap: theme.spacing.sm }}>
        <Text variant="pageHeading">Add a task</Text>
        <Text variant="body" color="secondary">
          Something to be done for {senior.data?.displayName ?? 'this person'}.
        </Text>
      </View>

      <Card>
        <TextField
          label="What needs doing?"
          value={title}
          onChangeText={setTitle}
          placeholder="Take a 20-minute walk"
          error={fieldErrors.title}
        />
        <TextField
          label="Any details? (optional)"
          value={description}
          onChangeText={setDescription}
          placeholder="Around the park, if the weather is good"
          multiline
          error={fieldErrors.description}
        />
      </Card>

      <View style={{ gap: theme.spacing.md }}>
        <Text variant="sectionHeading">How often?</Text>
        {RECURRENCE_PRESETS.map((option) => (
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
        {!repeats ? (
          <TextField
            label="On which day?"
            value={dueDate}
            onChangeText={setDueDate}
            placeholder="2026-08-20"
            autoCapitalize="none"
            error={fieldErrors.scheduledFor}
          />
        ) : null}

        <TextField
          label="At what time?"
          value={dueTime}
          onChangeText={setDueTime}
          placeholder="09:00"
          autoCapitalize="none"
          error={fieldErrors.dueTime}
        />
        <Text variant="secondary" color="secondary">
          Times are in {senior.data?.displayName ?? 'this person'}&apos;s own timezone.
        </Text>
      </Card>

      <View style={{ gap: theme.spacing.md }}>
        <Text variant="sectionHeading">Who is it for?</Text>
        <OptionCard
          title="Anyone"
          description="Whoever is available can do it."
          selected={assignee === null}
          onPress={() => setAssignee(null)}
        />
        {(members.data ?? []).map((member) => (
          <OptionCard
            key={member.id}
            title={member.displayName}
            description={member.isSelf ? 'You' : roleLabel(member.role)}
            selected={assignee === member.userId}
            onPress={() => setAssignee(member.userId)}
          />
        ))}
      </View>

      {createTask.isError && Object.keys(fieldErrors).length === 0 ? (
        <Text variant="secondary" color="danger">
          {createTask.error instanceof ApiError
            ? createTask.error.message
            : 'We could not add that task.'}
        </Text>
      ) : null}

      <Button
        label="Add task"
        onPress={handleCreate}
        disabled={incomplete}
        loading={createTask.isPending}
      />
      <Button variant="ghost" label="Cancel" onPress={() => router.back()} />
    </Screen>
  );
}
