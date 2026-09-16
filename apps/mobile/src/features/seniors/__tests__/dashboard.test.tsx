import type { Senior } from '@genxcare/contracts';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react-native';
import type { ReactNode } from 'react';
import { SafeAreaProvider } from 'react-native-safe-area-context';

import SeniorDashboardScreen from '@/app/seniors/[seniorId]/index';
import { ApiError } from '@/lib/api-error';
import { ThemeProvider } from '@/theme';

/**
 * The senior dashboard is the screen a family opens every day, so what is
 * pinned here is what it must never do: show a section the reader has no
 * permission for, leave a failed request as blank space, or greet a brand-new
 * circle with three empty boxes and no way forward.
 */

const mockApiRequest = jest.fn();

jest.mock('@/lib/api-client', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}));

// The dashboard's queries write today's data into the offline cache on the way
// through. That is expo-sqlite, which does not exist here, and a cache miss
// would send the query down its offline fallback path and show nothing.
jest.mock('@/lib/offline/database', () => ({
  sqliteSyncStore: { enqueue: jest.fn() },
  cacheDoses: jest.fn(),
  cachedDoses: jest.fn(async () => []),
  cacheTasks: jest.fn(),
  cachedTasks: jest.fn(async () => []),
  cacheAppointments: jest.fn(),
  cachedAppointments: jest.fn(async () => []),
}));

jest.mock('expo-router', () => ({
  Stack: { Screen: () => null },
  router: { push: jest.fn(), replace: jest.fn() },
  useLocalSearchParams: () => ({ seniorId: 'senior-1' }),
}));

function senior(overrides: Partial<Senior> = {}): Senior {
  return {
    id: 'senior-1',
    displayName: 'Amma',
    dateOfBirth: null,
    photoUrl: null,
    phone: null,
    address: null,
    emergencyContact: null,
    timezone: 'Asia/Karachi',
    isSelf: false,
    role: 'family_member',
    permissions: [
      'senior.view',
      'tasks.view',
      'medications.view',
      'appointments.view',
      'activity.view',
      'members.view',
    ],
    createdAt: '2026-08-01T00:00:00Z',
    updatedAt: '2026-08-01T00:00:00Z',
    ...overrides,
  };
}

/**
 * Routes each request to a canned answer.
 *
 * Matched on the whole path rather than a substring, so an entry for
 * `/seniors/senior-1` answers the profile request only and does not also
 * swallow `/seniors/senior-1/tasks`. Anything unlisted comes back empty.
 */
function respondWith(answers: Record<string, unknown>) {
  mockApiRequest.mockImplementation((path: string) => {
    for (const [pattern, answer] of Object.entries(answers)) {
      if (path === pattern || path.startsWith(`${pattern}?`)) {
        return answer instanceof Error ? Promise.reject(answer) : Promise.resolve(answer);
      }
    }
    return Promise.resolve({ items: [] });
  });
}

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

  return render(<SeniorDashboardScreen />, { wrapper: Wrapper });
}

beforeEach(() => {
  mockApiRequest.mockReset();
});

it('summarises today across every domain the reader may see', async () => {
  respondWith({
    '/seniors/senior-1/medications/doses': {
      items: [
        {
          id: 'dose-1',
          medicationId: 'med-1',
          seniorId: 'senior-1',
          name: 'Metformin',
          dosage: '500 mg',
          scheduledFor: '2026-08-19T03:00:00Z',
          status: 'pending',
          recurring: true,
          takenAt: null,
          takenBy: null,
          skippedAt: null,
          skippedBy: null,
          notes: null,
        },
      ],
    },
    '/seniors/senior-1/tasks': {
      items: [
        {
          id: 'task-1',
          templateId: null,
          seniorId: 'senior-1',
          title: 'Morning walk',
          description: null,
          assignedUserId: null,
          scheduledFor: '2026-08-19T04:00:00Z',
          status: 'pending',
          recurring: false,
          completedAt: null,
          completedBy: null,
          skippedAt: null,
          skippedBy: null,
          notes: null,
          createdAt: '2026-08-19T00:00:00Z',
          updatedAt: '2026-08-19T00:00:00Z',
        },
      ],
    },
    '/seniors/senior-1': senior(),
  });

  renderScreen();

  await waitFor(() => expect(screen.getByText('Metformin')).toBeTruthy());
  expect(screen.getByText('Morning walk')).toBeTruthy();
  expect(screen.getByText('Medication today')).toBeTruthy();
  expect(screen.getByText('Care tasks today')).toBeTruthy();
  expect(screen.getByText('Appointments today')).toBeTruthy();
});

