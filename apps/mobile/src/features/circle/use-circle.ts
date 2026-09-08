import type {
  AcceptInvitationResponse,
  CarePermission,
  CircleMember,
  CircleMemberListResponse,
  CreateInvitationRequest,
  CreateInvitationResponse,
  Invitation,
  InvitationListResponse,
  InvitationPreview,
} from '@genxcare/contracts';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { seniorKeys } from '@/features/seniors/use-seniors';
import { apiRequest } from '@/lib/api-client';

/**
 * Care circle and invitation data.
 *
 * All of it is server state and lives in TanStack Query; none of it is copied
 * into Zustand (docs/06-mobile-architecture.md).
 */
export const circleKeys = {
  members: (seniorId: string) => ['seniors', seniorId, 'members'] as const,
  invitations: (seniorId: string) => ['seniors', seniorId, 'invitations'] as const,
  invitationPreview: (token: string) => ['invitations', token] as const,
};

/** The senior's active care circle. */
export function useCircleMembers(seniorId: string | null) {
  return useQuery({
    queryKey: circleKeys.members(seniorId ?? ''),
    queryFn: async () =>
      (await apiRequest<CircleMemberListResponse>(`/seniors/${seniorId}/members`)).items,
    enabled: seniorId !== null,
  });
}

/** Invitations issued for this senior, including spent and expired ones. */
export function useInvitations(seniorId: string | null, enabled = true) {
  return useQuery({
    queryKey: circleKeys.invitations(seniorId ?? ''),
    queryFn: async () =>
      (await apiRequest<InvitationListResponse>(`/seniors/${seniorId}/invitations`)).items,
    enabled: seniorId !== null && enabled,
  });
}

/** Invites somebody into the circle. */
export function useCreateInvitation(seniorId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (body: CreateInvitationRequest) =>
      apiRequest<CreateInvitationResponse>(`/seniors/${seniorId}/invitations`, {
        method: 'POST',
        body,
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: circleKeys.invitations(seniorId) });
    },
  });
}

/** Withdraws a pending invitation. */
export function useRevokeInvitation(seniorId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (invitationId: string) =>
      apiRequest<Invitation>(`/invitations/${invitationId}/revoke`, { method: 'POST' }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: circleKeys.invitations(seniorId) });
    },
  });
}

/** Changes what a member may do. */
export function useUpdateMember(seniorId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      relationshipId,
      permissions,
    }: {
      relationshipId: string;
      permissions: CarePermission[];
    }) =>
      apiRequest<CircleMember>(`/seniors/${seniorId}/members/${relationshipId}`, {
        method: 'PATCH',
        body: { permissions },
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: circleKeys.members(seniorId) });
    },
  });
}

/** Removes a member from the circle. */
export function useRevokeMember(seniorId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (relationshipId: string) =>
      apiRequest<CircleMember>(`/seniors/${seniorId}/members/${relationshipId}`, {
        method: 'DELETE',
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: circleKeys.members(seniorId) });
    },
  });
}

/** Removes the signed-in caregiver from this care circle. */
export function useLeaveCareCircle(seniorId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () =>
      apiRequest<CircleMember>(`/seniors/${seniorId}/members/me`, { method: 'DELETE' }),
    onSuccess: () => {
      queryClient.removeQueries({ queryKey: seniorKeys.detail(seniorId) });
      queryClient.removeQueries({ queryKey: ['seniors', seniorId] });
      void queryClient.invalidateQueries({ queryKey: seniorKeys.all });
    },
  });
}

/** Reads an invitation from its token, before accepting. */
export function useInvitationPreview(token: string | null) {
  return useQuery({
    queryKey: circleKeys.invitationPreview(token ?? ''),
    queryFn: () => apiRequest<InvitationPreview>(`/invitations/${token}`, { authenticated: false }),
    enabled: token !== null && token.length > 0,
    // A token is either valid or not; retrying a rejection helps nobody.
    retry: false,
  });
}

/** Accepts an invitation, which creates the care relationship. */
export function useAcceptInvitation() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (token: string) =>
      apiRequest<AcceptInvitationResponse>(`/invitations/${token}/accept`, { method: 'POST' }),
    onSuccess: () => {
      // The newly reachable senior must appear straight away.
      void queryClient.invalidateQueries({ queryKey: seniorKeys.all });
    },
  });
}
