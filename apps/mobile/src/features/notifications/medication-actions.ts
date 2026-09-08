import type { MedicationDose } from '@genxcare/contracts';
import type { QueryClient } from '@tanstack/react-query';

import { medicationKeys } from '@/features/medications/use-medications';
import { apiRequest } from '@/lib/api-client';
import { ApiError } from '@/lib/api-error';
import { sqliteSyncStore } from '@/lib/offline/database';
import { newOperation } from '@/lib/offline/sync-queue';

export type MedicationNotificationAction = 'take' | 'skip';

/**
 * Records an action launched from a notification.
 *
 * The notification carries only the dose id. The direct API route resolves it
 * under the caller's authorization, while an offline action is durably queued
 * using that same privacy-preserving identifier.
 */
export async function recordMedicationNotificationAction(
  queryClient: QueryClient,
  seniorId: string,
  doseId: string,
  action: MedicationNotificationAction,
): Promise<'recorded' | 'queued'> {
  try {
    await apiRequest<MedicationDose>(`/medications/instances/${doseId}/${action}`, {
      method: 'POST',
    });
  } catch (error) {
    if (!(error instanceof ApiError) || !error.isOffline) throw error;

    await sqliteSyncStore.enqueue(
      newOperation(
        notificationOperationId(doseId, action),
        'medication',
        doseId,
        action,
        null,
        new Date().toISOString(),
      ),
    );
    await invalidateMedicationViews(queryClient, seniorId);
    return 'queued';
  }

  await invalidateMedicationViews(queryClient, seniorId);
  return 'recorded';
}

async function invalidateMedicationViews(queryClient: QueryClient, seniorId: string) {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: medicationKeys.forSenior(seniorId) }),
    queryClient.invalidateQueries({ queryKey: ['seniors', seniorId, 'medicationDoses'] }),
  ]);
}

function notificationOperationId(doseId: string, action: MedicationNotificationAction): string {
  return `notification-${action}-${doseId}`;
}
