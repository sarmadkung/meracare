import type {
  CareTask,
  CareTaskTemplate,
  CreateTaskRequest,
  CreateTaskResponse,
  TaskListResponse,
  TaskScope,
  TaskStatus,
  TaskTemplateListResponse,
  UpdateTaskRequest,
  UpdateTaskTemplateRequest,
} from '@genxcare/contracts';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { apiRequest } from '@/lib/api-client';
import { ApiError } from '@/lib/api-error';
import { cacheTasks, cachedTasks, sqliteSyncStore } from '@/lib/offline/database';
import { newOperation, newOperationId } from '@/lib/offline/sync-queue';

/**
 * Care task data.
 *
 * All of it is server state and lives in TanStack Query. Zustand holds only the
 * transient choices a screen makes — never a task list
 * (docs/06-mobile-architecture.md, plans/phase4.md §§24–25).
 */
export const taskKeys = {
  all: ['tasks'] as const,
  forSenior: (seniorId: string, scope: TaskScope) => ['seniors', seniorId, 'tasks', scope] as const,
  templates: (seniorId: string) => ['seniors', seniorId, 'taskTemplates'] as const,
  detail: (taskId: string) => ['tasks', taskId] as const,
  assigned: ['tasks', 'assigned'] as const,
};

/**
 * Today's tasks stay fresh for a minute.
 *
 * Long enough that moving between screens does not refetch constantly, short
 * enough that a completion by another member appears quickly.
 */
const TASK_STALE_TIME = 60_000;

/**
 * A senior's tasks for one view.
 *
 * The answer is cached to SQLite as it arrives, and read back from there when
 * the request fails offline — so the list is legible on a phone with no signal
 * (plans/phase4.md §26).
 */
export function useSeniorTasks(seniorId: string | null, scope: TaskScope = 'today') {
  return useQuery({
    queryKey: taskKeys.forSenior(seniorId ?? '', scope),
    queryFn: async () => {
      try {
        const response = await apiRequest<TaskListResponse>(
          `/seniors/${seniorId}/tasks?scope=${scope}`,
        );
        // Only today's list is worth keeping offline: it is the one somebody
        // needs while standing in front of the person they care for.
        if (scope === 'today' && seniorId !== null) {
          await cacheTasks(seniorId, response.items);
        }
        return response.items;
      } catch (error) {
        if (error instanceof ApiError && error.isOffline && scope === 'today' && seniorId) {
          return cachedTasks(seniorId);
        }
        throw error;
      }
    },
    enabled: seniorId !== null,
    staleTime: TASK_STALE_TIME,
  });
}

/** The caller's own assigned tasks, across every circle. */
export function useMyTasks() {
  return useQuery({
    queryKey: taskKeys.assigned,
    queryFn: async () => (await apiRequest<TaskListResponse>('/tasks')).items,
    staleTime: TASK_STALE_TIME,
  });
}

/** One task. */
export function useTask(taskId: string | null) {
  return useQuery({
    queryKey: taskKeys.detail(taskId ?? ''),
    queryFn: () => apiRequest<CareTask>(`/tasks/${taskId}`),
    enabled: taskId !== null,
  });
}

/** A senior's recurring routines. */
export function useTaskTemplates(seniorId: string | null) {
  return useQuery({
    queryKey: taskKeys.templates(seniorId ?? ''),
    queryFn: async () =>
      (await apiRequest<TaskTemplateListResponse>(`/seniors/${seniorId}/tasks/templates`)).items,
    enabled: seniorId !== null,
  });
}

/** Creates a one-time or recurring task. */
export function useCreateTask(seniorId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (body: CreateTaskRequest) =>
      apiRequest<CreateTaskResponse>(`/seniors/${seniorId}/tasks`, { method: 'POST', body }),
    onSuccess: () => invalidateTasks(queryClient, seniorId),
  });
}

/** Edits one occurrence, leaving the rest of the series alone. */
export function useUpdateTask(seniorId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ taskId, ...body }: UpdateTaskRequest & { taskId: string }) =>
      apiRequest<CareTask>(`/tasks/${taskId}`, { method: 'PATCH', body }),
    onSuccess: (task) => {
      queryClient.setQueryData(taskKeys.detail(task.id), task);
      void invalidateTasks(queryClient, seniorId);
    },
  });
}

/** Edits a recurring routine. Future occurrences are rebuilt by the server. */
export function useUpdateTaskTemplate(seniorId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ templateId, ...body }: UpdateTaskTemplateRequest & { templateId: string }) =>
      apiRequest<CareTaskTemplate>(`/seniors/${seniorId}/tasks/templates/${templateId}`, {
        method: 'PATCH',
        body,
      }),
    onSuccess: () => invalidateTasks(queryClient, seniorId),
  });
}

