import type { CareTask } from '@genxcare/contracts';

import { apiRequest } from '@/lib/api-client';
import { classify, readNotes } from '@/lib/offline/classify';
import type { ReplayOutcome, SyncOperation } from '@/lib/offline/sync-queue';

/**
 * Sending a queued task mutation back to the server.
 *
 * Kept apart from the queue mechanism so the decision that matters — which
 * failures are worth retrying and which are final — is testable on its own. That
 * decision now lives in lib/offline/classify, because a medication dose fails
 * in exactly the same ways.
 */

/**
 * Replays one queued completion or skip.
 *
 * The operation id travels as the idempotency key, so a mutation that reached
 * the server before the connection dropped is recognised rather than applied
 * twice (plans/phase4.md §27).
 */
export async function replayTaskOperation(operation: SyncOperation): Promise<ReplayOutcome> {
  const notes = readNotes(operation.payload);

  try {
    await apiRequest<CareTask>(`/tasks/${operation.entityId}/${operation.operationType}`, {
      method: 'POST',
      body: notes === null ? undefined : { notes },
      idempotencyKey: operation.operationId,
    });
    return { kind: 'applied' };
  } catch (error) {
    return classify(error);
  }
}
