import type { CareTask, Senior } from '@meracare/contracts';
import { statusLabel, taskTimeLabel } from '@meracare/contracts';
import { Link, Redirect, router } from 'expo-router';
import { ActivityIndicator, Pressable, StyleSheet, View } from 'react-native';

import { Button, Card, Screen, Text } from '@/components/ui';
import { useSession } from '@/features/auth/session-provider';
import { useAuthActions } from '@/features/auth/use-auth-actions';
import { useUnreadCount } from '@/features/notifications/use-notifications';
import { useSeniors } from '@/features/seniors/use-seniors';
import { useOfflineSync } from '@/features/sync/use-sync';
import { useMyTasks } from '@/features/tasks/use-tasks';
import { ApiError } from '@/lib/api-error';
import { useTheme } from '@/theme';

/**
 * Home / Today.
 *
 * One screen for every care mode: what the user sees is decided by their
 * relationships, not by a separate app (docs/13-mvp-screen-map.md).
 *
 * It stays a list of people plus the caller's own work. Today's medication,
 * tasks and appointments belong to a particular person, so they live on that
 * person's dashboard; repeating them here would mean a caregiver with six
 * clients scrolling through six days of care to find their own round
 * (plans/phase9.md §8).
 */
export default function HomeScreen() {
  const theme = useTheme();
  const { isSignedIn, isRestoring } = useSession();
  const { signOut, isSubmitting } = useAuthActions();
  const seniors = useSeniors(isSignedIn);
  const myTasks = useMyTasks();
  // Read off the inbox itself, so the badge and the list cannot disagree
  // (plans/phase11.md §61).
  const unread = useUnreadCount(isSignedIn);

  // Anything recorded while offline is sent as soon as the app is usable.
  useOfflineSync();

  if (!isRestoring && !isSignedIn) {
    return <Redirect href="/sign-in" />;
  }

  return (
    <Screen scrollable>
      <View
        style={{
          alignItems: 'center',
          flexDirection: 'row',
          gap: theme.spacing.md,
          justifyContent: 'space-between',
        }}
      >
        <Text variant="pageHeading">Today</Text>
        <NotificationsButton unread={unread} />
      </View>

      {(myTasks.data ?? []).length > 0 ? (
        <AssignedToMe tasks={myTasks.data ?? []} seniors={seniors.data ?? []} />
      ) : null}

      {seniors.isPending ? (
        <View
          style={{
            alignItems: 'center',
            gap: theme.spacing.md,
            paddingVertical: theme.spacing.xxl,
          }}
        >
          <ActivityIndicator color={theme.colors.primary} />
          <Text variant="secondary" color="secondary">
            Loading your care circle…
          </Text>
        </View>
      ) : seniors.isError ? (
        <Card>
          <Text variant="bodyStrong">We could not load your seniors</Text>
          <Text variant="secondary" color="secondary">
            {seniors.error instanceof ApiError
              ? seniors.error.message
              : 'Something went wrong. Please try again.'}
          </Text>
          <Button variant="secondary" label="Try again" onPress={() => void seniors.refetch()} />
        </Card>
      ) : seniors.data.length === 0 ? (
        <EmptyState />
      ) : (
        <View style={{ gap: theme.spacing.md }}>
          {seniors.data.map((senior) => (
            <SeniorRow key={senior.id} senior={senior} />
          ))}

          <Button
            variant="secondary"
            label="Add another person"
            onPress={() => router.push('/onboarding')}
          />
        </View>
      )}

      <Button
        variant="ghost"
        label="Notification settings"
        onPress={() => router.push('/settings/notifications')}
      />

      <Button variant="ghost" label="Sign out" onPress={signOut} loading={isSubmitting} />
    </Screen>
  );
}

/**
 * The way into the notification inbox, with the unread count on it.
 *
 * A count rather than a dot, because the number is the useful part for somebody
 * who has been away from the phone — and it is spoken as words to a screen
 * reader, since a numeral in a coloured circle says nothing on its own
 * (plans/phase11.md §§57, 61).
 */
