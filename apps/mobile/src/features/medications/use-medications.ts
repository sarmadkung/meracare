import type {
  AddMedicationDoseRequest,
  CreateMedicationRequest,
  CreateMedicationResponse,
  MedicationDetail,
  MedicationDose,
  MedicationDoseListResponse,
  MedicationDoseScope,
  MedicationHistoryResponse,
  MedicationListResponse,
  MedicationSchedule,
  MedicationScheduleInput,
  MedicationScheduleListResponse,
  MedicationStatus,
  UpdateMedicationRequest,
  UpdateMedicationScheduleRequest,
} from '@meracare/contracts';
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query';

import { apiRequest } from '@/lib/api-client';
import { ApiError } from '@/lib/api-error';
import { cacheDoses, cachedDoses, sqliteSyncStore } from '@/lib/offline/database';
import { newOperation, newOperationId } from '@/lib/offline/sync-queue';

import { doseEntityId } from './medication-sync';
import { cancelSnoozedMedicationNotifications } from '@/features/notifications/scheduler';

/**
 * Medication data.
 *
 * All of it is server state and lives in TanStack Query. Zustand holds only the
 * transient choices a screen makes — never a medication list
 * (docs/06-mobile-architecture.md, plans/phase5.md §§18–19).
 */
export const medicationKeys = {
  all: ['medications'] as const,
  forSenior: (seniorId: string) => ['seniors', seniorId, 'medications'] as const,
  doses: (seniorId: string, scope: MedicationDoseScope) =>
    ['seniors', seniorId, 'medicationDoses', scope] as const,
  detail: (medicationId: string) => ['medications', medicationId] as const,
  schedules: (medicationId: string) => ['medications', medicationId, 'schedules'] as const,
  history: (medicationId: string) => ['medications', medicationId, 'history'] as const,
};

/**
 * Medication stays fresh for a minute.
 *
 * Long enough that moving between screens does not refetch constantly, short
 * enough that a dose recorded by another member appears quickly.
 */
const MEDICATION_STALE_TIME = 60_000;

/** A senior's medications, active ones first. */
export function useMedications(seniorId: string | null) {
  return useQuery({
    queryKey: medicationKeys.forSenior(seniorId ?? ''),
    queryFn: async () =>
      (await apiRequest<MedicationListResponse>(`/seniors/${seniorId}/medications`)).items,
    enabled: seniorId !== null,
    staleTime: MEDICATION_STALE_TIME,
  });
}

/**
 * A senior's doses for one view.
 *
 * Today's answer is cached to SQLite as it arrives, and read back from there
 * when the request fails offline — so somebody can still see what is due while
 * standing in a house with no signal (plans/phase5.md §20).
 */
export function useMedicationDoses(seniorId: string | null, scope: MedicationDoseScope = 'today') {
  return useQuery({
    queryKey: medicationKeys.doses(seniorId ?? '', scope),
    queryFn: async () => {
      try {
        const response = await apiRequest<MedicationDoseListResponse>(
          `/seniors/${seniorId}/medications/doses?scope=${scope}`,
        );
        if (scope === 'today' && seniorId !== null) {
          await cacheDoses(seniorId, response.items);
        }
        return response.items;
      } catch (error) {
        if (error instanceof ApiError && error.isOffline && scope === 'today' && seniorId) {
          return cachedDoses(seniorId);
        }
        throw error;
      }
    },
    enabled: seniorId !== null,
    staleTime: MEDICATION_STALE_TIME,
  });
}

/** One medication, with its times and its next dose. */
export function useMedication(medicationId: string | null) {
  return useQuery({
    queryKey: medicationKeys.detail(medicationId ?? ''),
    queryFn: () => apiRequest<MedicationDetail>(`/medications/${medicationId}`),
    enabled: medicationId !== null,
  });
}

/** The times one medication is taken. */
export function useMedicationSchedules(medicationId: string | null) {
  return useQuery({
    queryKey: medicationKeys.schedules(medicationId ?? ''),
    queryFn: async () =>
      (await apiRequest<MedicationScheduleListResponse>(`/medications/${medicationId}/schedules`))
        .items,
    enabled: medicationId !== null,
  });
}

/**
 * A medication's history, a page at a time.
 *
 * Paged rather than fetched whole: a medicine somebody has taken twice a day
 * for two years is fifteen hundred doses, and loading them all to show the last
 * ten would be a slow screen and a large amount of memory
 * (plans/phase5.md §16).
 */
export function useMedicationHistory(medicationId: string | null) {
  return useInfiniteQuery({
    queryKey: medicationKeys.history(medicationId ?? ''),
    queryFn: ({ pageParam }) =>
      apiRequest<MedicationHistoryResponse>(
        `/medications/${medicationId}/instances${pageParam ? `?cursor=${encodeURIComponent(pageParam)}` : ''}`,
      ),
    initialPageParam: '',
    getNextPageParam: (lastPage) => lastPage.nextCursor ?? undefined,
    enabled: medicationId !== null,
  });
}

/** Adds a medication, with the times it is taken. */
export function useCreateMedication(seniorId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (body: CreateMedicationRequest) =>
      apiRequest<CreateMedicationResponse>(`/seniors/${seniorId}/medications`, {
        method: 'POST',
        body,
      }),
    onSuccess: () => invalidateMedications(queryClient, seniorId),
  });
}

/** Edits a medication. Stopping it is `active: false`, never a delete. */
export function useUpdateMedication(seniorId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ medicationId, ...body }: UpdateMedicationRequest & { medicationId: string }) =>
      apiRequest<MedicationDetail>(`/medications/${medicationId}`, { method: 'PATCH', body }),
    onSuccess: (medication) => {
      queryClient.setQueryData(medicationKeys.detail(medication.id), medication);
      void invalidateMedications(queryClient, seniorId);
    },
  });
}

