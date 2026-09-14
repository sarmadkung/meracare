import type { Appointment } from '@genxcare/contracts';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react-native';
import type { ReactNode } from 'react';

import { ApiError } from '@/lib/api-error';

import {
  appointmentKeys,
  useAppointment,
  useAppointmentHistory,
  useAppointments,
  useCancelAppointment,
  useCompleteAppointment,
  useCreateAppointment,
  useUpdateAppointment,
} from '../use-appointments';

/**
 * Appointments are read offline and only changed online (plans/phase6.md §23),
 * which makes two behaviours worth pinning:
 *
 *   - the upcoming list must survive a lost connection, because that is exactly
 *     when somebody is in a car looking for a hospital address;
 *   - a refused cancellation must put the appointment back, or the screen keeps
 *     claiming a visit was called off when it was not.
 */

const mockApiRequest = jest.fn();
const mockCachedAppointments = jest.fn(async () => [] as Appointment[]);

jest.mock('@/lib/api-client', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}));

jest.mock('@/lib/offline/database', () => ({
  cacheAppointments: jest.fn(),
  cachedAppointments: (...args: unknown[]) => mockCachedAppointments(...(args as [])),
}));

function appointment(overrides: Partial<Appointment> = {}): Appointment {
  return {
    id: 'appointment-1',
    seniorId: 'senior-1',
    title: 'Cardiology review',
    kind: 'doctor_visit',
    providerName: 'Dr Ahmed',
    location: 'City Hospital',
    notes: null,
    assignedUserId: null,
    scheduledAt: '2026-08-20T04:30:00Z',
    endsAt: null,
    status: 'scheduled',
    completedAt: null,
    completedBy: null,
    cancelledAt: null,
    cancelledBy: null,
    createdBy: 'user-1',
    createdAt: '2026-08-17T09:00:00Z',
    updatedAt: '2026-08-17T09:00:00Z',
    ...overrides,
  };
}

const UPCOMING_KEY = appointmentKeys.forSenior('senior-1', 'upcoming');

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
  queryClient.setQueryData(UPCOMING_KEY, [appointment()]);
  return { queryClient, wrapper: wrapperFor(queryClient) };
}

function statusInList(queryClient: QueryClient): string | undefined {
  return queryClient.getQueryData<Appointment[]>(UPCOMING_KEY)?.[0]?.status;
}

beforeEach(() => {
  mockApiRequest.mockReset();
  mockCachedAppointments.mockClear();
});

