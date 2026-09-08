import { newOperation, type SyncOperation } from '@/lib/offline/sync-queue';

import { doseEntityId, readDoseEntityId, replayMedicationOperation } from '../medication-sync';

/**
 * Sending a queued dose back to the server.
 *
 * A dose is addressed through its medication, so the queue has to carry both
 * IDs and get them back out again. An entry nobody can address is care that
 * cannot be recorded, which is why the unreadable case is pinned here.
 */

const mockApiRequest = jest.fn();

jest.mock('@/lib/api-client', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}));

function queued(overrides: Partial<SyncOperation> = {}): SyncOperation {
  return {
    ...newOperation(
      'op-1',
      'medication',
      doseEntityId('med-1', 'dose-1'),
      'take',
      null,
      '2026-08-17T09:00:00Z',
    ),
    ...overrides,
  };
}

beforeEach(() => {
  mockApiRequest.mockReset();
});

describe('doseEntityId', () => {
  it('round-trips the two ids a dose is addressed by', () => {
    expect(readDoseEntityId(doseEntityId('med-1', 'dose-1'))).toEqual({
      medicationId: 'med-1',
      doseId: 'dose-1',
    });
  });

  it('refuses an entity id it cannot read', () => {
    expect(readDoseEntityId('med-1')).toBeNull();
    expect(readDoseEntityId('med-1/dose-1/extra')).toBeNull();
  });
});

describe('replayMedicationOperation', () => {
  it('sends the dose to the route that records it', async () => {
    mockApiRequest.mockResolvedValue({});

    const outcome = await replayMedicationOperation(queued());

    expect(outcome).toEqual({ kind: 'applied' });
    const [path, options] = mockApiRequest.mock.calls[0] as [string, { method: string }];
    expect(path).toBe('/medications/med-1/instances/dose-1/take');
    expect(options.method).toBe('POST');
  });

  it('carries the note a skip was recorded with', async () => {
    mockApiRequest.mockResolvedValue({});

    await replayMedicationOperation(
      queued({
        operationType: 'skip',
        payload: JSON.stringify({ notes: 'She was asleep' }),
      }),
    );

    const [path, options] = mockApiRequest.mock.calls[0] as [string, { body: unknown }];
    expect(path).toBe('/medications/med-1/instances/dose-1/skip');
    expect(options.body).toEqual({ notes: 'She was asleep' });
  });

  it('replays a notification action through the direct dose route', async () => {
    mockApiRequest.mockResolvedValue({});

    await replayMedicationOperation(queued({ entityId: '2fb53d18-8ec8-4f21-9088-53a06f7647f6' }));

    expect(mockApiRequest.mock.calls[0]?.[0]).toBe(
      '/medications/instances/2fb53d18-8ec8-4f21-9088-53a06f7647f6/take',
    );
  });

  // Retrying it would never succeed, and would keep a stale entry alive
  // forever. Setting it aside surfaces it to somebody instead.
  it('sets aside an entry it cannot address', async () => {
    const outcome = await replayMedicationOperation(queued({ entityId: 'nonsense' }));

    expect(outcome.kind).toBe('permanent');
    expect(mockApiRequest).not.toHaveBeenCalled();
  });
});
