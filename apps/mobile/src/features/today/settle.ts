import type { TodayItem } from '@genxcare/contracts';
import type { QueryClient } from '@tanstack/react-query';

import { apiRequest } from '@/lib/api-client';
import { ApiError } from '@/lib/api-error';
import { sqliteSyncStore } from '@/lib/offline/database';
import { newOperation, newOperationId } from '@/lib/offline/sync-queue';

import type { AgendaAction } from './today-agenda';
import { todayKeys } from './use-today';

/**
 * Recording an outcome from the day itself.
 *
 * Today spans every circle, so it never loads a medication and cannot address a
 * dose the way the medication screens do. It uses the by-instance route built
 * for notification actions, which resolves the dose under the caller's own
 * authorization and carries no medical detail in the URL.
 *
 * The optimistic update lives here rather than in a mutation's `onMutate`
 * because the rollback and the queueing decision belong together: an offline
 * action is recorded, not lost, so its optimistic state is kept deliberately
 * while a refusal puts the row back.
 */

/** What the person said happened. */
export type Outcome = 'done' | 'skipped';

/** How each domain names the two outcomes. */
const verbs = {
  dose: { done: 'take', skipped: 'skip' },
  task: { done: 'complete', skipped: 'skip' },
} as const;

/** The status the row takes on immediately, before the server has answered. */
const optimisticStatus = {
  dose: { done: 'taken', skipped: 'skipped' },
  task: { done: 'completed', skipped: 'skipped' },
} as const;

/**
 * Records a dose or a task from Today.
 *
 * Resolves with `queued` when the phone had no signal — the action is durably
 * stored and will be sent, so it is not an error the reader can act on.
 * Rejects only when the server actually refused.
 */
export async function settleAgendaItem(
  queryClient: QueryClient,
  action: AgendaAction,
  outcome: Outcome,
): Promise<'recorded' | 'queued'> {
  const verb = verbs[action.kind][outcome];
  const snapshot = queryClient.getQueryData<TodayItem[]>(todayKeys.all);

  apply(queryClient, action, optimisticStatus[action.kind][outcome]);

  const path =
    action.kind === 'dose'
      ? `/medications/instances/${action.id}/${verb}`
      : `/tasks/${action.id}/${verb}`;

  try {
    await apiRequest(path, { method: 'POST' });
  } catch (error) {
    if (error instanceof ApiError && error.isOffline) {
      // The medication queue reads a bare dose id as a by-instance action, and
      // replays it through the same route (medication-sync.ts).
      await sqliteSyncStore.enqueue(
        newOperation(
          newOperationId(),
          action.kind === 'dose' ? 'medication' : 'task',
          action.id,
          verb,
          null,
          new Date().toISOString(),
        ),
      );
      await refresh(queryClient, action);
      return 'queued';
    }

    // The server said no. Showing the row as settled would be a lie about care
    // that was not recorded.
    queryClient.setQueryData(todayKeys.all, snapshot);
    throw error;
  }

  await refresh(queryClient, action);
  return 'recorded';
}

/** Writes a settled status onto the cached day, if the day is cached at all. */
function apply(queryClient: QueryClient, action: AgendaAction, status: string): void {
  queryClient.setQueryData<TodayItem[]>(todayKeys.all, (items) =>
    items?.map((item) =>
      item.kind === action.kind && item.id === action.id ? { ...item, status } : item,
    ),
  );
}

/**
 * Refreshes everything the outcome also changed.
 *
 * Broad on purpose, matching the medication and task hooks: recording a dose
 * changes today's list, the missed list and the medicine's own history, and
 * working out precisely which is a correctness risk for no meaningful saving.
 */
async function refresh(queryClient: QueryClient, action: AgendaAction): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: todayKeys.all }),
    queryClient.invalidateQueries({ queryKey: ['seniors', action.seniorId] }),
    queryClient.invalidateQueries({
      queryKey:
        action.kind === 'dose'
          ? ['seniors', action.seniorId, 'medicationDoses']
          : ['seniors', action.seniorId, 'tasks'],
    }),
  ]);
}
