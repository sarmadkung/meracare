import type { Senior } from '@meracare/contracts';
import { statusLabel, taskTimeLabel } from '@meracare/contracts';
import { Redirect, Stack } from 'expo-router';
import { View } from 'react-native';

import { EmptyState, ListRow, Screen, SectionHeader, Text } from '@/components/ui';
import { useSession } from '@/features/auth/session-provider';
import { useSeniors } from '@/features/seniors/use-seniors';
import { useOfflineSync } from '@/features/sync/use-sync';
import { useMyTasks } from '@/features/tasks/use-tasks';
import { useTheme } from '@/theme';

/**
 * Today (docs/13-mvp-screen-map.md, screen 9).
 *
 * The caller's own work, across every circle they belong to — what a
 * professional caregiver opens the app for, in order, without first having to
 * pick which client they are looking at.
 *
 * It no longer lists people: that is the Circle tab. Today answers "what do I
 * have to do", Circle answers "who am I looking after", and keeping the two
 * apart is what stops a caregiver with six clients scrolling through six days
 * of care to find their own round (plans/phase9.md §8).
 */
export default function HomeScreen() {
  const theme = useTheme();
  const { isSignedIn, isRestoring } = useSession();
  const seniors = useSeniors(isSignedIn);
  const myTasks = useMyTasks();

  // Anything recorded while offline is sent as soon as the app is usable.
  useOfflineSync();

  if (!isRestoring && !isSignedIn) {
    return <Redirect href="/sign-in" />;
  }

  const tasks = myTasks.data ?? [];
  const people = seniors.data ?? [];

  return (
    <Screen scrollable variant="list">
      <Stack.Screen options={{ headerShown: false }} />

      <View style={{ gap: theme.spacing.xs, marginBottom: theme.spacing.sm }}>
        <Text variant="label" color="secondary">
          {new Date().toLocaleDateString(undefined, {
            weekday: 'long',
            day: 'numeric',
            month: 'long',
          })}
        </Text>
        <Text accessibilityRole="header" variant="pageHeading">
          Today
        </Text>
      </View>

      {tasks.length === 0 ? (
        <EmptyState
          illustration="allCaughtUp"
          title="Nothing needs you"
          body="When something is due for anyone in your circle, it will appear here."
        />
      ) : (
        <>
          <SectionHeader title="Yours to do" />

          {tasks.slice(0, 5).map((task) => (
            <ListRow
              key={task.id}
              title={task.title}
              subtitle={`${taskTimeLabel(task, timezoneFor(people, task.seniorId))} · ${statusLabel(
                task.status,
              )}`}
              href={{ pathname: '/tasks/[taskId]', params: { taskId: task.id } }}
            />
          ))}
        </>
      )}
    </Screen>
  );
}

/**
 * This list spans circles, so each row is read in its own senior's timezone
 * rather than one zone for the whole screen.
 */
function timezoneFor(people: Senior[], seniorId: string): string {
  return people.find((person) => person.id === seniorId)?.timezone ?? 'UTC';
}