describe('useAppointments', () => {
  it('loads the upcoming appointments by default', async () => {
    const queryClient = newClient();
    mockApiRequest.mockResolvedValue({ items: [appointment()], nextCursor: null });

    const { result } = renderHook(() => useAppointments('senior-1'), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(1);
    expect(mockApiRequest).toHaveBeenCalledWith('/seniors/senior-1/appointments?scope=upcoming');
  });

  it('asks for the senior‘s own day when told to', async () => {
    const queryClient = newClient();
    mockApiRequest.mockResolvedValue({ items: [], nextCursor: null });

    const { result } = renderHook(() => useAppointments('senior-1', 'today'), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiRequest).toHaveBeenCalledWith('/seniors/senior-1/appointments?scope=today');
  });

  it('shows an empty calendar as empty, not as a failure', async () => {
    const queryClient = newClient();
    mockApiRequest.mockResolvedValue({ items: [], nextCursor: null });

    const { result } = renderHook(() => useAppointments('senior-1'), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([]);
  });

  it('reports a failure rather than showing an empty list', async () => {
    const queryClient = newClient();
    mockApiRequest.mockRejectedValue(new ApiError(500, 'INTERNAL', 'Something went wrong.'));

    const { result } = renderHook(() => useAppointments('senior-1'), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.data).toBeUndefined();
  });

  // The list somebody needs on the way to the hospital.
  it('falls back to the cached list when the phone is offline', async () => {
    const queryClient = newClient();
    mockApiRequest.mockRejectedValue(ApiError.network(new Error('offline')));
    mockCachedAppointments.mockResolvedValue([appointment({ id: 'cached-1' })]);

    const { result } = renderHook(() => useAppointments('senior-1'), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.[0]?.id).toBe('cached-1');
  });

  // Only the upcoming list is cached. Answering "today" from stale local rows
  // would be worse than saying we could not load it.
  it('does not answer other views from the cache', async () => {
    const queryClient = newClient();
    mockApiRequest.mockRejectedValue(ApiError.network(new Error('offline')));

    const { result } = renderHook(() => useAppointments('senior-1', 'today'), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(mockCachedAppointments).not.toHaveBeenCalled();
  });

  it('asks for nothing until a senior is chosen', () => {
    const queryClient = newClient();

    const { result } = renderHook(() => useAppointments(null), {
      wrapper: wrapperFor(queryClient),
    });

    expect(result.current.fetchStatus).toBe('idle');
    expect(mockApiRequest).not.toHaveBeenCalled();
  });
});

describe('useAppointmentHistory', () => {
  it('reads the first page and follows the cursor', async () => {
    const queryClient = newClient();

    mockApiRequest
      .mockResolvedValueOnce({ items: [appointment({ id: 'a-2' })], nextCursor: 'cursor-1' })
      .mockResolvedValueOnce({ items: [appointment({ id: 'a-3' })], nextCursor: null });

    const { result } = renderHook(() => useAppointmentHistory('senior-1'), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.hasNextPage).toBe(true);
    expect(mockApiRequest.mock.calls[0]?.[0]).toBe('/seniors/senior-1/appointments?scope=past');

    await result.current.fetchNextPage();

    await waitFor(() => expect(result.current.hasNextPage).toBe(false));
    expect(result.current.data?.pages.flatMap((page) => page.items)).toHaveLength(2);
    expect(mockApiRequest.mock.calls[1]?.[0]).toBe(
      '/seniors/senior-1/appointments?scope=past&cursor=cursor-1',
    );
  });

  it('stops at the end of the history', async () => {
    const queryClient = newClient();
    mockApiRequest.mockResolvedValue({ items: [appointment()], nextCursor: null });

    const { result } = renderHook(() => useAppointmentHistory('senior-1'), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.hasNextPage).toBe(false);
  });
});

describe('useAppointment', () => {
  it('loads one appointment', async () => {
    const queryClient = newClient();
    mockApiRequest.mockResolvedValue(appointment());

    const { result } = renderHook(() => useAppointment('appointment-1'), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.title).toBe('Cardiology review');
    expect(mockApiRequest).toHaveBeenCalledWith('/appointments/appointment-1');
  });

  it('surfaces an appointment somebody may not see as an error', async () => {
    const queryClient = newClient();
    mockApiRequest.mockRejectedValue(
      new ApiError(404, 'NOT_FOUND', 'That appointment does not exist, or you do not have access.'),
    );

    const { result } = renderHook(() => useAppointment('appointment-1'), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});

describe('useCreateAppointment', () => {
  it('books an appointment and refreshes the lists', async () => {
    const { queryClient, wrapper } = setup();
    mockApiRequest.mockResolvedValue(appointment({ id: 'appointment-2' }));

    const { result } = renderHook(() => useCreateAppointment('senior-1'), { wrapper });
    result.current.mutate({ title: 'Blood test', scheduledAt: '2026-08-21T05:00:00Z' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const [path, options] = mockApiRequest.mock.calls[0] as [
      string,
      { method: string; body: unknown },
    ];
    expect(path).toBe('/seniors/senior-1/appointments');
    expect(options.method).toBe('POST');
    expect(queryClient.getQueryState(UPCOMING_KEY)?.isInvalidated).toBe(true);
  });

  it('reports a validation failure with its field details', async () => {
    const { wrapper } = setup();
    mockApiRequest.mockRejectedValue(
      new ApiError(422, 'VALIDATION_FAILED', 'Please check the highlighted fields.', {
        scheduledAt: 'Give the date and time as an ISO-8601 timestamp.',
      }),
    );

    const { result } = renderHook(() => useCreateAppointment('senior-1'), { wrapper });
    result.current.mutate({ title: 'Blood test', scheduledAt: 'next Tuesday' });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect((result.current.error as ApiError).details?.scheduledAt).toBeTruthy();
  });
});

describe('useUpdateAppointment', () => {
  it('saves an edit and seeds the detail cache with the answer', async () => {
    const { queryClient, wrapper } = setup();
    mockApiRequest.mockResolvedValue(appointment({ title: 'Cardiology review, moved' }));

    const { result } = renderHook(() => useUpdateAppointment('senior-1'), { wrapper });
    result.current.mutate({ appointmentId: 'appointment-1', title: 'Cardiology review, moved' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const [path, options] = mockApiRequest.mock.calls[0] as [string, { method: string }];
    expect(path).toBe('/appointments/appointment-1');
    expect(options.method).toBe('PATCH');
    expect(
      queryClient.getQueryData<Appointment>(appointmentKeys.detail('appointment-1'))?.title,
    ).toBe('Cardiology review, moved');
  });

  // Editing an appointment that has already been settled is refused by the
  // server, and the screen has to show that rather than pretend it landed.
  it('surfaces the conflict when the appointment has already been settled', async () => {
    const { wrapper } = setup();
    mockApiRequest.mockRejectedValue(
      new ApiError(409, 'CONFLICT', 'This appointment has already happened or been called off.'),
    );

    const { result } = renderHook(() => useUpdateAppointment('senior-1'), { wrapper });
    result.current.mutate({ appointmentId: 'appointment-1', title: 'Rewritten' });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect((result.current.error as ApiError).status).toBe(409);
  });
});

describe('useCancelAppointment', () => {
  it('shows the appointment as cancelled before the server has answered', async () => {
    const { queryClient, wrapper } = setup();

    // Hold the request open, so the optimistic state is observable.
    let resolve: ((value: Appointment) => void) | undefined;
    mockApiRequest.mockReturnValue(
      new Promise<Appointment>((r) => {
        resolve = r;
      }),
    );

    const { result } = renderHook(() => useCancelAppointment('senior-1'), { wrapper });
    result.current.mutate({ appointmentId: 'appointment-1' });

    await waitFor(() => expect(statusInList(queryClient)).toBe('cancelled'));

    resolve?.(appointment({ status: 'cancelled' }));
    await waitFor(() => expect(result.current.isPending).toBe(false));
  });

  // The whole point of the rollback: never leave an appointment on screen as
  // cancelled when the server refused it.
  it('puts the appointment back when the server refuses', async () => {
    const { queryClient, wrapper } = setup();
    mockApiRequest.mockRejectedValue(
      new ApiError(409, 'CONFLICT', 'Somebody has already recorded a different outcome.'),
    );

    const { result } = renderHook(() => useCancelAppointment('senior-1'), { wrapper });
    result.current.mutate({ appointmentId: 'appointment-1' });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(statusInList(queryClient)).toBe('scheduled');
  });

  // Nothing queues an appointment mutation, so a lost connection is a genuine
  // failure and the optimistic change must not survive it.
  it('puts the appointment back when the phone is offline', async () => {
    const { queryClient, wrapper } = setup();
    mockApiRequest.mockRejectedValue(ApiError.network(new Error('offline')));

    const { result } = renderHook(() => useCancelAppointment('senior-1'), { wrapper });
    result.current.mutate({ appointmentId: 'appointment-1' });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(statusInList(queryClient)).toBe('scheduled');
  });
});

describe('useCompleteAppointment', () => {
  it('records that the appointment happened', async () => {
    const { queryClient, wrapper } = setup();
    mockApiRequest.mockResolvedValue(appointment({ status: 'completed' }));

    const { result } = renderHook(() => useCompleteAppointment('senior-1'), { wrapper });
    result.current.mutate({ appointmentId: 'appointment-1' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(statusInList(queryClient)).toBe('completed');
    expect(mockApiRequest.mock.calls[0]?.[0]).toBe('/appointments/appointment-1/complete');
  });
});
