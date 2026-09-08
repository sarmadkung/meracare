import { QueryClient } from '@tanstack/react-query';

import { ApiError } from '@/lib/api-error';

import { recordMedicationNotificationAction } from '../medication-actions';

const mockApiRequest = jest.fn();
const mockEnqueue = jest.fn();

jest.mock('@/lib/api-client', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}));

jest.mock('@/lib/offline/database', () => ({
  sqliteSyncStore: { enqueue: (...args: unknown[]) => mockEnqueue(...args) },
}));

let queryClient: QueryClient;

beforeEach(() => {
  mockApiRequest.mockReset();
  mockEnqueue.mockReset().mockResolvedValue(undefined);
  queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
});

afterEach(() => queryClient.clear());

it('records Taken through the privacy-preserving dose route', async () => {
  mockApiRequest.mockResolvedValue({});

  const outcome = await recordMedicationNotificationAction(
    queryClient,
    'senior-1',
    'dose-1',
    'take',
  );

  expect(outcome).toBe('recorded');
  expect(mockApiRequest).toHaveBeenCalledWith('/medications/instances/dose-1/take', {
    method: 'POST',
  });
  expect(mockEnqueue).not.toHaveBeenCalled();
});

it('durably queues an offline notification action', async () => {
  mockApiRequest.mockRejectedValue(ApiError.network(new Error('offline')));

  const outcome = await recordMedicationNotificationAction(
    queryClient,
    'senior-1',
    '2fb53d18-8ec8-4f21-9088-53a06f7647f6',
    'take',
  );

  expect(outcome).toBe('queued');
  expect(mockEnqueue).toHaveBeenCalledWith(
    expect.objectContaining({
      entityType: 'medication',
      entityId: '2fb53d18-8ec8-4f21-9088-53a06f7647f6',
      operationType: 'take',
    }),
  );
});

it('does not hide an authorization or transition error in the offline queue', async () => {
  const denied = new ApiError(404, 'NOT_FOUND', 'Not found');
  mockApiRequest.mockRejectedValue(denied);

  await expect(
    recordMedicationNotificationAction(queryClient, 'senior-1', 'dose-1', 'skip'),
  ).rejects.toBe(denied);
  expect(mockEnqueue).not.toHaveBeenCalled();
});
