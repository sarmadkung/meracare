import type { CareTask } from '@genxcare/contracts';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react-native';
import type { ReactNode } from 'react';

import { ApiError } from '@/lib/api-error';

import { taskKeys, useCompleteTask, useSkipTask } from '../use-tasks';

/**
 * Completing a task updates the list immediately and the request follows
 * (plans/phase4.md §23). Two failures matter more than the happy path:
 *
 *   - a server refusal must put the old state back, or the screen keeps
 *     claiming care happened when it did not;
 *   - a lost connection must NOT roll back, because the action is queued and
 *     will be sent.
 */

const mockApiRequest = jest.fn();
const mockEnqueue = jest.fn();

jest.mock('@/lib/api-client', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}));

jest.mock('@/lib/offline/database', () => ({
  sqliteSyncStore: { enqueue: (...args: unknown[]) => mockEnqueue(...args) },
  cacheTasks: jest.fn(),
  cachedTasks: jest.fn(async () => []),
}));

function task(overrides: Partial<CareTask> = {}): CareTask {
  return {
    id: 'task-1',
    templateId: null,
    seniorId: 'senior-1',
    title: 'Morning walk',
    description: null,
    assignedUserId: null,
    scheduledFor: '2026-08-17T04:00:00Z',
    status: 'pending',
    recurring: false,
    completedAt: null,
    completedBy: null,
    skippedAt: null,
    skippedBy: null,
    notes: null,
    createdAt: '2026-08-16T09:00:00Z',
    updatedAt: '2026-08-16T09:00:00Z',
    ...overrides,
  };
}

const TODAY_KEY = ['seniors', 'senior-1', 'tasks', 'today'] as const;

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

function setup() {
  const queryClient = new QueryClient({
    // Cache entries are seeded directly and have no observer, so they must not
    // be garbage-collected before the assertions read them back.
    defaultOptions: {
      queries: { retry: false, gcTime: Infinity },
      mutations: { retry: false },
    },
  });
  clients.push(queryClient);

  queryClient.setQueryData(TODAY_KEY, [task()]);
  queryClient.setQueryData(taskKeys.detail('task-1'), task());

  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );

  return { queryClient, wrapper };
}

function statusInList(queryClient: QueryClient): string | undefined {
  return queryClient.getQueryData<CareTask[]>(TODAY_KEY)?.[0]?.status;
}

beforeEach(() => {
  mockApiRequest.mockReset();
  mockEnqueue.mockReset();
});

describe('useCompleteTask', () => {
  it('shows the task as done before the server has answered', async () => {
    const { queryClient, wrapper } = setup();

    // Hold the request open, so the optimistic state is observable.
    let resolve: ((value: CareTask) => void) | undefined;
    mockApiRequest.mockReturnValue(
      new Promise<CareTask>((r) => {
        resolve = r;
      }),
    );

    const { result } = renderHook(() => useCompleteTask('senior-1'), { wrapper });
    result.current.mutate({ taskId: 'task-1' });

    await waitFor(() => expect(statusInList(queryClient)).toBe('completed'));

    // And the detail view agrees.
    expect(queryClient.getQueryData<CareTask>(taskKeys.detail('task-1'))?.status).toBe('completed');

    resolve?.(task({ status: 'completed' }));
    await waitFor(() => expect(result.current.isPending).toBe(false));
  });

  // The whole point of the rollback: never leave a completion on screen that
  // the server refused.
  it('puts the task back when the server refuses', async () => {
    const { queryClient, wrapper } = setup();

    mockApiRequest.mockRejectedValue(
      new ApiError(409, 'CONFLICT', 'Somebody has already recorded a different outcome.'),
    );

    const { result } = renderHook(() => useCompleteTask('senior-1'), { wrapper });
    result.current.mutate({ taskId: 'task-1' });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(statusInList(queryClient)).toBe('pending');
    expect(queryClient.getQueryData<CareTask>(taskKeys.detail('task-1'))?.status).toBe('pending');
  });

  // Offline is not a failure. The action is recorded locally and will be sent,
  // so rolling it back would tell the user their work was lost when it was not.
  it('queues the action when the phone is offline and keeps it on screen', async () => {
    const { queryClient, wrapper } = setup();

    mockApiRequest.mockRejectedValue(ApiError.network(new Error('offline')));

    const { result } = renderHook(() => useCompleteTask('senior-1'), { wrapper });
    result.current.mutate({ taskId: 'task-1' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockEnqueue).toHaveBeenCalledTimes(1);
    expect(mockEnqueue.mock.calls[0]?.[0]).toMatchObject({
      entityId: 'task-1',
      entityType: 'task',
      operationType: 'complete',
      status: 'pending',
    });
    expect(statusInList(queryClient)).toBe('completed');
  });
});

describe('useSkipTask', () => {
  it('shows the task as skipped straight away', async () => {
    const { queryClient, wrapper } = setup();
    mockApiRequest.mockResolvedValue(task({ status: 'skipped' }));

    const { result } = renderHook(() => useSkipTask('senior-1'), { wrapper });
    result.current.mutate({ taskId: 'task-1', notes: 'She was asleep' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(statusInList(queryClient)).toBe('skipped');
  });

  it('queues a skip with its note when offline', async () => {
    const { wrapper } = setup();
    mockApiRequest.mockRejectedValue(ApiError.network(new Error('offline')));

    const { result } = renderHook(() => useSkipTask('senior-1'), { wrapper });
    result.current.mutate({ taskId: 'task-1', notes: 'She was asleep' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const queued = mockEnqueue.mock.calls[0]?.[0] as { operationType: string; payload: string };
    expect(queued.operationType).toBe('skip');
    expect(JSON.parse(queued.payload)).toEqual({ notes: 'She was asleep' });
  });

  it('puts the task back when a skip is refused', async () => {
    const { queryClient, wrapper } = setup();
    mockApiRequest.mockRejectedValue(new ApiError(404, 'NOT_FOUND', 'That task does not exist.'));

    const { result } = renderHook(() => useSkipTask('senior-1'), { wrapper });
    result.current.mutate({ taskId: 'task-1' });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(statusInList(queryClient)).toBe('pending');
  });
});
