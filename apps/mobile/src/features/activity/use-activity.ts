import type { ActivityResponse } from '@genxcare/contracts';
import { useInfiniteQuery } from '@tanstack/react-query';

import { apiRequest } from '@/lib/api-client';

/**
 * The activity timeline.
 *
 * Server state, so it lives in TanStack Query and never in Zustand
 * (plans/phase7.md §§18, 22).
 *
 * There is no mutation here and there never will be: events are written by the
 * server as a side effect of domain actions, and a client that could create one
 * could manufacture a history of care that was never given
 * (plans/phase7.md §21).
 */
export const activityKeys = {
  forSenior: (seniorId: string) => ['seniors', seniorId, 'activity'] as const,
};

/**
 * A senior's activity, a page at a time.
 *
 * Paged because a timeline grows without limit: a circle a year old has
 * thousands of entries, and loading them to show the last twenty would be a
 * slow screen and a large amount of memory (plans/phase7.md §30).
 */
export function useActivity(seniorId: string | null) {
  return useInfiniteQuery({
    queryKey: activityKeys.forSenior(seniorId ?? ''),
    queryFn: ({ pageParam }) =>
      apiRequest<ActivityResponse>(
        `/seniors/${seniorId}/activity${
          pageParam ? `?cursor=${encodeURIComponent(pageParam)}` : ''
        }`,
      ),
    initialPageParam: '',
    getNextPageParam: (lastPage) => lastPage.nextCursor ?? undefined,
    enabled: seniorId !== null,
    // Activity is a record of what has already happened, so it does not go
    // stale the way today's task list does. A minute keeps a screen returned to
    // from feeling frozen without refetching on every navigation.
    staleTime: 60_000,
  });
}
