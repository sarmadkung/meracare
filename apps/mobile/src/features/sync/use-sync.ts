import { useQueryClient } from '@tanstack/react-query';
import { useCallback, useEffect, useRef } from 'react';
import { AppState, type AppStateStatus } from 'react-native';

import { syncQueuedOperations } from './replay';

/**
 * Drains the offline queue when the app is usable again.
 *
 * Two triggers, both events rather than timers: the app coming to the
 * foreground, and mounting. plans/phase4.md §31 and plans/phase5.md §27 both
 * rule out polling and interval timers, and they would be the wrong tool anyway
 * — a queue that is empty ninety-nine times out of a hundred does not want
 * waking every few seconds.
 */
export function useOfflineSync() {
  const queryClient = useQueryClient();
  // Guards against two passes overlapping, which would replay the same
  // operation twice while the first is still in flight.
  const running = useRef(false);

  const drain = useCallback(async () => {
    if (running.current) return;
    running.current = true;

    try {
      const report = await syncQueuedOperations();

      // Only disturb the cache when something actually reached the server.
      if (report.applied > 0 || report.failed.length > 0) {
        await queryClient.invalidateQueries({ queryKey: ['tasks'] });
        await queryClient.invalidateQueries({ queryKey: ['medications'] });
        await queryClient.invalidateQueries({ queryKey: ['seniors'] });
        // Today is built from its own endpoint, so it is not reached by any of
        // the keys above — a drained dose would go on reading as pending.
        await queryClient.invalidateQueries({ queryKey: ['today'] });
      }
    } catch {
      // A failed pass is not an error the user can act on; the operations are
      // still queued and the next trigger will try again.
    } finally {
      running.current = false;
    }
  }, [queryClient]);

  useEffect(() => {
    void drain();

    const subscription = AppState.addEventListener('change', (state: AppStateStatus) => {
      if (state === 'active') void drain();
    });

    return () => subscription.remove();
  }, [drain]);
}
