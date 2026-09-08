import type { TodayListResponse } from '@meracare/contracts';
import { useQuery } from '@tanstack/react-query';

import { apiRequest } from '@/lib/api-client';

/** Query key for the cross-circle day. */
export const todayKeys = {
  all: ['today'] as const,
};

/**
 * Loads everything due today across every circle the reader belongs to.
 *
 * Distinct from `useMyTasks`, which asks `/v1/tasks` for work assigned to the
 * reader. That is the right list for a professional working a round and the
 * wrong one for a family member caring alone, who assigns nothing to
 * themselves and would see an empty screen forever.
 */
export function useToday(enabled = true) {
  return useQuery({
    queryKey: todayKeys.all,
    queryFn: async () => (await apiRequest<TodayListResponse>('/today')).items,
    enabled,
    // Care moves through the day, and this is the screen people return to.
    staleTime: 60_000,
  });
}
