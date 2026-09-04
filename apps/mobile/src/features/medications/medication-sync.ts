import type { MedicationDose } from '@genxcare/contracts';

import { apiRequest } from '@/lib/api-client';
import { classify, readNotes } from '@/lib/offline/classify';
import type { ReplayOutcome, SyncOperation } from '@/lib/offline/sync-queue';

/**
 * Sending a queued medication mutation back to the server.
 *
 * A dose is addressed through its medication, matching the route
 * docs/05-api-and-backend-spec.md defines, so the queue has to carry both IDs.
 * They travel together in the entity ID as `medicationId/doseId` rather than in
 * a second column: docs/07 fixes the queue's record shape, and one composite
 * key is a smaller change than a schema every other entity would then carry
 * (plans/phase5.md §20).
 */

const SEPARATOR = '/';

/** Builds the queue's entity ID for one dose. */
export function doseEntityId(medicationId: string, doseId: string): string {
  return `${medicationId}${SEPARATOR}${doseId}`;
}

/** Reads back the dose a queued operation refers to. */
export function readDoseEntityId(
  entityId: string,
): { medicationId: string; doseId: string } | null {
  const [medicationId, doseId, ...rest] = entityId.split(SEPARATOR);

  if (medicationId === undefined || doseId === undefined || rest.length > 0) {
    return null;
  }
  return { medicationId, doseId };
}

/**
 * Replays one queued dose.
 *
 * The operation id travels as the idempotency key, so a mutation that reached
 * the server before the connection dropped is recognised rather than recording
 * a second dose of the same medicine (plans/phase5.md §21).
 */
export async function replayMedicationOperation(operation: SyncOperation): Promise<ReplayOutcome> {
  const dose = readDoseEntityId(operation.entityId);
  if (dose === null) {
    // Nothing can be done with an entry nobody can address. Setting it aside
    // surfaces it rather than retrying it forever.
    return { kind: 'permanent', message: 'This entry could not be sent.' };
  }

  const notes = readNotes(operation.payload);

  try {
    await apiRequest<MedicationDose>(
      `/medications/${dose.medicationId}/instances/${dose.doseId}/${operation.operationType}`,
      {
        method: 'POST',
        body: notes === null ? undefined : { notes },
        idempotencyKey: operation.operationId,
      },
    );
    return { kind: 'applied' };
  } catch (error) {
    return classify(error);
  }
}
