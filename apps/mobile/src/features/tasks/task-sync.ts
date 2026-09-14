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
 * The server's terminal transition is semantically idempotent: replaying the
 * same action returns the original result and emits no second care event
 * (plans/phase4.md §27).
 */
export async function replayTaskOperation(operation: SyncOperation): Promise<ReplayOutcome> {
  const notes = readNotes(operation.payload);

  try {
    await apiRequest<CareTask>(`/tasks/${operation.entityId}/${operation.operationType}`, {
      method: 'POST',
      body: notes === null ? undefined : { notes },
    });
    return { kind: 'applied' };
  } catch (error) {
    return classify(error);
  }
}
