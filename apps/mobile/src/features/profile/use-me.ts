import type { Me, UpdateMeRequest } from '@genxcare/contracts';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { apiRequest } from '@/lib/api-client';

/** Query keys for the authenticated user's profile. */
export const meKeys = {
  all: ['me'] as const,
};

/**
 * Loads the authenticated user's profile from `GET /v1/me`.
 *
 * The first call also creates the application user record server-side, so this
 * is the request that completes sign-up.
 */
export function useMe(enabled = true) {
  return useQuery({
    queryKey: meKeys.all,
    queryFn: () => apiRequest<Me>('/me'),
    enabled,
  });
}

/** Updates the authenticated user's profile via `PATCH /v1/me`. */
export function useUpdateMe() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (body: UpdateMeRequest) => apiRequest<Me>('/me', { method: 'PATCH', body }),
    onSuccess: (updated) => {
      queryClient.setQueryData(meKeys.all, updated);
    },
  });
}