function NotificationsButton({ unread }: { unread: number }) {
  const theme = useTheme();
  const capped = unread > 99 ? '99+' : String(unread);

  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={
        unread === 0
          ? 'Notifications'
          : `Notifications, ${unread} unread ${unread === 1 ? 'notification' : 'notifications'}`
      }
      onPress={() => router.push('/notifications')}
      style={({ pressed }) => ({
        alignItems: 'center',
        borderColor: theme.colors.border,
        borderRadius: theme.radii.pill,
        borderWidth: 1,
        flexDirection: 'row',
        gap: theme.spacing.sm,
        minHeight: theme.minTouchTarget,
        opacity: pressed ? 0.85 : 1,
        paddingHorizontal: theme.spacing.lg,
      })}
    >
      <Text variant="bodyStrong" style={{ color: theme.colors.primary }}>
        Alerts
      </Text>

      {unread > 0 ? (
        <View
          accessible={false}
          style={{
            alignItems: 'center',
            backgroundColor: theme.colors.primary,
            borderRadius: theme.radii.pill,
            justifyContent: 'center',
            minWidth: 24,
            paddingHorizontal: 6,
            paddingVertical: 2,
          }}
        >
          <Text variant="secondary" style={{ color: theme.colors.onPrimary }}>
            {capped}
          </Text>
        </View>
      ) : null}
    </Pressable>
  );
}

/**
 * The caller's own work, across every circle they belong to.
 *
 * This is what a professional caregiver opens the app for: their round, in
 * order, without first having to pick which client they are looking at
 * (docs/13, "Professional Home").
 */
function AssignedToMe({ tasks, seniors }: { tasks: CareTask[]; seniors: Senior[] }) {
  const theme = useTheme();

  // This list spans circles, so each row is read in its own senior's timezone
  // rather than one zone for the whole screen.
  const timezones = new Map(seniors.map((senior) => [senior.id, senior.timezone]));
  const names = new Map(seniors.map((senior) => [senior.id, senior.displayName]));

  return (
    <Card>
      <Text variant="sectionHeading">Yours to do</Text>
      <View style={{ gap: theme.spacing.md }}>
        {tasks.slice(0, 5).map((task) => (
          <Link
            key={task.id}
            href={{ pathname: '/tasks/[taskId]', params: { taskId: task.id } }}
            asChild
          >
            <Pressable
              accessibilityRole="button"
              accessibilityLabel={`${task.title}, ${statusLabel(task.status)}`}
              style={{ gap: theme.spacing.xs, minHeight: theme.minTouchTarget }}
            >
              <Text variant="bodyStrong">{task.title}</Text>
              <Text variant="secondary" color="secondary">
                {taskTimeLabel(task, timezones.get(task.seniorId) ?? 'UTC')}
                {names.has(task.seniorId) ? ` · ${names.get(task.seniorId)}` : ''} ·{' '}
                {statusLabel(task.status)}
              </Text>
            </Pressable>
          </Link>
        ))}
      </View>
    </Card>
  );
}

/** Shown before the first profile exists — Solo Mode is one tap away. */
function EmptyState() {
  const theme = useTheme();

  return (
    <Card>
      <Text variant="sectionHeading">Let&apos;s get set up</Text>
      <Text variant="body" color="secondary">
        Add the person you are caring for — or yourself. You can invite family and caregivers
        whenever you need help, and never before.
      </Text>
      <View style={{ marginTop: theme.spacing.sm }}>
        <Button label="Get started" onPress={() => router.push('/onboarding')} />
      </View>
    </Card>
  );
}

/** One senior in the list, labelled with the reader's own relationship. */
function SeniorRow({ senior }: { senior: Senior }) {
  const theme = useTheme();

  return (
    <Link href={{ pathname: '/seniors/[seniorId]', params: { seniorId: senior.id } }} asChild>
      <Pressable
        accessibilityRole="button"
        accessibilityLabel={`${senior.displayName}, ${describeRole(senior)}`}
        style={({ pressed }) => [
          styles.row,
          {
            minHeight: theme.minTouchTarget,
            padding: theme.spacing.lg,
            borderRadius: theme.radii.lg,
            backgroundColor: theme.colors.surface,
            borderColor: theme.colors.border,
            opacity: pressed ? 0.85 : 1,
          },
        ]}
      >
        <View style={{ flex: 1, gap: theme.spacing.xs }}>
          <Text variant="bodyStrong">{senior.displayName}</Text>
          <Text variant="secondary" color="secondary">
            {describeRole(senior)}
          </Text>
        </View>
        <Text variant="body" color="secondary">
          ›
        </Text>
      </Pressable>
    </Link>
  );
}

/** Plain-language description of the reader's relationship to this senior. */
function describeRole(senior: Senior): string {
  if (senior.isSelf) return 'Your own care';

  switch (senior.role) {
    case 'family_member':
      return 'Family care';
    case 'professional_caregiver':
      return 'In your professional care';
    default:
      return 'Care circle';
  }
}

const styles = StyleSheet.create({
  row: {
    alignItems: 'center',
    borderWidth: StyleSheet.hairlineWidth,
    flexDirection: 'row',
    gap: 12,
  },
});
