import type {
  MarkAllReadResult,
  Notification,
  NotificationInbox,
  NotificationPreferences,
  RegisteredDevice,
  ReminderPlan,
} from '@meracare/contracts';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { apiRequest } from '@/lib/api-client';

import { describeDevice } from './device';

/**
 * Notification settings and the reminder plan.
 *
 * All of it is server state, so all of it lives in TanStack Query. Zustand
 * holds nothing here: a preference the server has not accepted is not a
 * preference, and a plan cached in a store would go on scheduling reminders for
 * a senior the user no longer has access to (plans/phase8.md §35).
 */

export const notificationKeys = {
  preferences: ['notifications', 'preferences'] as const,
  reminders: ['notifications', 'reminders'] as const,
  inbox: ['notifications', 'inbox'] as const,
};

/** The caller's notification settings. */
export function useNotificationPreferences(enabled = true) {
  return useQuery({
    queryKey: notificationKeys.preferences,
    queryFn: () => apiRequest<NotificationPreferences>('/notifications/preferences'),
    enabled,
    // Settings change only when this user changes them, and the mutation below
    // writes the result straight into the cache.
    staleTime: 5 * 60_000,
  });
}

/**
 * Changes one or more categories.
 *
 * Deliberately not queued for offline replay. The offline queue exists for care
 * that was given — a dose taken, a task completed — where losing the record
 * would lose something that actually happened. A preference is a statement
 * about the future, and one applied optimistically offline would have the app
 * showing reminders as off while the server went on planning them
 * (plans/phase8.md §36).
 */
export function useUpdateNotificationPreferences() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (change: Partial<Omit<NotificationPreferences, 'updatedAt'>>) =>
      apiRequest<NotificationPreferences>('/notifications/preferences', {
        method: 'PATCH',
        body: change,
      }),
    onSuccess: (preferences) => {
      queryClient.setQueryData(notificationKeys.preferences, preferences);
      // A silenced category must disappear from the device, not just from the
      // screen, so the plan is refetched and reconciled (plans/phase8.md §22).
      void queryClient.invalidateQueries({ queryKey: notificationKeys.reminders });
    },
  });
}

/**
 * The reminders this device should have scheduled.
 *
 * Refetched rather than long-cached: the plan is how a revoked caregiver's
 * reminders stop, so a stale one is not merely out of date, it is a small
 * privacy problem (plans/phase8.md §23).
 */
export function useReminderPlan(enabled = true) {
  return useQuery({
    queryKey: notificationKeys.reminders,
    queryFn: () => apiRequest<ReminderPlan>('/notifications/reminders'),
    enabled,
    staleTime: 60_000,
  });
}

/**
 * Registers this installation with the server.
 *
 * Called on sign-in and after permission is granted, because both change what
 * the answer would be. Repeating it is free: the endpoint upserts on the device
 * identifier (plans/phase8.md §25).
 */
export function useRegisterDevice() {
  return useMutation({
    mutationFn: async () =>
      apiRequest<RegisteredDevice>('/notifications/devices', {
        method: 'POST',
        body: await describeDevice(),
      }),
  });
}

/**
 * The notification inbox, newest first.
 *
 * Keyset-paged through the same cursor every other history in MeraCare uses, so
 * a professional caregiver with months of notifications loads a screenful
 * rather than all of them (plans/phase11.md §§41, 56).
 *
 * Refetched on mount rather than served long from cache: the unread count is on
 * every page, and a badge showing a number from ten minutes ago is worse than
 * no badge.
 */
export function useNotificationInbox(enabled = true) {
  return useInfiniteQuery({
    queryKey: notificationKeys.inbox,
    queryFn: ({ pageParam }) =>
      apiRequest<NotificationInbox>(
        pageParam === undefined
          ? '/notifications'
          : `/notifications?cursor=${encodeURIComponent(pageParam)}`,
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (lastPage) => lastPage.nextCursor ?? undefined,
    enabled,
    staleTime: 30_000,
  });
}

/**
 * How many notifications the reader has not seen.
 *
 * Read off the inbox rather than counted separately, so the badge and the list
 * come from one answer and cannot drift apart (plans/phase11.md §61).
 */
export function useUnreadCount(enabled = true): number {
  const inbox = useNotificationInbox(enabled);
  return inbox.data?.pages[0]?.unreadCount ?? 0;
}

/**
 * Marks one notification as read.
 *
 * Not queued for offline replay, for the same reason a preference is not: the
 * offline queue exists for care that was given, where losing the record loses
 * something that happened. Whether somebody has read a notification is not
 * that (plans/phase8.md §36).
 */
export function useMarkNotificationRead() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: string) =>
      apiRequest<Notification>(`/notifications/${encodeURIComponent(id)}/read`, {
        method: 'PATCH',
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: notificationKeys.inbox });
    },
  });
}

/** Marks every arrived notification as read. */
export function useMarkAllNotificationsRead() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => apiRequest<MarkAllReadResult>('/notifications/read-all', { method: 'POST' }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: notificationKeys.inbox });
    },
  });
}