it('omits every section the reader has no permission for', async () => {
  // A caregiver granted appointments only. Hiding is a courtesy — the API
  // refuses regardless — but showing a medication section she cannot open
  // would be a promise the app cannot keep.
  respondWith({
    '/seniors/senior-1': senior({
      permissions: ['senior.view', 'appointments.view'],
      role: 'professional_caregiver',
    }),
  });

  renderScreen();

  await waitFor(() => expect(screen.getByText('Appointments today')).toBeTruthy());
  expect(screen.queryByText('Medication today')).toBeNull();
  expect(screen.queryByText('Care tasks today')).toBeNull();
  expect(screen.queryByText('Recent activity')).toBeNull();
  // The section, not the subtitle under the heading, which also reads
  // "Care circle" for anyone who is not the senior themselves.
  expect(screen.queryByText('View care circle')).toBeNull();
});

it('offers a way forward when nothing has been set up yet', async () => {
  respondWith({
    '/seniors/senior-1': senior({
      permissions: [
        'senior.view',
        'tasks.view',
        'tasks.manage',
        'medications.view',
        'medications.manage',
        'appointments.view',
        'appointments.manage',
        'members.view',
        'members.invite',
      ],
    }),
  });

  renderScreen();

  await waitFor(() => expect(screen.getByText('Getting started')).toBeTruthy());
  expect(screen.getByText('Add a medication')).toBeTruthy();
  expect(screen.getByText('Add a care task')).toBeTruthy();
  expect(screen.getByText('Invite someone to help')).toBeTruthy();
});

it('suggests only the steps the reader is allowed to take', async () => {
  // A caregiver who can record care but not create it must not be invited to
  // add a medicine the API would refuse.
  respondWith({
    '/seniors/senior-1': senior({
      role: 'professional_caregiver',
      permissions: ['senior.view', 'tasks.view', 'tasks.complete', 'medications.view'],
    }),
  });

  renderScreen();

  await waitFor(() => expect(screen.getByText('Care tasks today')).toBeTruthy());
  expect(screen.queryByText('Add a medication')).toBeNull();
  expect(screen.queryByText('Add a care task')).toBeNull();
  expect(screen.queryByText('Invite someone to help')).toBeNull();
});

it('shows a retry rather than blank space when a section fails', async () => {
  respondWith({
    '/seniors/senior-1/medications/doses': new Error('network'),
    '/seniors/senior-1': senior(),
  });

  renderScreen();

  await waitFor(() => expect(screen.getByText('We could not load this just now.')).toBeTruthy());
  expect(screen.getAllByText('Try again').length).toBeGreaterThan(0);
});

it('never puts a raw server error in front of the reader', async () => {
  respondWith({
    '/seniors/senior-1/tasks': new ApiError(500, 'INTERNAL', 'pq: SQLSTATE 28P01 relation missing'),
    '/seniors/senior-1': senior(),
  });

  renderScreen();

  await waitFor(() => expect(screen.getByText('We could not load this just now.')).toBeTruthy());
  expect(screen.queryByText(/SQLSTATE/)).toBeNull();
});

it('says the profile is unavailable when access has gone', async () => {
  // What a revoked caregiver sees. The API answers 404 for a senior that does
  // not exist and one they may not see, deliberately, so the wording must not
  // guess between them.
  respondWith({
    '/seniors/senior-1': new ApiError(404, 'NOT_FOUND', 'That senior does not exist.'),
  });

  renderScreen();

  await waitFor(() => expect(screen.getByText('This profile is not available')).toBeTruthy());
  expect(screen.getByText('You may no longer be part of this care circle.')).toBeTruthy();
});

it('still offers guidance to a reader who can see only part of the care', async () => {
  // A query the reader has no permission for is disabled, and a disabled query
  // never stops being pending. Waiting on all three would mean this reader —
  // who can manage appointments and nothing else — never saw a way in.
  respondWith({
    '/seniors/senior-1': senior({
      permissions: ['senior.view', 'appointments.view', 'appointments.manage'],
    }),
  });

  renderScreen();

  await waitFor(() => expect(screen.getByText('Getting started')).toBeTruthy());
  expect(screen.getByText('Add an appointment')).toBeTruthy();
  expect(screen.queryByText('Add a medication')).toBeNull();
});
