import type { MedicationDose } from '@genxcare/contracts';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react-native';
import type { ReactNode } from 'react';

import { ApiError } from '@/lib/api-error';

import {
  medicationKeys,
  useMedicationDoses,
  useMedicationHistory,
  useMedications,
  useSkipDose,
  useTakeDose,
} from '../use-medications';

/**
 * Recording a dose updates the list immediately and the request follows
 * (plans/phase5.md §17). Two failures matter more than the happy path:
 *
 *   - a server refusal must put the old state back, or the screen keeps
 *     claiming a medicine was taken when it was not;
 *   - a lost connection must NOT roll back, because the dose is queued and will
 *     be sent.
 */

const mockApiRequest = jest.fn();
const mockEnqueue = jest.fn();
const mockCachedDoses = jest.fn(async () => [] as MedicationDose[]);

jest.mock('@/lib/api-client', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}));

jest.mock('@/lib/offline/database', () => ({
  sqliteSyncStore: { enqueue: (...args: unknown[]) => mockEnqueue(...args) },
  cacheDoses: jest.fn(),
  cachedDoses: (...args: unknown[]) => mockCachedDoses(...(args as [])),
}));

function dose(overrides: Partial<MedicationDose> = {}): MedicationDose {
  return {
    id: 'dose-1',
    scheduleId: 'schedule-1',
    medicationId: 'med-1',
    seniorId: 'senior-1',
    name: 'Metformin',
    dosage: '500 mg',
    scheduledFor: '2026-08-17T03:00:00Z',
    status: 'pending',
    recurring: true,
    takenAt: null,
    takenBy: null,
    skippedAt: null,
    skippedBy: null,
    notes: null,
    createdAt: '2026-08-16T09:00:00Z',
    updatedAt: '2026-08-16T09:00:00Z',
    ...overrides,
  };
}

const TODAY_KEY = medicationKeys.doses('senior-1', 'today');

/** Every client made by a test, so none is left holding timers afterwards. */
const clients: QueryClient[] = [];

afterEach(() => {
  // A QueryClient schedules garbage-collection timers for its cache. Left
  // running, they keep the Jest process alive after the last assertion.
  for (const client of clients.splice(0)) {
    client.clear();
    client.unmount();
  }
});