/** Permanently removes a mistaken entry before any dose has been recorded. */
export function useDeleteMedication(seniorId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (medicationId: string) =>
      apiRequest<void>(`/medications/${medicationId}`, { method: 'DELETE' }),
    onSuccess: (_result, medicationId) => {
      queryClient.removeQueries({ queryKey: medicationKeys.detail(medicationId) });
      queryClient.removeQueries({ queryKey: medicationKeys.schedules(medicationId) });
      queryClient.removeQueries({ queryKey: medicationKeys.history(medicationId) });
      void invalidateMedications(queryClient, seniorId);
    },
  });
}

/** Adds a time of day to a medication. */
export function useAddMedicationSchedule(seniorId: string, medicationId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (body: MedicationScheduleInput) =>
      apiRequest<MedicationSchedule>(`/medications/${medicationId}/schedules`, {
        method: 'POST',
        body,
      }),
    onSuccess: () => invalidateOneMedication(queryClient, seniorId, medicationId),
  });
}

/** Edits or stops one time of day, leaving the others alone. */
export function useUpdateMedicationSchedule(seniorId: string, medicationId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      scheduleId,
      ...body
    }: UpdateMedicationScheduleRequest & { scheduleId: string }) =>
      apiRequest<MedicationSchedule>(`/medications/${medicationId}/schedules/${scheduleId}`, {
        method: 'PATCH',
        body,
      }),
    onSuccess: () => invalidateOneMedication(queryClient, seniorId, medicationId),
  });
}

/** Records a single extra dose, outside any repeating time. */
export function useAddMedicationDose(seniorId: string, medicationId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (body: AddMedicationDoseRequest) =>
      apiRequest<MedicationDose>(`/medications/${medicationId}/doses`, { method: 'POST', body }),
    onSuccess: () => invalidateOneMedication(queryClient, seniorId, medicationId),
  });
}

/** Marks a dose taken. */
export function useTakeDose(seniorId: string) {
  return useDoseAction(seniorId, 'take');
}

/** Marks a dose deliberately not taken. */
export function useSkipDose(seniorId: string) {
  return useDoseAction(seniorId, 'skip');
}

export interface DoseActionInput {
  medicationId: string;
  doseId: string;
  notes?: string;
}

/**
 * Taking or skipping a dose.
 *
 * "Mark as taken" is the app's most frequent medication action, so the list
 * updates the moment it is tapped and the request follows
 * (plans/phase5.md §17). Two things make that safe:
 *
 *   - if the server refuses, `onError` puts back the exact snapshot taken
 *     before the change, so the screen never keeps showing a dose as taken when
 *     it was not;
 *   - if the request never leaves the phone, the mutation is queued and the
 *     optimistic state is kept deliberately, because the action *will* be sent.
 */
function useDoseAction(seniorId: string, operationType: 'take' | 'skip') {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ medicationId, doseId, notes }: DoseActionInput) => {
      const operationId = newOperationId();

      try {
        return await apiRequest<MedicationDose>(
          `/medications/${medicationId}/instances/${doseId}/${operationType}`,
          {
            method: 'POST',
            body: notes === undefined ? undefined : { notes },
          },
        );
      } catch (error) {
        if (error instanceof ApiError && error.isOffline) {
          await sqliteSyncStore.enqueue(
            newOperation(
              operationId,
              'medication',
              doseEntityId(medicationId, doseId),
              operationType,
              notes === undefined ? null : { notes },
              new Date().toISOString(),
            ),
          );
          // Resolve rather than throw: the dose is recorded and will be sent.
          // Throwing here would roll the optimistic update back and tell the
          // user their record was lost, which is the opposite of what happened.
          return null;
        }
        throw error;
      }
    },

    onMutate: async ({ doseId }) => {
      // Stop any in-flight refetch from landing on top of the optimistic
      // change and undoing it.
      await queryClient.cancelQueries({ queryKey: medicationKeys.all });

      const snapshot = queryClient.getQueriesData<MedicationDose[]>({ queryKey: ['seniors'] });

      const status: MedicationStatus = operationType === 'take' ? 'taken' : 'skipped';
      const apply = (doses: MedicationDose[] | undefined) =>
        doses?.map((dose) => (dose.id === doseId ? { ...dose, status } : dose));

      queryClient.setQueriesData<MedicationDose[]>({ queryKey: ['seniors'] }, apply);

      return { snapshot };
    },

    onError: (_error, _input, context) => {
      if (!context) return;

      for (const [key, value] of context.snapshot) {
        queryClient.setQueryData(key, value);
      }
    },

    onSettled: async (_result, error, { medicationId, doseId }) => {
      if (!error) await cancelSnoozedMedicationNotifications(doseId).catch(() => {});
      await invalidateOneMedication(queryClient, seniorId, medicationId);
    },
  });
}

/**
 * Refreshes every medication view a change could affect.
 *
 * Broad on purpose: recording a dose changes today's list, the missed list, and
 * the medicine's own history, and working out precisely which is a correctness
 * risk for no meaningful saving.
 */
async function invalidateMedications(queryClient: QueryClient, seniorId: string): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: medicationKeys.forSenior(seniorId) }),
    queryClient.invalidateQueries({ queryKey: ['seniors', seniorId, 'medicationDoses'] }),
    queryClient.invalidateQueries({ queryKey: ['today'] }),
  ]);
}

async function invalidateOneMedication(
  queryClient: QueryClient,
  seniorId: string,
  medicationId: string,
): Promise<void> {
  await Promise.all([
    invalidateMedications(queryClient, seniorId),
    queryClient.invalidateQueries({ queryKey: medicationKeys.detail(medicationId) }),
  ]);
}
