import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react-native';
import * as Notifications from 'expo-notifications';
import type { ReactNode } from 'react';

import {
  usePendingDestination,
  usePendingMedicationNotificationAction,
  useReminderTaps,
} from '../use-reminder-sync';

import { useUIStore } from '@/stores/ui-store';

/**
 * A reminder is tapped from a lock screen. Whether the app is signed in at that
 * moment is not something the notification can know, and losing the destination
 * because the session had not restored yet would make the reminder useless at
 * exactly the moment it mattered (plans/phase9.md §26).
 */

const mockPush = jest.fn();
const mockRecord = jest.fn();
const mockConfirm = jest.fn();
const mockSnooze = jest.fn();

jest.mock('expo-router', () => ({ router: { push: (...args: unknown[]) => mockPush(...args) } }));

jest.mock('expo-notifications', () => ({
  addNotificationResponseReceivedListener: jest.fn(() => ({ remove: jest.fn() })),
}));

jest.mock('@/lib/api-client', () => ({ apiRequest: jest.fn(async () => ({ items: [] })) }));

jest.mock('@/features/notifications/scheduler', () => ({
  syncReminders: jest.fn(),
  clearReminders: jest.fn(),
  MEDICATION_SKIP_ACTION: 'medication_skip',
  MEDICATION_SNOOZE_ACTION: 'medication_snooze',
  MEDICATION_TAKEN_ACTION: 'medication_taken',
  cancelSnoozedMedicationNotifications: jest.fn(async () => undefined),
  registerMedicationNotificationActions: jest.fn(),
  snoozeMedicationNotification: (...args: unknown[]) => mockSnooze(...args),
}));

jest.mock('@/features/notifications/medication-actions', () => ({
  recordMedicationNotificationAction: (...args: unknown[]) => mockRecord(...args),
}));

jest.mock('@/lib/dialogs', () => ({
  confirmAction: (...args: unknown[]) => mockConfirm(...args),
  showMessage: jest.fn(),
}));

jest.mock('@/features/notifications/permission', () => ({
  notificationPermission: jest.fn(async () => 'granted'),
  permissionAllowsDelivery: () => true,
}));

const listen = Notifications.addNotificationResponseReceivedListener as jest.Mock;

/** Fires the listener the hook registered, as the OS would on a tap. */
function tapNotification(actionIdentifier?: string) {
  const handler = listen.mock.calls.at(-1)?.[0] as (response: unknown) => void;
  handler({
    notification: {
      request: {
        content: {
          data: {
            reminderId: 'reminder-1',
            type: 'MEDICATION_REMINDER',
            seniorId: 'senior-1',
            entityType: 'medication_dose',
            entityId: 'dose-1',
          },
        },
      },
    },
    actionIdentifier,
  });
}

const clients: QueryClient[] = [];

afterEach(() => {
  for (const client of clients.splice(0)) {
    client.clear();
    client.unmount();
  }
  useUIStore.getState().reset();
});

function wrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: Infinity }, mutations: { retry: false } },
  });
  clients.push(queryClient);

  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  };
}

beforeEach(() => {
  mockPush.mockReset();
  mockRecord.mockReset().mockResolvedValue('recorded');
  mockConfirm.mockReset();
  mockSnooze.mockReset().mockResolvedValue(undefined);
  listen.mockClear();
  useUIStore.getState().reset();
});

it('navigates straight away when the user is already signed in', () => {
  renderHook(() => useReminderTaps(true, false), { wrapper: wrapper() });

  tapNotification();

  expect(mockPush).toHaveBeenCalledWith({
    pathname: '/seniors/[seniorId]/medications',
    params: { seniorId: 'senior-1' },
  });
  expect(useUIStore.getState().pendingDestination).toBeNull();
});

