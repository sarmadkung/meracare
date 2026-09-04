import type {
  CreateSeniorRequest,
  Senior,
  SeniorListResponse,
  UpdateSeniorRequest,
} from '@genxcare/contracts';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { apiRequest } from '@/lib/api-client';

/** Query keys for senior data. */
export const seniorKeys = {
  all: ['seniors'] as const,
  detail: (seniorId: string) => ['seniors', seniorId] as const,
};

/** Lists the seniors the signed-in user can reach. */
export function useSeniors(enabled = true) {
  return useQuery({
    queryKey: seniorKeys.all,
    queryFn: async () => (await apiRequest<SeniorListResponse>('/seniors')).items,
    enabled,
  });
}

/** Loads one senior profile. */
export function useSenior(seniorId: string | null) {
  return useQuery({
    queryKey: seniorKeys.detail(seniorId ?? ''),
    queryFn: () => apiRequest<Senior>(`/seniors/${seniorId}`),
    enabled: seniorId !== null,
  });
}

/** Creates a senior profile — the entry point for all four care modes. */
export function useCreateSenior() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (body: CreateSeniorRequest) =>
      apiRequest<Senior>('/seniors', { method: 'POST', body }),
    onSuccess: (created) => {
      queryClient.setQueryData(seniorKeys.detail(created.id), created);
      void queryClient.invalidateQueries({ queryKey: seniorKeys.all });
    },
  });
}

/** Updates a senior profile. */
export function useUpdateSenior(seniorId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (body: UpdateSeniorRequest) =>
      apiRequest<Senior>(`/seniors/${seniorId}`, { method: 'PATCH', body }),
    onSuccess: (updated) => {
      queryClient.setQueryData(seniorKeys.detail(seniorId), updated);
      void queryClient.invalidateQueries({ queryKey: seniorKeys.all });
    },
  });
}
