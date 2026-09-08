import type { CareTask, TaskScope } from '@genxcare/contracts';
import { can } from '@genxcare/contracts';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import { useCallback, useState } from 'react';
import { ActivityIndicator, FlatList, View } from 'react-native';

import { Button, Card, Illustration, Screen, TaskCard, Text } from '@/components/ui';
import { useSenior } from '@/features/seniors/use-seniors';
import { useCompleteTask, useSeniorTasks, useSkipTask } from '@/features/tasks/use-tasks';
import { ApiError } from '@/lib/api-error';
import { useTheme } from '@/theme';

/**
 * Care tasks for one senior.
 *
 * Today comes first because it is what somebody standing with the person needs.
 * The list is virtualised: a long-running circle accumulates a lot of history,
 * and only what is on screen should be rendered (plans/phase4.md §35).
 */

const VIEWS: { scope: TaskScope; label: string }[] = [
  { scope: 'today', label: 'Today' },
  { scope: 'upcoming', label: 'Coming up' },
  { scope: 'overdue', label: 'Missed' },
];

export default function SeniorTasksScreen() {
  const theme = useTheme();
  const { seniorId } = useLocalSearchParams<{ seniorId: string }>();

  // A view choice is transient screen state, so it lives here rather than in a
  // store, and never holds task data (plans/phase4.md §25).
  const [scope, setScope] = useState<TaskScope>('today');

  const senior = useSenior(seniorId ?? null);
  const tasks = useSeniorTasks(seniorId ?? null, scope);
  const complete = useCompleteTask(seniorId ?? '');
  const skip = useSkipTask(seniorId ?? '');

  const timezone = senior.data?.timezone ?? 'UTC';
  const canComplete = senior.data ? can(senior.data, 'tasks.complete') : false;
  const canManage = senior.data ? can(senior.data, 'tasks.manage') : false;

  const renderTask = useCallback(
    ({ item }: { item: CareTask }) => (
      <TaskCard
        task={item}
        timezone={timezone}
        canComplete={canComplete}
        onPress={() => router.push({ pathname: '/tasks/[taskId]', params: { taskId: item.id } })}
        onComplete={() => complete.mutate({ taskId: item.id })}
        onSkip={() => skip.mutate({ taskId: item.id })}
      />
    ),
    [timezone, canComplete, complete, skip],
  );

  if (senior.isPending) {
    return (
      <Screen>
        <View style={{ alignItems: 'center', flex: 1, justifyContent: 'center' }}>
          <ActivityIndicator color={theme.colors.primary} />
        </View>
      </Screen>
    );
  }

  return (
    <Screen>
      <Stack.Screen options={{ headerShown: true, title: 'Care tasks' }} />

      <FlatList
        data={tasks.data ?? []}
        keyExtractor={(task) => task.id}
        renderItem={renderTask}
        contentContainerStyle={{ gap: theme.spacing.md, paddingBottom: theme.spacing.xl }}
        ListHeaderComponent={
          <View style={{ gap: theme.spacing.lg, paddingBottom: theme.spacing.md }}>
            <Text variant="pageHeading">
              {scope === 'today' ? "Today's care" : scope === 'upcoming' ? 'Coming up' : 'Missed'}
            </Text>

            <View
              accessibilityRole="tablist"
              style={{ flexDirection: 'row', gap: theme.spacing.sm }}
            >
              {VIEWS.map((view) => (
                <Button
                  key={view.scope}
                  variant={view.scope === scope ? 'primary' : 'secondary'}
                  label={view.label}
                  accessibilityRole="tab"
                  accessibilityState={{ selected: view.scope === scope }}
                  style={{ flex: 1 }}
                  onPress={() => setScope(view.scope)}
                />
              ))}
            </View>

            {canManage ? (
              <Button
                variant="secondary"
                label="Add a task"
                onPress={() =>
                  router.push({
                    pathname: '/seniors/[seniorId]/tasks/new',
                    params: { seniorId: seniorId ?? '' },
                  })
                }
              />
            ) : null}

            {complete.isError || skip.isError ? (
              <Card>
                <Text variant="bodyStrong" color="danger">
                  That did not save
                </Text>
                <Text variant="body" color="secondary">
                  {errorMessage(complete.error ?? skip.error)}
                </Text>
              </Card>
            ) : null}
          </View>
        }
        ListEmptyComponent={
          tasks.isPending ? (
            <View style={{ alignItems: 'center', padding: theme.spacing.xl }}>
              <ActivityIndicator color={theme.colors.primary} />
            </View>
          ) : tasks.isError ? (
            <Card>
              <Text variant="sectionHeading">We could not load these tasks</Text>
              <Text variant="body" color="secondary">
                {errorMessage(tasks.error)}
              </Text>
              <Button variant="secondary" label="Try again" onPress={() => void tasks.refetch()} />
            </Card>
          ) : (
            <Card>
              <Illustration name="allCaughtUp" height={140} />
              <Text variant="sectionHeading">{emptyHeading(scope)}</Text>
              <Text variant="body" color="secondary">
                {emptyBody(scope)}
              </Text>
            </Card>
          )
        }
        refreshing={tasks.isRefetching}
        onRefresh={() => void tasks.refetch()}
      />
    </Screen>
  );
}

function emptyHeading(scope: TaskScope): string {
  if (scope === 'overdue') return 'Nothing has been missed';
  if (scope === 'upcoming') return 'Nothing scheduled yet';
  return 'Nothing to do today';
}

function emptyBody(scope: TaskScope): string {
  if (scope === 'overdue') return 'Everything due so far has been dealt with.';
  if (scope === 'upcoming') return 'Tasks added for the coming week will appear here.';
  return 'Add a task when there is something to remember.';
}

function errorMessage(error: unknown): string {
  return error instanceof ApiError ? error.message : 'Please try again.';
}
