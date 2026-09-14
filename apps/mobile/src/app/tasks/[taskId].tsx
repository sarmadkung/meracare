import {
  can,
  isOpen,
  recurrenceLabel,
  statusLabel,
  taskDateLabel,
  taskTimeLabel,
} from '@genxcare/contracts';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import { ActivityIndicator, View } from 'react-native';

import { Button, Card, Screen, Text } from '@/components/ui';
import { useCircleMembers } from '@/features/circle/use-circle';
import { useSenior } from '@/features/seniors/use-seniors';
import {
  useCancelTask,
  useCompleteTask,
  useSkipTask,
  useTask,
  useTaskTemplates,
} from '@/features/tasks/use-tasks';
import { ApiError } from '@/lib/api-error';
import { useTheme } from '@/theme';

/**
 * One task in full.
 *
 * Every action is permission-aware: the API refuses what the caller may not do,
 * and offering a control that will be refused only wastes their time
 * (plans/phase4.md §22).
 */
export default function TaskDetailScreen() {
  const theme = useTheme();
  const { taskId } = useLocalSearchParams<{ taskId: string }>();

  const task = useTask(taskId ?? null);
  const seniorId = task.data?.seniorId ?? null;

  const senior = useSenior(seniorId);
  const members = useCircleMembers(seniorId);
  const templates = useTaskTemplates(seniorId);

  const complete = useCompleteTask(seniorId ?? '');
  const skip = useSkipTask(seniorId ?? '');
  const cancel = useCancelTask(seniorId ?? '');

  if (task.isPending || senior.isPending) {
    return (
      <Screen>
        <View style={{ alignItems: 'center', flex: 1, justifyContent: 'center' }}>
          <ActivityIndicator color={theme.colors.primary} />
        </View>
      </Screen>
    );
  }

  if (task.isError) {
    return (
      <Screen scrollable>
        <Stack.Screen options={{ headerShown: true, title: 'Task' }} />
        <Card>
          <Text variant="sectionHeading">This task is not available</Text>
          <Text variant="body" color="secondary">
            {task.error instanceof ApiError
              ? task.error.message
              : 'It may have been removed, or you may not have access.'}
          </Text>
          <Button variant="secondary" label="Back" onPress={() => router.back()} />
        </Card>
      </Screen>
    );
  }

  const detail = task.data;
  const timezone = senior.data?.timezone ?? 'UTC';
  const template = templates.data?.find((entry) => entry.id === detail.templateId);

  const assignee = members.data?.find((member) => member.userId === detail.assignedUserId);
  const actorName = (userId: string | null) =>
    members.data?.find((member) => member.userId === userId)?.displayName ?? 'Somebody';

  const canComplete = senior.data ? can(senior.data, 'tasks.complete') : false;
  const canManage = senior.data ? can(senior.data, 'tasks.manage') : false;
  const open = isOpen(detail);

  const failure = complete.error ?? skip.error ?? cancel.error;

  return (
    <Screen scrollable>
      <Stack.Screen options={{ headerShown: true, title: 'Task' }} />

      <View style={{ gap: theme.spacing.sm }}>
        <Text variant="pageHeading">{detail.title}</Text>
        <Text variant="body" color="secondary">
          {statusLabel(detail.status)}
        </Text>
      </View>

      {detail.description ? (
        <Card>
          <Text variant="sectionHeading">Details</Text>
          <Text variant="body">{detail.description}</Text>
        </Card>
      ) : null}

      <Card>
        <Text variant="sectionHeading">When</Text>
        <Row label="Date" value={taskDateLabel(detail, timezone)} />
        <Row label="Time" value={taskTimeLabel(detail, timezone)} />
        <Row
          label="Repeats"
          value={template ? recurrenceLabel(template.recurrence) : 'Just this once'}
        />
        <Text variant="secondary" color="secondary">
          Shown in {senior.data?.displayName ?? 'this person'}&apos;s timezone.
        </Text>
      </Card>

      <Card>
        <Text variant="sectionHeading">Who</Text>
        <Row label="For" value={assignee?.displayName ?? 'Anyone who is available'} />

        {detail.completedAt ? (
          <Row
            label="Done by"
            value={`${actorName(detail.completedBy)}, ${taskDateLabel(
              { scheduledFor: detail.completedAt },
              timezone,
            )}`}
          />
        ) : null}

        {detail.skippedAt ? (
          <Row
            label="Skipped by"
            value={`${actorName(detail.skippedBy)}, ${taskDateLabel(
              { scheduledFor: detail.skippedAt },
              timezone,
            )}`}
          />
        ) : null}

        {detail.notes ? <Row label="Note" value={detail.notes} /> : null}
      </Card>

      {failure ? (
        <Text variant="secondary" color="danger">
          {failure instanceof ApiError ? failure.message : 'That did not save. Please try again.'}
        </Text>
      ) : null}

      {open && canComplete ? (
        <View style={{ gap: theme.spacing.md }}>
          <Button
            label="Mark as done"
            loading={complete.isPending}
            onPress={() => complete.mutate({ taskId: detail.id })}
          />
          <Button
            variant="secondary"
            label="Not this time"
            loading={skip.isPending}
            onPress={() => skip.mutate({ taskId: detail.id })}
          />
        </View>
      ) : null}

      {open && canManage ? (
        <Button
          variant="ghost"
          label="Cancel this task"
          loading={cancel.isPending}
          onPress={() => cancel.mutate(detail.id)}
        />
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
