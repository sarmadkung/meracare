import type { Notification, NotificationInbox } from '@meracare/contracts';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react-native';
import type { ReactNode } from 'react';
import { SafeAreaProvider } from 'react-native-safe-area-context';

import NotificationsScreen from '@/app/notifications';
import { ThemeProvider } from '@/theme';

/**
 * The inbox screen.
 *
 * What matters here is that it tells the truth about what has been read, that
 * the badge count and the list come from the same answer, and that opening a
 * notification navigates without pretending to have checked whether the reader
 * may see the destination (plans/phase11.md §§27, 28, 30, 61).
 */

const mockApiRequest = jest.fn();
const mockPush = jest.fn();

jest.mock('@/lib/api-client', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}));

jest.mock('@/features/auth/session-provider', () => ({
  useSession: () => ({ isSignedIn: true, isRestoring: false }),
}));

jest.mock('expo-router', () => ({
  Stack: { Screen: () => null },
  Redirect: () => null,
  router: { push: (...args: unknown[]) => mockPush(...args) },
}));

function notification(overrides: Partial<Notification> = {}): Notification {
  return {
    id: 'notification-1',
    type: 'MEDICATION_REMINDER',
    title: 'Medication reminder',
    body: 'A dose is due for Amma at 08:00.',
    seniorId: 'senior-1',
    entityType: 'medication_dose',
    entityId: 'dose-1',
    occurredAt: '2026-08-20T02:15:00Z',
    read: false,
    readAt: '',
    ...overrides,
  };
}

function page(overrides: Partial<NotificationInbox> = {}): NotificationInbox {
  return {
    items: [notification()],
    nextCursor: null,
    unreadCount: 1,
    ...overrides,
  };
}

const clients: QueryClient[] = [];

beforeEach(() => {
  jest.clearAllMocks();
});

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

  return render(<NotificationsScreen />, { wrapper: Wrapper });
}

it('lists notifications with the words that were sent', async () => {
  mockApiRequest.mockResolvedValue(page());
  renderScreen();

  await waitFor(() => expect(screen.getByText('Medication reminder')).toBeTruthy());
  // The server's wording, unchanged — so the inbox and the lock screen agree.
  expect(screen.getByText('A dose is due for Amma at 08:00.')).toBeTruthy();
});

it('says how many are unread', async () => {
  mockApiRequest.mockResolvedValue(
    page({
      items: [notification(), notification({ id: 'notification-2' })],
      unreadCount: 2,
    }),
  );
  renderScreen();

  await waitFor(() => expect(screen.getByText('2 unread notifications.')).toBeTruthy());
});

it('says so when everything has been read, and offers no mark-all button', async () => {
  mockApiRequest.mockResolvedValue(
    page({ items: [notification({ read: true, readAt: '2026-08-20T03:00:00Z' })], unreadCount: 0 }),
  );
  renderScreen();

  await waitFor(() => expect(screen.getByText('Everything here has been read.')).toBeTruthy());
  expect(screen.queryByText('Mark all as read')).toBeNull();
});

it('offers an empty state rather than a blank screen', async () => {
  mockApiRequest.mockResolvedValue(page({ items: [], unreadCount: 0 }));
  renderScreen();

  await waitFor(() => expect(screen.getByText('Nothing yet')).toBeTruthy());
});

it('marks a notification read when it is opened', async () => {
  mockApiRequest.mockImplementation((path: string) => {
    if (path === '/notifications') return Promise.resolve(page());
    return Promise.resolve(notification({ read: true, readAt: '2026-08-20T03:00:00Z' }));
  });
  renderScreen();

  await waitFor(() => expect(screen.getByText('Medication reminder')).toBeTruthy());
  fireEvent.press(screen.getByLabelText(/^Unread\. Medication reminder/));

  await waitFor(() =>
    expect(mockApiRequest).toHaveBeenCalledWith('/notifications/notification-1/read', {
      method: 'PATCH',
    }),
  );
});

it('navigates to the thing the notification is about', async () => {
  mockApiRequest.mockResolvedValue(page());
  renderScreen();

  await waitFor(() => expect(screen.getByText('Medication reminder')).toBeTruthy());
  fireEvent.press(screen.getByLabelText(/^Unread\. Medication reminder/));

  expect(mockPush).toHaveBeenCalledWith({
    pathname: '/seniors/[seniorId]/medications',
    params: { seniorId: 'senior-1' },
  });
});

it('does not re-mark a notification that is already read', async () => {
  mockApiRequest.mockResolvedValue(
    page({ items: [notification({ read: true, readAt: '2026-08-20T03:00:00Z' })], unreadCount: 0 }),
  );
  renderScreen();

  await waitFor(() => expect(screen.getByText('Medication reminder')).toBeTruthy());
  mockApiRequest.mockClear();

  fireEvent.press(screen.getByLabelText(/^Medication reminder/));

  expect(mockApiRequest).not.toHaveBeenCalledWith(
    expect.stringContaining('/read'),
    expect.anything(),
  );
  expect(mockPush).toHaveBeenCalled();
});

it('marks everything read in one request', async () => {
  mockApiRequest.mockImplementation((path: string) => {
    if (path === '/notifications') return Promise.resolve(page());
    return Promise.resolve({ markedRead: 1, unreadCount: 0 });
  });
  renderScreen();

  await waitFor(() => expect(screen.getByText('Mark all as read')).toBeTruthy());
  fireEvent.press(screen.getByText('Mark all as read'));

  await waitFor(() =>
    expect(mockApiRequest).toHaveBeenCalledWith('/notifications/read-all', { method: 'POST' }),
  );
});

it('offers a retry rather than blank space when the inbox fails to load', async () => {
  mockApiRequest.mockRejectedValue(new Error('network'));
  renderScreen();

  await waitFor(() =>
    expect(screen.getByText('We could not load your notifications')).toBeTruthy(),
  );
  expect(screen.getByText('Try again')).toBeTruthy();
});

it('offers the next page only when there is one', async () => {
  mockApiRequest.mockResolvedValue(page({ nextCursor: 'cursor-2' }));
  renderScreen();

  await waitFor(() => expect(screen.getByText('Show more')).toBeTruthy());

  fireEvent.press(screen.getByText('Show more'));

  await waitFor(() =>
    expect(mockApiRequest).toHaveBeenCalledWith('/notifications?cursor=cursor-2'),
  );
});

it('never shows a raw server error to the reader', async () => {
  mockApiRequest.mockRejectedValue(new Error('pq: relation "notifications" does not exist'));
  renderScreen();

  await waitFor(() =>
    expect(screen.getByText('We could not load your notifications')).toBeTruthy(),
  );
  expect(screen.queryByText(/relation "notifications"/)).toBeNull();
});