/** Withdraws one occurrence. The record is kept, marked cancelled. */
export function useCancelTask(seniorId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (taskId: string) => apiRequest<CareTask>(`/tasks/${taskId}`, { method: 'DELETE' }),
    onSuccess: () => invalidateTasks(queryClient, seniorId),
  });
}

/** Marks a task done. */
export function useCompleteTask(seniorId: string) {
  return useTaskAction(seniorId, 'complete');
}

/** Marks a task deliberately not done. */
export function useSkipTask(seniorId: string) {
  return useTaskAction(seniorId, 'skip');
}

export interface TaskActionInput {
  taskId: string;
  notes?: string;
}

/**
 * Completing or skipping a task.
 *
 * Completion is the app's most frequent action, so the list updates the moment
 * it is tapped and the request follows (plans/phase4.md §23). Two things make
 * that safe:
 *
 *   - if the server refuses, `onError` puts back the exact snapshot taken
 *     before the change, so the screen never keeps showing a completion that
 *     did not happen;
 *   - if the request never leaves the phone, the mutation is queued and the
 *     optimistic state is kept deliberately, because the action *will* be sent.
 */
function useTaskAction(seniorId: string, operationType: 'complete' | 'skip') {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ taskId, notes }: TaskActionInput) => {
      const operationId = newOperationId();

      try {
        return await apiRequest<CareTask>(`/tasks/${taskId}/${operationType}`, {
          method: 'POST',
          body: notes === undefined ? undefined : { notes },
        });
      } catch (error) {
        if (error instanceof ApiError && error.isOffline) {
          await sqliteSyncStore.enqueue(
            newOperation(
              operationId,
              'task',
              taskId,
              operationType,
              notes === undefined ? null : { notes },
              new Date().toISOString(),
            ),
          );
          // Resolve rather than throw: the action is recorded and will be sent.
          // Throwing here would roll the optimistic update back and tell the
          // user their work was lost, which is the opposite of what happened.
          return null;
        }
        throw error;
      }
    },

    onMutate: async ({ taskId }) => {
      // Stop any in-flight refetch from landing on top of the optimistic
      // change and undoing it.
      await queryClient.cancelQueries({ queryKey: taskKeys.all });

      const snapshot = queryClient.getQueriesData<CareTask[]>({ queryKey: ['seniors'] });
      const assigned = queryClient.getQueryData<CareTask[]>(taskKeys.assigned);
      const detail = queryClient.getQueryData<CareTask>(taskKeys.detail(taskId));

      const status: TaskStatus = operationType === 'complete' ? 'completed' : 'skipped';
      const apply = (tasks: CareTask[] | undefined) =>
        tasks?.map((task) => (task.id === taskId ? { ...task, status } : task));

      queryClient.setQueriesData<CareTask[]>({ queryKey: ['seniors'] }, apply);
      queryClient.setQueryData<CareTask[]>(taskKeys.assigned, apply);
      if (detail) {
        queryClient.setQueryData<CareTask>(taskKeys.detail(taskId), { ...detail, status });
      }

      return { snapshot, assigned, detail };
    },

    onError: (_error, { taskId }, context) => {
      if (!context) return;

      for (const [key, value] of context.snapshot) {
        queryClient.setQueryData(key, value);
      }
      queryClient.setQueryData(taskKeys.assigned, context.assigned);
      queryClient.setQueryData(taskKeys.detail(taskId), context.detail);
    },

    onSuccess: (task, { taskId }) => {
      // A queued action returns null; there is nothing authoritative to apply
      // yet, and the optimistic state stands until the queue drains.
      if (task !== null) {
        queryClient.setQueryData(taskKeys.detail(taskId), task);
      }
    },

    onSettled: () => invalidateTasks(queryClient, seniorId),
  });
}

/**
 * Refreshes every task view a change could affect.
 *
 * Broad on purpose: completing a task changes today's list, the overdue list,
 * and the caller's assigned list, and working out precisely which is a
 * correctness risk for no meaningful saving.
 */
async function invalidateTasks(
  queryClient: ReturnType<typeof useQueryClient>,
  seniorId: string,
): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ['seniors', seniorId, 'tasks'] }),
    queryClient.invalidateQueries({ queryKey: taskKeys.assigned }),
    // Today reads its own endpoint and none of the keys above reach it.
    queryClient.invalidateQueries({ queryKey: ['today'] }),
  ]);
}
