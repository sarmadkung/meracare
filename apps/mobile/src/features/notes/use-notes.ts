import type {
  CareNote,
  CareNoteListResponse,
  CreateCareNoteRequest,
  UpdateCareNoteRequest,
} from '@genxcare/contracts';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { apiRequest } from '@/lib/api-client';

export const noteKeys = {
  forSenior: (seniorId: string) => ['seniors', seniorId, 'notes'] as const,
};

export function useNotes(seniorId: string | null) {
  return useQuery({
    queryKey: noteKeys.forSenior(seniorId ?? ''),
    queryFn: async () =>
      (await apiRequest<CareNoteListResponse>(`/seniors/${seniorId}/notes`)).items,
    enabled: seniorId !== null,
  });
}

export function useCreateNote(seniorId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: CreateCareNoteRequest) =>
      apiRequest<CareNote>(`/seniors/${seniorId}/notes`, { method: 'POST', body }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: noteKeys.forSenior(seniorId) });
      void queryClient.invalidateQueries({ queryKey: ['seniors', seniorId, 'activity'] });
    },
  });
}

export function useUpdateNote(seniorId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ noteId, ...body }: UpdateCareNoteRequest & { noteId: string }) =>
      apiRequest<CareNote>(`/notes/${noteId}`, { method: 'PATCH', body }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: noteKeys.forSenior(seniorId) });
    },
  });
}
