import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react-native';
import type { ReactNode } from 'react';
import { SafeAreaProvider } from 'react-native-safe-area-context';

import NotificationSettingsScreen from '@/app/settings/notifications';
import { ThemeProvider } from '@/theme';

/**
 * The settings screen has to tell the truth about two separate things: what
 * GenXcare will send, and what the phone will allow. Most of what is asserted
 * here is that the second one is never hidden (plans/phase8.md §§6, 19).
 */

const mockApiRequest = jest.fn();
const mockPermission = jest.fn();
const mockRequestPermission = jest.fn();

jest.mock('@/lib/api-client', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}));

jest.mock('@/features/auth/session-provider', () => ({
  useSession: () => ({ isSignedIn: true, isRestoring: false }),
}));

jest.mock('@/features/notifications/permission', () => ({
  notificationPermission: () => mockPermission(),
  requestNotificationPermission: () => mockRequestPermission(),
  permissionAllowsDelivery: (state: string) => state === 'granted' || state === 'provisional',
}));

jest.mock('expo-router', () => ({
  Stack: { Screen: () => null },
}));

const clients: QueryClient[] = [];

afterEach(() => {
  for (const client of clients.splice(0)) {
    client.clear();
    client.unmount();
  }
});

function renderScreen() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: Infinity }, mutations: { retry: false } },
  });
  clients.push(queryClient);

  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <SafeAreaProvider
        initialMetrics={{
          frame: { x: 0, y: 0, width: 390, height: 844 },
          insets: { top: 47, left: 0, right: 0, bottom: 34 },
        }}
      >
        <QueryClientProvider client={queryClient}>
          <ThemeProvider>{children}</ThemeProvider>
        </QueryClientProvider>
      </SafeAreaProvider>
    );
  }

  return render(<NotificationSettingsScreen />, { wrapper: Wrapper });
}

const preferences = {
  taskReminders: true,
  medicationReminders: true,
  appointmentReminders: true,
  overdueTaskAlerts: true,
  missedMedicationAlerts: true,
  careActivity: true,
  updatedAt: '2026-08-19T06:00:00Z',
};

beforeEach(() => {
  mockApiRequest.mockReset().mockResolvedValue(preferences);
  mockPermission.mockReset().mockResolvedValue('granted');
  mockRequestPermission.mockReset().mockResolvedValue('granted');
});

it('shows every category in plain language', async () => {
  renderScreen();

  await waitFor(() => expect(screen.getByText('Medication reminders')).toBeTruthy());
  expect(screen.getByText('Care task reminders')).toBeTruthy();
  expect(screen.getByText('Appointment reminders')).toBeTruthy();
  expect(screen.getByText('Missed medication alerts')).toBeTruthy();
});

it('never shows an internal notification type', async () => {
  renderScreen();

  await waitFor(() => expect(screen.getByText('Medication reminders')).toBeTruthy());

  for (const identifier of [
    'MEDICATION_REMINDER',
    'MEDICATION_MISSED',
    'TASK_REMINDER',
    'APPOINTMENT_REMINDER',
  ]) {
    expect(screen.queryByText(identifier)).toBeNull();
  }
});

it('saves a category the user switches off', async () => {
  renderScreen();

  await waitFor(() => expect(screen.getByText('Medication reminders')).toBeTruthy());

  fireEvent(screen.getByLabelText('Medication reminders'), 'valueChange', false);

  await waitFor(() =>
    expect(mockApiRequest).toHaveBeenCalledWith('/notifications/preferences', {
      method: 'PATCH',
      body: { medicationReminders: false },
    }),
  );
});

it('says so when the phone is blocking notifications', async () => {
  // Without this the screen would show three switches turned on and the user
  // would never learn why their phone is silent.
  mockPermission.mockResolvedValue('denied');

  renderScreen();

  await waitFor(() => expect(screen.getByText('Notifications are turned off')).toBeTruthy());
  expect(screen.getByText('Open Settings')).toBeTruthy();
});

it('offers the prompt when permission has not been asked for', async () => {
  mockPermission.mockResolvedValue('undetermined');

  renderScreen();

  await waitFor(() => expect(screen.getByText('Turn on notifications')).toBeTruthy());

  fireEvent.press(screen.getByText('Allow notifications'));

  await waitFor(() => expect(mockRequestPermission).toHaveBeenCalled());
});

it('registers the device once permission is granted', async () => {
  // The push token only exists after permission does, so this is the moment to
  // tell the server about this installation again.
  mockPermission.mockResolvedValue('undetermined');

  renderScreen();

  await waitFor(() => expect(screen.getByText('Turn on notifications')).toBeTruthy());
  fireEvent.press(screen.getByText('Allow notifications'));

  await waitFor(() =>
    expect(mockApiRequest.mock.calls.some(([path]) => path === '/notifications/devices')).toBe(
      true,
    ),
  );
});

it('explains iOS quiet delivery rather than calling it granted', async () => {
  mockPermission.mockResolvedValue('provisional');

  renderScreen();

  await waitFor(() => expect(screen.getByText('Reminders arrive quietly')).toBeTruthy());
});

it('says nothing about permission while it is still being read', async () => {
  renderScreen();

  expect(screen.queryByText('Notifications are turned off')).toBeNull();
  expect(screen.queryByText('Turn on notifications')).toBeNull();

  await waitFor(() => expect(screen.getByText('Medication reminders')).toBeTruthy());
});

it('reports a failure to load rather than showing everything as off', async () => {
  mockApiRequest.mockRejectedValue(new Error('offline'));

  renderScreen();

  await waitFor(() => expect(screen.getByText('We could not load your settings')).toBeTruthy());
  expect(screen.queryByText('Medication reminders')).toBeNull();
});
