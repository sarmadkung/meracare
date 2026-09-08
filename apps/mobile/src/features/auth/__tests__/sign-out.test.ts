import { apiRequest } from '@/lib/api-client';
import { ApiError } from '@/lib/api-error';
import { clearOfflineData, queuedOperationCount } from '@/lib/offline/database';
import { syncQueuedOperations } from '@/features/sync/replay';
import { clearReminders } from '@/features/notifications/scheduler';
import { deviceId } from '@/features/notifications/device';

import { prepareForSignOut, SignOutPreparationError } from '../sign-out';

jest.mock('@/lib/api-client', () => ({ apiRequest: jest.fn() }));
jest.mock('@/lib/offline/database', () => ({
  clearOfflineData: jest.fn(),
  queuedOperationCount: jest.fn(),
}));
jest.mock('@/features/sync/replay', () => ({ syncQueuedOperations: jest.fn() }));
jest.mock('@/features/notifications/scheduler', () => ({ clearReminders: jest.fn() }));
jest.mock('@/features/notifications/device', () => ({ deviceId: jest.fn() }));

const request = apiRequest as jest.Mock;
const count = queuedOperationCount as jest.Mock;
const sync = syncQueuedOperations as jest.Mock;
const identifier = deviceId as jest.Mock;

beforeEach(() => {
  jest.clearAllMocks();
  sync.mockResolvedValue({ applied: 0, retrying: 0, failed: [] });
  count.mockResolvedValue(0);
  identifier.mockResolvedValue('device/one');
  request.mockResolvedValue(undefined);
});

it('drains care, deactivates the device, then clears local state', async () => {
  await prepareForSignOut();

  expect(sync).toHaveBeenCalledTimes(1);
  expect(request).toHaveBeenCalledWith('/notifications/devices/device%2Fone', {
    method: 'DELETE',
  });
  expect(clearReminders).toHaveBeenCalledTimes(1);
  expect(clearOfflineData).toHaveBeenCalledTimes(1);
});

it('does not erase anything while a care mutation remains', async () => {
  count.mockResolvedValue(1);

  await expect(prepareForSignOut()).rejects.toBeInstanceOf(SignOutPreparationError);
  expect(request).not.toHaveBeenCalled();
  expect(clearReminders).not.toHaveBeenCalled();
  expect(clearOfflineData).not.toHaveBeenCalled();
});

it('does not claim sign-out when the device cannot be deactivated', async () => {
  request.mockRejectedValue(new Error('offline'));

  await expect(prepareForSignOut()).rejects.toThrow('disconnected securely');
  expect(clearReminders).not.toHaveBeenCalled();
  expect(clearOfflineData).not.toHaveBeenCalled();
});

// A device the server has no record of is already disconnected, which is what
// this call is for. Refusing to sign out then would trap somebody in the app
// over a registration that failed at sign-in and was never retried.
it('signs out when the server has never heard of this device', async () => {
  request.mockRejectedValue(
    new ApiError(404, 'NOT_FOUND', 'The requested resource does not exist.'),
  );

  await expect(prepareForSignOut()).resolves.toBeUndefined();
  expect(clearReminders).toHaveBeenCalledTimes(1);
  expect(clearOfflineData).toHaveBeenCalledTimes(1);
});

// Any other refusal still means the device may still be reachable, so the
// account must not be released on this phone.
it('still refuses when deactivation fails for any other reason', async () => {
  request.mockRejectedValue(new ApiError(500, 'INTERNAL', 'Something went wrong.'));

  await expect(prepareForSignOut()).rejects.toThrow('disconnected securely');
  expect(clearReminders).not.toHaveBeenCalled();
  expect(clearOfflineData).not.toHaveBeenCalled();
});