it('holds the destination when the session is still restoring', () => {
  renderHook(() => useReminderTaps(false, true), { wrapper: wrapper() });

  tapNotification();

  expect(mockPush).not.toHaveBeenCalled();
  expect(useUIStore.getState().pendingDestination).toEqual({
    pathname: '/seniors/[seniorId]/medications',
    params: { seniorId: 'senior-1' },
  });
});

it('holds the destination when the user is signed out', () => {
  renderHook(() => useReminderTaps(false, false), { wrapper: wrapper() });

  tapNotification();

  expect(mockPush).not.toHaveBeenCalled();
  expect(useUIStore.getState().pendingDestination).not.toBeNull();
});

it('goes there once the user signs in', async () => {
  const { rerender } = renderHook(
    ({ signedIn }: { signedIn: boolean }) => {
      useReminderTaps(signedIn, false);
      usePendingDestination(signedIn, false);
    },
    { wrapper: wrapper(), initialProps: { signedIn: false } },
  );

  tapNotification();
  expect(mockPush).not.toHaveBeenCalled();

  rerender({ signedIn: true });

  await waitFor(() =>
    expect(mockPush).toHaveBeenCalledWith({
      pathname: '/seniors/[seniorId]/medications',
      params: { seniorId: 'senior-1' },
    }),
  );
  // Consumed, so a later render does not send the user there again.
  expect(useUIStore.getState().pendingDestination).toBeNull();
});

it('does not act on a held destination while the session is still restoring', async () => {
  renderHook(() => usePendingDestination(false, true), { wrapper: wrapper() });

  useUIStore.getState().setPendingDestination({ pathname: '/home' });

  await waitFor(() => expect(useUIStore.getState().pendingDestination).not.toBeNull());
  expect(mockPush).not.toHaveBeenCalled();
});

it('ignores a notification whose payload it cannot read', () => {
  renderHook(() => useReminderTaps(true, false), { wrapper: wrapper() });

  const handler = listen.mock.calls.at(-1)?.[0] as (response: unknown) => void;
  handler({ notification: { request: { content: { data: { nonsense: true } } } } });

  expect(mockPush).not.toHaveBeenCalled();
  expect(useUIStore.getState().pendingDestination).toBeNull();
});

it('records Taken directly from a medication notification', async () => {
  renderHook(() => useReminderTaps(true, false), { wrapper: wrapper() });

  tapNotification('medication_taken');

  await waitFor(() =>
    expect(mockRecord).toHaveBeenCalledWith(expect.anything(), 'senior-1', 'dose-1', 'take'),
  );
});

it('asks for confirmation before skipping a dose', () => {
  renderHook(() => useReminderTaps(true, false), { wrapper: wrapper() });

  tapNotification('medication_skip');

  expect(mockConfirm).toHaveBeenCalledWith(
    expect.objectContaining({ title: 'Skip this dose?', confirmLabel: 'Skip dose' }),
  );
  expect(mockRecord).not.toHaveBeenCalled();
});

it('schedules Remind in 10 minutes without recording the dose', async () => {
  renderHook(() => useReminderTaps(true, false), { wrapper: wrapper() });

  tapNotification('medication_snooze');

  await waitFor(() => expect(mockSnooze).toHaveBeenCalled());
  expect(mockRecord).not.toHaveBeenCalled();
});

it('holds Taken until a signed-out user signs in', async () => {
  const { rerender } = renderHook(
    ({ signedIn }: { signedIn: boolean }) => {
      useReminderTaps(signedIn, false);
      usePendingDestination(signedIn, false);
      usePendingMedicationNotificationAction(signedIn, false);
    },
    { wrapper: wrapper(), initialProps: { signedIn: false } },
  );

  tapNotification('medication_taken');
  expect(mockRecord).not.toHaveBeenCalled();

  rerender({ signedIn: true });

  await waitFor(() =>
    expect(mockRecord).toHaveBeenCalledWith(expect.anything(), 'senior-1', 'dose-1', 'take'),
  );
  expect(useUIStore.getState().pendingMedicationNotificationAction).toBeNull();
});