function newClient() {
  const queryClient = new QueryClient({
    // Cache entries are seeded directly and have no observer, so they must not
    // be garbage-collected before the assertions read them back.
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

function setup() {
  const queryClient = newClient();
  queryClient.setQueryData(TODAY_KEY, [dose()]);
  return { queryClient, wrapper: wrapperFor(queryClient) };
}

function statusInList(queryClient: QueryClient): string | undefined {
  return queryClient.getQueryData<MedicationDose[]>(TODAY_KEY)?.[0]?.status;
}

beforeEach(() => {
  mockApiRequest.mockReset();
  mockEnqueue.mockReset();
  mockCachedDoses.mockClear();
});

describe('useMedications', () => {
  it('loads a senior‘s medications', async () => {
    const queryClient = newClient();
    mockApiRequest.mockResolvedValue({
      items: [{ id: 'med-1', name: 'Metformin', active: true }],
    });

    const { result } = renderHook(() => useMedications('senior-1'), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(1);
    expect(mockApiRequest).toHaveBeenCalledWith('/seniors/senior-1/medications');
  });

  it('reports a failure rather than showing an empty list', async () => {
    const queryClient = newClient();
    mockApiRequest.mockRejectedValue(new ApiError(500, 'INTERNAL', 'Something went wrong.'));

    const { result } = renderHook(() => useMedications('senior-1'), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.data).toBeUndefined();
  });

  it('asks for nothing until a senior is chosen', () => {
    const queryClient = newClient();

    const { result } = renderHook(() => useMedications(null), {
      wrapper: wrapperFor(queryClient),
    });

    expect(result.current.fetchStatus).toBe('idle');
    expect(mockApiRequest).not.toHaveBeenCalled();
  });
});

describe('useMedicationDoses', () => {
  it('loads today by default', async () => {
    const queryClient = newClient();
    mockApiRequest.mockResolvedValue({ items: [dose()] });

    const { result } = renderHook(() => useMedicationDoses('senior-1'), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiRequest).toHaveBeenCalledWith('/seniors/senior-1/medications/doses?scope=today');
  });

  it('shows an empty day as empty, not as a failure', async () => {
    const queryClient = newClient();
    mockApiRequest.mockResolvedValue({ items: [] });

    const { result } = renderHook(() => useMedicationDoses('senior-1'), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([]);
  });

  // The list somebody needs while standing in a house with no signal.
  it('falls back to the cached day when the phone is offline', async () => {
    const queryClient = newClient();
    mockApiRequest.mockRejectedValue(ApiError.network(new Error('offline')));
    mockCachedDoses.mockResolvedValue([dose({ id: 'cached-1' })]);

    const { result } = renderHook(() => useMedicationDoses('senior-1'), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.[0]?.id).toBe('cached-1');
  });

  // The cache is for the day somebody is standing in; a missed list read from
  // stale local rows would be worse than saying we could not load it.
  it('does not answer other views from the cache', async () => {
    const queryClient = newClient();
    mockApiRequest.mockRejectedValue(ApiError.network(new Error('offline')));

    const { result } = renderHook(() => useMedicationDoses('senior-1', 'missed'), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(mockCachedDoses).not.toHaveBeenCalled();
  });
});

describe('useTakeDose', () => {
  it('shows the dose as taken before the server has answered', async () => {
    const { queryClient, wrapper } = setup();

    // Hold the request open, so the optimistic state is observable.
    let resolve: ((value: MedicationDose) => void) | undefined;
    mockApiRequest.mockReturnValue(
      new Promise<MedicationDose>((r) => {
        resolve = r;
      }),
    );

    const { result } = renderHook(() => useTakeDose('senior-1'), { wrapper });
    result.current.mutate({ medicationId: 'med-1', doseId: 'dose-1' });

    await waitFor(() => expect(statusInList(queryClient)).toBe('taken'));

    resolve?.(dose({ status: 'taken' }));
    await waitFor(() => expect(result.current.isPending).toBe(false));
  });

  // The whole point of the rollback: never leave a dose on screen as taken when
  // the server refused it.
  it('puts the dose back when the server refuses', async () => {
    const { queryClient, wrapper } = setup();

    mockApiRequest.mockRejectedValue(
      new ApiError(409, 'CONFLICT', 'Somebody has already recorded a different outcome.'),
    );

    const { result } = renderHook(() => useTakeDose('senior-1'), { wrapper });
    result.current.mutate({ medicationId: 'med-1', doseId: 'dose-1' });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(statusInList(queryClient)).toBe('pending');
  });

  // Offline is not a failure. The dose is recorded locally and will be sent, so
  // rolling it back would tell the user their record was lost when it was not.
  it('queues the dose when the phone is offline and keeps it on screen', async () => {
    const { queryClient, wrapper } = setup();

    mockApiRequest.mockRejectedValue(ApiError.network(new Error('offline')));

    const { result } = renderHook(() => useTakeDose('senior-1'), { wrapper });
    result.current.mutate({ medicationId: 'med-1', doseId: 'dose-1' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockEnqueue).toHaveBeenCalledTimes(1);
    expect(mockEnqueue.mock.calls[0]?.[0]).toMatchObject({
      // Both IDs travel, because a dose is addressed through its medication.
      entityId: 'med-1/dose-1',
      entityType: 'medication',
      operationType: 'take',
      status: 'pending',
    });
    expect(statusInList(queryClient)).toBe('taken');
  });
});

describe('useSkipDose', () => {
  it('shows the dose as skipped straight away', async () => {
    const { queryClient, wrapper } = setup();
    mockApiRequest.mockResolvedValue(dose({ status: 'skipped' }));

    const { result } = renderHook(() => useSkipDose('senior-1'), { wrapper });
    result.current.mutate({ medicationId: 'med-1', doseId: 'dose-1', notes: 'She was asleep' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(statusInList(queryClient)).toBe('skipped');
  });

  it('queues a skip with its note when offline', async () => {
    const { wrapper } = setup();
    mockApiRequest.mockRejectedValue(ApiError.network(new Error('offline')));

    const { result } = renderHook(() => useSkipDose('senior-1'), { wrapper });
    result.current.mutate({ medicationId: 'med-1', doseId: 'dose-1', notes: 'She was asleep' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const queued = mockEnqueue.mock.calls[0]?.[0] as { operationType: string; payload: string };
    expect(queued.operationType).toBe('skip');
    expect(JSON.parse(queued.payload)).toEqual({ notes: 'She was asleep' });
  });

  it('puts the dose back when a skip is refused', async () => {
    const { queryClient, wrapper } = setup();
    mockApiRequest.mockRejectedValue(
      new ApiError(404, 'NOT_FOUND', 'That medication does not exist.'),
    );

    const { result } = renderHook(() => useSkipDose('senior-1'), { wrapper });
    result.current.mutate({ medicationId: 'med-1', doseId: 'dose-1' });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(statusInList(queryClient)).toBe('pending');
  });
});

describe('useMedicationHistory', () => {
  it('reads the first page and follows the cursor', async () => {
    const queryClient = newClient();

    mockApiRequest
      .mockResolvedValueOnce({ items: [dose({ id: 'dose-2' })], nextCursor: 'cursor-1' })
      .mockResolvedValueOnce({ items: [dose({ id: 'dose-3' })], nextCursor: null });

    const { result } = renderHook(() => useMedicationHistory('med-1'), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.hasNextPage).toBe(true);

    await result.current.fetchNextPage();

    await waitFor(() => expect(result.current.hasNextPage).toBe(false));
    expect(result.current.data?.pages.flatMap((page) => page.items)).toHaveLength(2);
    expect(mockApiRequest.mock.calls[1]?.[0]).toBe('/medications/med-1/instances?cursor=cursor-1');
  });

  it('stops at the end of the history', async () => {
    const queryClient = newClient();
    mockApiRequest.mockResolvedValue({ items: [dose()], nextCursor: null });

    const { result } = renderHook(() => useMedicationHistory('med-1'), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.hasNextPage).toBe(false);
  });
});
