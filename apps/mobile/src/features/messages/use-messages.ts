import type {
  CareMessage,
  CreateMessageRequest,
  MarkMessagesReadRequest,
  MessagePage,
} from '@genxcare/contracts';
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query';

import { apiRequest } from '@/lib/api-client';

export const messageKeys = {
  forSenior: (seniorId: string) => ['seniors', seniorId, 'messages'] as const,
};

export function useMessages(seniorId: string | null) {
  return useInfiniteQuery({
    queryKey: messageKeys.forSenior(seniorId ?? ''),
    queryFn: ({ pageParam }) =>
      apiRequest<MessagePage>(
        `/seniors/${seniorId}/messages${
          pageParam ? `?cursor=${encodeURIComponent(pageParam)}` : ''
        }`,
      ),
    initialPageParam: '',
    getNextPageParam: (lastPage) => lastPage.nextCursor ?? undefined,
    enabled: seniorId !== null,
    refetchInterval: 30_000,
  });
}

export function useSendMessage(seniorId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: CreateMessageRequest) =>
      apiRequest<CareMessage>(`/seniors/${seniorId}/messages`, { method: 'POST', body }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: messageKeys.forSenior(seniorId) });
    },
  });
}

export function useMarkMessagesRead(seniorId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: MarkMessagesReadRequest) =>
      apiRequest<void>(`/seniors/${seniorId}/messages/read`, { method: 'POST', body }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: messageKeys.forSenior(seniorId) });
    },
  });
}
