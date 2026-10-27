import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { client } from '../api/client';
import type { components } from '../api/schema';
import { ensureOk, unwrap } from './apiError';

export type Task = components['schemas']['Task'];

export const POLL_INTERVAL_MS = 1000;

/** Cache key of one user's task list; the user id keeps accounts apart. */
export const tasksKey = (userId: string) => ['tasks', userId] as const;

/** Task list of the signed-in user, polled every second. */
export function useTasks(userId: string) {
  return useQuery({
    queryKey: tasksKey(userId),
    queryFn: async ({ signal }) => unwrap(await client.GET('/api/tasks', { signal })).items,
    refetchInterval: POLL_INTERVAL_MS,
    retry: false,
  });
}

function useInvalidateTasks() {
  const queryClient = useQueryClient();
  return () => queryClient.invalidateQueries({ queryKey: ['tasks'] });
}

export function useCreateTask() {
  const invalidate = useInvalidateTasks();
  return useMutation({
    mutationFn: async (expression: string) =>
      unwrap(await client.POST('/api/tasks', { body: { expression } })),
    onSettled: invalidate,
  });
}

export function useUpdateTask() {
  const invalidate = useInvalidateTasks();
  return useMutation({
    mutationFn: async ({ id, expression }: { id: string; expression: string }) =>
      unwrap(await client.PATCH('/api/tasks/{id}', { params: { path: { id } }, body: { expression } })),
    onSettled: invalidate,
  });
}

export function useDeleteTask() {
  const invalidate = useInvalidateTasks();
  return useMutation({
    mutationFn: async (id: string) =>
      ensureOk(await client.DELETE('/api/tasks/{id}', { params: { path: { id } } })),
    onSettled: invalidate,
  });
}
