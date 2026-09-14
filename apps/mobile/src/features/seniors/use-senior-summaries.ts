import type { SeniorSummaryListResponse } from '@genxcare/contracts';
import { useQuery } from '@tanstack/react-query';

import { apiRequest } from '@/lib/api-client';

/** Query key for the circle-wide summary. */
export const seniorSummaryKeys = {
  all: ['seniors', 'summary'] as const,
};

/**
 * Loads today's counts for every senior the caller can reach.
 *
 * A separate query from the senior list on purpose: names arrive immediately
 * and the cards fill in, rather than the whole circle waiting on the slower
 * request. The counts are also the part worth refetching when the reader comes
 * back to the screen, and the names are not.
 */
export function useSeniorSummaries(enabled = true) {
  return useQuery({
    queryKey: seniorSummaryKeys.all,
    queryFn: async () => (await apiRequest<SeniorSummaryListResponse>('/seniors/summary')).items,
    enabled,
    // Care moves during the day, and this is the number somebody glances at to
    // decide whether to act.
    staleTime: 60_000,
  });
}
