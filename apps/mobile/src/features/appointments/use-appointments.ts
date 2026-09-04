import type {
  Appointment,
  AppointmentListResponse,
  AppointmentScope,
  AppointmentStatus,
  CreateAppointmentRequest,
  UpdateAppointmentRequest,
} from '@genxcare/contracts';
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query';

import { apiRequest } from '@/lib/api-client';
import { ApiError } from '@/lib/api-error';
import { cacheAppointments, cachedAppointments } from '@/lib/offline/database';

/**
 * Appointment data.
 *
 * All of it is server state and lives in TanStack Query. Zustand holds only the
 * transient choices a screen makes — never an appointment list
 * (docs/06-mobile-architecture.md, plans/phase6.md §§21–22).
 */
export const appointmentKeys = {
  all: ['appointments'] as const,
  forSenior: (seniorId: string, scope: AppointmentScope) =>
    ['seniors', seniorId, 'appointments', scope] as const,
  history: (seniorId: string) => ['seniors', seniorId, 'appointments', 'history'] as const,
  detail: (appointmentId: string) => ['appointments', appointmentId] as const,
};

/**
 * Appointments stay fresh for a minute.
 *
 * Long enough that moving between screens does not refetch constantly, short
 * enough that a visit another member cancelled appears quickly. Deliberately
 * not polling: nothing here should be asking the server every few seconds
 * (plans/phase6.md §26).
 */
const APPOINTMENT_STALE_TIME = 60_000;

/**
 * A senior's appointments for one view.
 *
 * The upcoming answer is cached to SQLite as it arrives and read back when the
 * request fails offline — so somebody in a car on the way to a hospital can
 * still see where they are going. Appointments are read offline and never
 * written offline, which is why there is no queue here (plans/phase6.md §23).
 */
export function useAppointments(seniorId: string | null, scope: AppointmentScope = 'upcoming') {
  return useQuery({
    queryKey: appointmentKeys.forSenior(seniorId ?? '', scope),
    queryFn: async () => {
      try {
        const response = await apiRequest<AppointmentListResponse>(
          `/seniors/${seniorId}/appointments?scope=${scope}`,
        );
        if (scope === 'upcoming' && seniorId !== null) {
          await cacheAppointments(seniorId, response.items);
        }
        return response.items;
      } catch (error) {
        if (error instanceof ApiError && error.isOffline && scope === 'upcoming' && seniorId) {
          return cachedAppointments(seniorId);
        }
        throw error;
      }
    },
    enabled: seniorId !== null,
    staleTime: APPOINTMENT_STALE_TIME,
  });
}

/**
 * A senior's past appointments, a page at a time.
 *
 * Paged rather than fetched whole: a circle accumulates appointments for years,
 * and loading them all to show the last ten would be a slow screen and a large
 * amount of memory (plans/phase6.md §§6, 32).
 */
export function useAppointmentHistory(seniorId: string | null) {
  return useInfiniteQuery({
    queryKey: appointmentKeys.history(seniorId ?? ''),
    queryFn: ({ pageParam }) =>
      apiRequest<AppointmentListResponse>(
        `/seniors/${seniorId}/appointments?scope=past${
          pageParam ? `&cursor=${encodeURIComponent(pageParam)}` : ''
        }`,
      ),
    initialPageParam: '',
    getNextPageParam: (lastPage) => lastPage.nextCursor ?? undefined,
    enabled: seniorId !== null,
  });
}

/** One appointment in full. */
export function useAppointment(appointmentId: string | null) {
  return useQuery({
    queryKey: appointmentKeys.detail(appointmentId ?? ''),
    queryFn: () => apiRequest<Appointment>(`/appointments/${appointmentId}`),
    enabled: appointmentId !== null,
  });
}

/** Books an appointment. */
export function useCreateAppointment(seniorId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (body: CreateAppointmentRequest) =>
      apiRequest<Appointment>(`/seniors/${seniorId}/appointments`, { method: 'POST', body }),
    onSuccess: () => invalidateAppointments(queryClient, seniorId),
  });
}

/**
 * Edits an appointment.
 *
 * The server refuses an edit to one that has already been completed or
 * cancelled, and answers 409. The screen surfaces that rather than pretending
 * the change landed (plans/phase6.md §§8, 19).
 */
export function useUpdateAppointment(seniorId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      appointmentId,
      ...body
    }: UpdateAppointmentRequest & { appointmentId: string }) =>
      apiRequest<Appointment>(`/appointments/${appointmentId}`, { method: 'PATCH', body }),
    onSuccess: (appointment) => {
      queryClient.setQueryData(appointmentKeys.detail(appointment.id), appointment);
      void invalidateAppointments(queryClient, seniorId);
    },
  });
}

/** Marks an appointment as called off. Its record is kept. */
export function useCancelAppointment(seniorId: string) {
  return useAppointmentAction(seniorId, 'cancel', 'cancelled');
}

/** Records that an appointment happened. */
export function useCompleteAppointment(seniorId: string) {
  return useAppointmentAction(seniorId, 'complete', 'completed');
}

export interface AppointmentActionInput {
  appointmentId: string;
}

/**
 * Cancelling or completing an appointment.
 *
 * The list updates the moment the action is confirmed and the request follows,
 * and `onError` puts back the exact snapshot taken beforehand — so the screen
 * never keeps showing an appointment as cancelled when the server refused
 * (plans/phase6.md §20).
 *
 * There is deliberately no offline branch, unlike a medication dose. Nothing
 * queues an appointment mutation, so losing the connection is a genuine failure
 * and the rollback is the honest answer (plans/phase6.md §23).
 */
function useAppointmentAction(
  seniorId: string,
  action: 'cancel' | 'complete',
  resulting: AppointmentStatus,
) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ appointmentId }: AppointmentActionInput) =>
      apiRequest<Appointment>(`/appointments/${appointmentId}/${action}`, {
        method: 'POST',
        // The operation id is the idempotency key, so a request that reached
        // the server before the connection dropped is recognised on retry
        // rather than fighting with itself (plans/phase6.md §24).
        idempotencyKey: newOperationId(),
      }),

    onMutate: async ({ appointmentId }) => {
      // Stop any in-flight refetch from landing on top of the optimistic change
      // and undoing it.
      await queryClient.cancelQueries({ queryKey: ['seniors'] });

      const snapshot = queryClient.getQueriesData<Appointment[]>({ queryKey: ['seniors'] });

      queryClient.setQueriesData<Appointment[]>({ queryKey: ['seniors'] }, (appointments) =>
        appointments?.map((appointment) =>
          appointment.id === appointmentId ? { ...appointment, status: resulting } : appointment,
        ),
      );

      return { snapshot };
    },

    onError: (_error, _input, context) => {
      if (!context) return;

      for (const [key, value] of context.snapshot) {
        queryClient.setQueryData(key, value);
      }
    },

    onSettled: (_result, _error, { appointmentId }) =>
      Promise.all([
        invalidateAppointments(queryClient, seniorId),
        queryClient.invalidateQueries({ queryKey: appointmentKeys.detail(appointmentId) }),
      ]),
  });
}

/**
 * Refreshes every appointment view a change could affect.
 *
 * Broad on purpose: booking or cancelling changes today's list, the upcoming
 * list and the history, and working out precisely which is a correctness risk
 * for no meaningful saving.
 */
async function invalidateAppointments(queryClient: QueryClient, seniorId: string): Promise<void> {
  await queryClient.invalidateQueries({ queryKey: ['seniors', seniorId, 'appointments'] });
}

/** A unique id for one user action, used as the idempotency key. */
function newOperationId(): string {
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
}
