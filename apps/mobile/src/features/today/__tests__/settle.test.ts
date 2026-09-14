import type { TodayItem } from '@genxcare/contracts';
import { QueryClient } from '@tanstack/react-query';

import { ApiError } from '@/lib/api-error';

import { settleAgendaItem } from '../settle';
import { todayKeys } from '../use-today';

/**
 * Recording an outcome from Today itself.
 *
 * The screen shows work across every circle, so it cannot address a dose
 * through its medication the way the medication screens do — it never loaded
 * one. It uses the by-instance route built for notification actions, which
 * resolves the dose under the caller's own authorization.
 */

const mockApiRequest = jest.fn();
const mockEnqueue = jest.fn();

jest.mock('@/lib/api-client', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}));

jest.mock('@/lib/offline/database', () => ({
  sqliteSyncStore: { enqueue: (...args: unknown[]) => mockEnqueue(...args) },
}));

let queryClient: QueryClient;

function cachedDay(overrides: Partial<TodayItem> = {}): TodayItem[] {
  return [
    {
      kind: 'dose',
      id: 'dose-1',
      seniorId: 'senior-1',
      seniorName: 'Amina Bibi',
      timezone: 'Asia/Karachi',
      title: 'Metformin',
      detail: '500 mg',
      scheduledFor: '2026-09-08T09:00:00.000Z',
      status: 'pending',
      assignedToMe: false,
      ...overrides,
    },
  ];
}

function cachedStatus(): string | undefined {
  return queryClient.getQueryData<TodayItem[]>(todayKeys.all)?.[0]?.status;
}

beforeEach(() => {
  mockApiRequest.mockReset();
  mockEnqueue.mockReset().mockResolvedValue(undefined);
  queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  queryClient.setQueryData(todayKeys.all, cachedDay());
});

afterEach(() => queryClient.clear());

it('records a dose taken without needing the medication it belongs to', async () => {
  mockApiRequest.mockResolvedValue({});

  const outcome = await settleAgendaItem(
    queryClient,
    { kind: 'dose', id: 'dose-1', seniorId: 'senior-1' },
    'done',
  );

  expect(outcome).toBe('recorded');
  expect(mockApiRequest).toHaveBeenCalledWith('/medications/instances/dose-1/take', {
    method: 'POST',
  });
});

it('records a dose deliberately not taken', async () => {
  mockApiRequest.mockResolvedValue({});

  await settleAgendaItem(queryClient, { kind: 'dose', id: 'dose-1', seniorId: 's1' }, 'skipped');

  expect(mockApiRequest).toHaveBeenCalledWith('/medications/instances/dose-1/skip', {
    method: 'POST',
  });
});

it('records a task done', async () => {
  mockApiRequest.mockResolvedValue({});

  await settleAgendaItem(queryClient, { kind: 'task', id: 'task-1', seniorId: 's1' }, 'done');

  expect(mockApiRequest).toHaveBeenCalledWith('/tasks/task-1/complete', { method: 'POST' });
});

/**
 * The most frequent action in the app, so the row settles on the tap and the
 * request follows (plans/phase5.md §17).
 */
it('shows the row settled before the server has answered', async () => {
  let release: (value: unknown) => void = () => {};
  mockApiRequest.mockReturnValue(
    new Promise((resolve) => {
      release = resolve;
    }),
  );

  const pending = settleAgendaItem(
    queryClient,
    { kind: 'dose', id: 'dose-1', seniorId: 's1' },
    'done',
  );

  expect(cachedStatus()).toBe('taken');

  release({});
  await pending;
});

it('puts the row back when the server refuses', async () => {
  mockApiRequest.mockRejectedValue(new ApiError(403, 'FORBIDDEN', 'You cannot record this.'));

  await expect(
    settleAgendaItem(queryClient, { kind: 'dose', id: 'dose-1', seniorId: 's1' }, 'done'),
  ).rejects.toThrow();

  expect(cachedStatus()).toBe('pending');
});

/**
 * A queued action is not a lost one, so the optimistic state stands. Rolling it
 * back would tell the person their record was lost, which is the opposite of
 * what happened.
 */
it('keeps the row settled when the phone is offline, and queues it', async () => {
  mockApiRequest.mockRejectedValue(ApiError.network(new Error('offline')));

  const outcome = await settleAgendaItem(
    queryClient,
    { kind: 'dose', id: 'dose-1', seniorId: 's1' },
    'done',
  );

  expect(outcome).toBe('queued');
  expect(cachedStatus()).toBe('taken');
  expect(mockEnqueue).toHaveBeenCalledWith(
    expect.objectContaining({
      entityType: 'medication',
      entityId: 'dose-1',
      operationType: 'take',
    }),
  );
});

it('queues an offline task under its own id', async () => {
  mockApiRequest.mockRejectedValue(ApiError.network(new Error('offline')));

  await settleAgendaItem(queryClient, { kind: 'task', id: 'task-1', seniorId: 's1' }, 'done');

  expect(mockEnqueue).toHaveBeenCalledWith(
    expect.objectContaining({
      entityType: 'task',
      entityId: 'task-1',
      operationType: 'complete',
    }),
  );
});

/** Recording a dose changes the medication screens too, not only this one. */
it('refreshes the care screens the outcome also changed', async () => {
  mockApiRequest.mockResolvedValue({});
  const invalidate = jest.spyOn(queryClient, 'invalidateQueries');

  await settleAgendaItem(queryClient, { kind: 'dose', id: 'dose-1', seniorId: 'senior-9' }, 'done');

  const keys = invalidate.mock.calls.map((call) => JSON.stringify(call[0]?.queryKey));
  expect(keys).toContainEqual(JSON.stringify(['seniors', 'senior-9', 'medicationDoses']));
});
