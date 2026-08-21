import type { Notification } from '@meracare/contracts';
import { dateKeyInTimezone, dayHeading } from '@meracare/contracts';
import { Redirect, Stack, router } from 'expo-router';
import { useMemo } from 'react';
import { ActivityIndicator, SectionList, View } from 'react-native';

import { Button, Card, NotificationRow, Screen, Text } from '@/components/ui';
import { useSession } from '@/features/auth/session-provider';
import { notificationDestination } from '@/features/notifications/routes';
import {
  useMarkAllNotificationsRead,
  useMarkNotificationRead,
  useNotificationInbox,
} from '@/features/notifications/use-notifications';
import { ApiError } from '@/lib/api-error';
import { useTheme } from '@/theme';

/**
 * The notification inbox.
 *
 * A history of what MeraCare has told this person, in the device's own
 * timezone. That is the one place in the app where the reader's clock is the
 * right one: a reminder's *content* is about a senior's day and reads in their
 * zone, but "when did I get this?" is a question about the reader
 * (plans/phase11.md §§27, 33).
 *
 * A SectionList, like the activity timeline: day headings stick as it scrolls
 * and the rows are virtualised, which matters most for the professional
 * caregiver whose inbox covers several people (plans/phase11.md §56).
 */
export default function NotificationsScreen() {
  const theme = useTheme();
  const { isSignedIn, isRestoring } = useSession();

  const inbox = useNotificationInbox(isSignedIn);
  const markRead = useMarkNotificationRead();
  const markAllRead = useMarkAllNotificationsRead();

  const timezone = deviceTimezone();

  const notifications = useMemo(
    () => inbox.data?.pages.flatMap((page) => page.items) ?? [],
    [inbox.data],
  );
  const unread = inbox.data?.pages[0]?.unreadCount ?? 0;

  const sections = useMemo(() => groupByDay(notifications, timezone), [notifications, timezone]);

  if (!isRestoring && !isSignedIn) {
    return <Redirect href="/sign-in" />;
  }

  function open(notification: Notification) {
    // Marked read as it is opened rather than after the destination loads: the
    // person has seen it either way, and a notification that stays bold because
    // the screen behind it failed to load is a badge that never clears.
    if (!notification.read) markRead.mutate(notification.id);

    // Nothing here asks whether the destination is still permitted. The screen
    // asks the server, which decides — so a notification that outlived somebody's
    // access opens a "not found", not somebody else's care
    // (plans/phase11.md §§30, 31).
    router.push(notificationDestination(notification));
  }

  return (
    <Screen>
      <Stack.Screen options={{ headerShown: true, title: 'Notifications' }} />

      <SectionList
        sections={sections}
        keyExtractor={(notification) => notification.id}
        stickySectionHeadersEnabled
        renderItem={({ item }) => (
          <NotificationRow notification={item} timezone={timezone} onPress={() => open(item)} />
        )}
        renderSectionHeader={({ section }) => (
          <View
            style={{
              backgroundColor: theme.colors.background,
              paddingBottom: theme.spacing.sm,
              paddingTop: theme.spacing.md,
            }}
          >
            <Text variant="sectionHeading">{section.title}</Text>
          </View>
        )}
        contentContainerStyle={{ gap: theme.spacing.md, paddingBottom: theme.spacing.xl }}
        ListHeaderComponent={
          <View style={{ gap: theme.spacing.sm, paddingBottom: theme.spacing.sm }}>
            <Text variant="pageHeading">Notifications</Text>
            <Text variant="body" color="secondary">
              {unread === 0
                ? 'Everything here has been read.'
                : `${unread} unread ${unread === 1 ? 'notification' : 'notifications'}.`}
            </Text>

            {unread > 0 ? (
              <Button
                variant="secondary"
                label="Mark all as read"
                loading={markAllRead.isPending}
                onPress={() => markAllRead.mutate()}
              />
            ) : null}
          </View>
        }
        ListEmptyComponent={
          inbox.isPending ? (
            <View style={{ alignItems: 'center', padding: theme.spacing.xl }}>
              <ActivityIndicator color={theme.colors.primary} />
            </View>
          ) : inbox.isError ? (
            <Card>
              <Text variant="sectionHeading">We could not load your notifications</Text>
              <Text variant="body" color="secondary">
                {inbox.error instanceof ApiError ? inbox.error.message : 'Please try again.'}
              </Text>
              <Button variant="secondary" label="Try again" onPress={() => void inbox.refetch()} />
            </Card>
          ) : (
            <Card>
              <Text variant="sectionHeading">Nothing yet</Text>
              <Text variant="body" color="secondary">
                Reminders about medication, care tasks, and appointments will appear here.
              </Text>
            </Card>
          )
        }
        ListFooterComponent={
          inbox.hasNextPage ? (
            <Button
              variant="secondary"
              label="Show more"
              loading={inbox.isFetchingNextPage}
              onPress={() => void inbox.fetchNextPage()}
            />
          ) : null
        }
        refreshing={inbox.isRefetching}
        onRefresh={() => void inbox.refetch()}
      />
    </Screen>
  );
}

interface NotificationDay {
  key: string;
  title: string;
  data: Notification[];
}

/**
 * Groups an already-sorted inbox into days.
 *
 * Its own function rather than the care-event one, because that one is typed to
 * care events and widening it would give two callers a shared shape neither
 * needs. The arithmetic — one pass, relying on the server's ordering — is the
 * same (plans/phase7.md §16).
 */
function groupByDay(notifications: Notification[], timezone: string): NotificationDay[] {
  const days: NotificationDay[] = [];

  for (const notification of notifications) {
    const key = dateKeyInTimezone(notification.occurredAt, timezone);
    const current = days[days.length - 1];

    if (current !== undefined && current.key === key) {
      current.data.push(notification);
      continue;
    }

    days.push({
      key,
      title: dayHeading(notification.occurredAt, timezone),
      data: [notification],
    });
  }

  return days;
}

/**
 * The reader's own timezone, falling back to UTC where the runtime cannot say.
 */
function deviceTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  } catch {
    return 'UTC';
  }
}
