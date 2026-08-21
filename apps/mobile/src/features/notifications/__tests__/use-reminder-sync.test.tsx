import type { ReminderPlan } from '@meracare/contracts';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react-native';
import type { ReactNode } from 'react';

import { useReminderSync } from '../use-reminder-sync';

/**
 * The reconciliation loop. Most of its behaviour is covered by reconcile and
 * scheduler tests; what is asserted here is when it runs — and, more
 * importantly, when it must not.
 */

const mockApiRequest = jest.fn();
const mockSync = jest.fn();
const mockClear = jest.fn();
const mockPermission = jest.fn();

jest.mock('@/lib/api-client', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}));

jest.mock('@/features/notifications/scheduler', () => ({
  syncReminders: (...args: unknown[]) => mockSync(...args),
  clearReminders: () => mockClear(),
}));

jest.mock('@/features/notifications/permission', () => ({
  notificationPermission: () => mockPermission(),
  permissionAllowsDelivery: (state: string) => state === 'granted' || state === 'provisional',
}));

jest.mock('expo-notifications', () => ({
  addNotificationResponseReceivedListener: jest.fn(() => ({ remove: jest.fn() })),
}));

jest.mock('expo-router', () => ({ router: { push: jest.fn() } }));

const plan: ReminderPlan = {
  reminders: [],
  generatedAt: '2026-08-19T06:00:00Z',
  horizonEndsAt: '2026-08-26T06:00:00Z',
};

const clients: QueryClient[] = [];

afterEach(() => {
  for (const client of clients.splice(0)) {
    client.clear();
    client.unmount();
  }
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
  mockApiRequest.mockReset().mockResolvedValue(plan);
  mockSync.mockReset().mockResolvedValue({ scheduled: 0, cancelled: 0 });
  mockClear.mockReset().mockResolvedValue(undefined);
  mockPermission.mockReset().mockResolvedValue('granted');
});

it('registers the device and applies the plan once signed in', async () => {
  renderHook(() => useReminderSync(true, false), { wrapper: wrapper() });

  await waitFor(() =>
    expect(mockApiRequest.mock.calls.some(([path]) => path === '/notifications/devices')).toBe(
      true,
    ),
  );
  await waitFor(() => expect(mockSync).toHaveBeenCalledWith(plan));
});

it('does not clear reminders while the session is still being restored', async () => {
  // A cold start is signed-out until the stored session loads. Clearing then
  // would wipe every scheduled reminder on every launch — and while the phone
  // is offline, nothing would reschedule them.
  renderHook(() => useReminderSync(false, true), { wrapper: wrapper() });

  await waitFor(() => expect(mockApiRequest).not.toHaveBeenCalled());
  expect(mockClear).not.toHaveBeenCalled();
});

it('does not clear reminders for a user who was never signed in', async () => {
  renderHook(() => useReminderSync(false, false), { wrapper: wrapper() });

  await waitFor(() => expect(mockApiRequest).not.toHaveBeenCalled());
  expect(mockClear).not.toHaveBeenCalled();
});

it('clears reminders on an actual sign-out', async () => {
  // A phone handed to somebody else must stop announcing a family's care.
  const { rerender } = renderHook(
    ({ signedIn }: { signedIn: boolean }) => useReminderSync(signedIn, false),
    { wrapper: wrapper(), initialProps: { signedIn: true } },
  );

  await waitFor(() => expect(mockSync).toHaveBeenCalled());

  rerender({ signedIn: false });

  await waitFor(() => expect(mockClear).toHaveBeenCalled());
});

it('schedules nothing while the OS is refusing notifications', async () => {
  // The reminders stay in the plan; they are simply not scheduled until
  // permission exists.
  mockPermission.mockResolvedValue('denied');

  renderHook(() => useReminderSync(true, false), { wrapper: wrapper() });

  await waitFor(() => expect(mockApiRequest).toHaveBeenCalled());
  await new Promise((resolve) => setTimeout(resolve, 20));

  expect(mockSync).not.toHaveBeenCalled();
});

it('survives a device that refuses to schedule', async () => {
  // Notifications failing must not break the app; the care is on the screens
  // either way (plans/phase8.md §37).
  mockSync.mockRejectedValue(new Error('scheduling unavailable'));

  renderHook(() => useReminderSync(true, false), { wrapper: wrapper() });

  await waitFor(() => expect(mockSync).toHaveBeenCalled());
});

/**
 * Phase 11 gave the server a push path, which means exactly one of the two must
 * schedule each reminder. Two would show every dose twice
 * (plans/phase11.md §35).
 */

/** Answers the devices endpoint separately from the plan. */
function respondWith(registration: unknown) {
  mockApiRequest.mockImplementation((path: string) =>
    path === '/notifications/devices' ? Promise.resolve(registration) : Promise.resolve(plan),
  );
}

it('does not schedule locally when the server can push to this device', async () => {
  respondWith({ pushTokenRegistered: true });

  renderHook(() => useReminderSync(true, false), { wrapper: wrapper() });

  await waitFor(() => expect(mockClear).toHaveBeenCalled());
  expect(mockSync).not.toHaveBeenCalled();
});

it('schedules locally when the server holds no token for this device', async () => {
  respondWith({ pushTokenRegistered: false });

  renderHook(() => useReminderSync(true, false), { wrapper: wrapper() });

  await waitFor(() => expect(mockSync).toHaveBeenCalledWith(plan));
});

it('falls back to local scheduling when registration fails', async () => {
  // Offline, or the server is down. The reminders still have to happen, and the
  // device is the only party that can make them happen without a network.
  mockApiRequest.mockImplementation((path: string) =>
    path === '/notifications/devices'
      ? Promise.reject(new Error('offline'))
      : Promise.resolve(plan),
  );

  renderHook(() => useReminderSync(true, false), { wrapper: wrapper() });

  await waitFor(() => expect(mockSync).toHaveBeenCalledWith(plan));
});
