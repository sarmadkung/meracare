import type { NotificationPreferences } from '@meracare/contracts';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react-native';
import type { ReactNode } from 'react';

import { ApiError } from '@/lib/api-error';

import {
  notificationKeys,
  useNotificationPreferences,
  useUpdateNotificationPreferences,
} from '../use-notifications';

/**
 * Settings are server state. What matters is that a change is only believed
 * once the server has accepted it, and that a failure reads as a failure —
 * a switch that slides back is honest; one that stays put while the server
 * disagrees is not.
 */

const mockApiRequest = jest.fn();

jest.mock('@/lib/api-client', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}));

function preferences(overrides: Partial<NotificationPreferences> = {}): NotificationPreferences {
  return {
    taskReminders: true,
    medicationReminders: true,
    appointmentReminders: true,
    overdueTaskAlerts: true,
    careActivity: true,
    updatedAt: '2026-08-19T06:00:00Z',
    ...overrides,
  };
}

const clients: QueryClient[] = [];

afterEach(() => {
  for (const client of clients.splice(0)) {
    client.clear();
    client.unmount();
  }
});

function newClient() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: Infinity },
      mutations: { retry: false },
    },
  });
  clients.push(queryClient);
  return queryClient;
}

function wrapperFor(queryClient: QueryClient) {
  function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  }
  return Wrapper;
}

beforeEach(() => {
  mockApiRequest.mockReset();
});

it('loads the settings', async () => {
  mockApiRequest.mockResolvedValue(preferences());

  const { result } = renderHook(() => useNotificationPreferences(), {
    wrapper: wrapperFor(newClient()),
  });

  await waitFor(() => expect(result.current.isSuccess).toBe(true));
  expect(mockApiRequest).toHaveBeenCalledWith('/notifications/preferences');
  expect(result.current.data?.medicationReminders).toBe(true);
});

it('sends only the category that changed', async () => {
  // A full-object write would let a screen loaded five minutes ago overwrite a
  // change made on another device since.
  mockApiRequest.mockResolvedValue(preferences({ medicationReminders: false }));

  const { result } = renderHook(() => useUpdateNotificationPreferences(), {
    wrapper: wrapperFor(newClient()),
  });

  await act(async () => {
    await result.current.mutateAsync({ medicationReminders: false });
  });

  expect(mockApiRequest).toHaveBeenCalledWith('/notifications/preferences', {
    method: 'PATCH',
    body: { medicationReminders: false },
  });
});

it('writes the server’s answer into the cache, not the requested change', async () => {
  const queryClient = newClient();
  mockApiRequest.mockResolvedValue(preferences({ taskReminders: false }));

  const { result } = renderHook(() => useUpdateNotificationPreferences(), {
    wrapper: wrapperFor(queryClient),
  });

  await act(async () => {
    await result.current.mutateAsync({ taskReminders: false });
  });

  expect(queryClient.getQueryData(notificationKeys.preferences)).toEqual(
    preferences({ taskReminders: false }),
  );
});

it('refreshes the reminder plan after a change', async () => {
  // A silenced category has to leave the device, not just the screen.
  const queryClient = newClient();
  const invalidate = jest.spyOn(queryClient, 'invalidateQueries');
  mockApiRequest.mockResolvedValue(preferences({ medicationReminders: false }));

  const { result } = renderHook(() => useUpdateNotificationPreferences(), {
    wrapper: wrapperFor(queryClient),
  });

  await act(async () => {
    await result.current.mutateAsync({ medicationReminders: false });
  });

  expect(invalidate).toHaveBeenCalledWith({ queryKey: notificationKeys.reminders });
});

it('leaves the cache alone when the change fails', async () => {
  // Offline, a preference change is refused rather than queued: it is a
  // statement about the future, not a record of care that was given
  // (plans/phase8.md §36).
  const queryClient = newClient();
  queryClient.setQueryData(notificationKeys.preferences, preferences());
  mockApiRequest.mockRejectedValue(ApiError.network(new Error('offline')));

  const { result } = renderHook(() => useUpdateNotificationPreferences(), {
    wrapper: wrapperFor(queryClient),
  });

  await act(async () => {
    await result.current.mutateAsync({ medicationReminders: false }).catch(() => {});
  });

  await waitFor(() => expect(result.current.isError).toBe(true));
  expect(queryClient.getQueryData(notificationKeys.preferences)).toEqual(preferences());
});

it('asks for nothing until the user is signed in', () => {
  renderHook(() => useNotificationPreferences(false), { wrapper: wrapperFor(newClient()) });

  expect(mockApiRequest).not.toHaveBeenCalled();
});
