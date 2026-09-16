import type { CareEvent } from '@genxcare/contracts';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react-native';
import type { ReactNode } from 'react';

import { ApiError } from '@/lib/api-error';

import { useActivity } from '../use-activity';

/**
 * The timeline is read-only and paged. What is worth pinning is that it pages
 * without repeating, and that a failure reads as a failure rather than as an
 * empty history — "nothing has happened" and "we could not load it" mean very
 * different things to somebody checking on a relative.
 */

const mockApiRequest = jest.fn();

jest.mock('@/lib/api-client', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}));

function event(overrides: Partial<CareEvent> = {}): CareEvent {
  return {
    id: 'event-1',
    seniorId: 'senior-1',
    type: 'TASK_COMPLETED',
    actorUserId: 'user-1',
    entityType: 'task',
    entityId: 'task-1',
    metadata: { taskTitle: 'Morning walk' },
    occurredAt: '2026-08-18T05:42:00Z',
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

describe('useActivity', () => {
  it('loads the first page', async () => {
    const queryClient = newClient();
    mockApiRequest.mockResolvedValue({ items: [event()], nextCursor: null });

    const { result } = renderHook(() => useActivity('senior-1'), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiRequest).toHaveBeenCalledWith('/seniors/senior-1/activity');
    expect(result.current.data?.pages[0]?.items).toHaveLength(1);
  });

  it('shows an empty timeline as empty, not as a failure', async () => {
    const queryClient = newClient();
    mockApiRequest.mockResolvedValue({ items: [], nextCursor: null });

    const { result } = renderHook(() => useActivity('senior-1'), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.pages.flatMap((page) => page.items)).toEqual([]);
    expect(result.current.isError).toBe(false);
  });

  it('reports a failure rather than an empty history', async () => {
    const queryClient = newClient();
    mockApiRequest.mockRejectedValue(new ApiError(500, 'INTERNAL', 'Something went wrong.'));

    const { result } = renderHook(() => useActivity('senior-1'), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.data).toBeUndefined();
  });

  it('surfaces a timeline the caller may not read as an error', async () => {
    const queryClient = newClient();
    mockApiRequest.mockRejectedValue(
      new ApiError(404, 'NOT_FOUND', 'That senior does not exist, or you do not have access.'),
    );

    const { result } = renderHook(() => useActivity('senior-1'), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect((result.current.error as ApiError).status).toBe(404);
  });

  it('follows the cursor and stops at the end', async () => {
    const queryClient = newClient();

    mockApiRequest
      .mockResolvedValueOnce({ items: [event({ id: 'event-1' })], nextCursor: 'cursor-1' })
      .mockResolvedValueOnce({ items: [event({ id: 'event-2' })], nextCursor: null });

    const { result } = renderHook(() => useActivity('senior-1'), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.hasNextPage).toBe(true);

    await result.current.fetchNextPage();

    await waitFor(() => expect(result.current.hasNextPage).toBe(false));
    expect(mockApiRequest.mock.calls[1]?.[0]).toBe('/seniors/senior-1/activity?cursor=cursor-1');

    const ids = result.current.data?.pages.flatMap((page) => page.items.map((item) => item.id));
    expect(ids).toEqual(['event-1', 'event-2']);
  });

  it('does not repeat an event across pages', async () => {
    const queryClient = newClient();

    // Keyed on the request rather than call order, so the assertion is about
    // what the hook asks for rather than how many times it happens to render.
    mockApiRequest.mockImplementation((path: string) =>
      path.includes('cursor=')
        ? Promise.resolve({ items: [event({ id: 'c' })], nextCursor: null })
        : Promise.resolve({
            items: [event({ id: 'a' }), event({ id: 'b' })],
            nextCursor: 'cursor-1',
          }),
    );

    const { result } = renderHook(() => useActivity('senior-1'), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    await waitFor(() => expect(result.current.hasNextPage).toBe(true));

    await result.current.fetchNextPage();
    await waitFor(() => expect(result.current.data?.pages).toHaveLength(2));

    const ids = result.current.data?.pages.flatMap((page) => page.items.map((item) => item.id));
    expect(ids).toEqual(['a', 'b', 'c']);
    // The whole point of a keyset cursor: walking the feed visits each event
    // exactly once, with no repeats at a page boundary.
    expect(new Set(ids).size).toBe(ids?.length);
  });

  it('asks for nothing until a senior is chosen', () => {
    const queryClient = newClient();

    const { result } = renderHook(() => useActivity(null), {
      wrapper: wrapperFor(queryClient),
    });

    expect(result.current.fetchStatus).toBe('idle');
    expect(mockApiRequest).not.toHaveBeenCalled();
  });
});
