import { queryClient } from './queryClient'
import { queryKeys } from './keys'
import { taskApi } from '../api/task'
import { adminTaskApi } from '../api/adminTask'
import { monitorApi } from '../api/monitor'
import { sanitizeApiErrorMessage } from '../utils/errorMessages'
import { extractImageUrlsFromTaskResult } from './taskResult'

export function invalidateTaskDetail(taskId: string) {
  return queryClient.invalidateQueries({ queryKey: queryKeys.tasks.detail(taskId) })
}

export function invalidateTaskLists() {
  return queryClient.invalidateQueries({ queryKey: queryKeys.tasks.all })
}

export function invalidateAdminMonitor() {
  return queryClient.invalidateQueries({ queryKey: queryKeys.admin.monitor() })
}

export async function fetchTask(taskId: string) {
  const res = await taskApi.get(taskId)
  if (res.code !== 0 || !res.data?.task) {
    throw new Error(sanitizeApiErrorMessage(res.msg, '获取任务失败'))
  }
  return res.data.task
}

export async function fetchUserTasks(params: { page?: number; page_size?: number }) {
  return taskApi.list(params)
}

export async function fetchAdminTasks(params: {
  page?: number; page_size?: number; status?: string; user_id?: number; keyword?: string
}) {
  return adminTaskApi.list(params)
}

export async function fetchAdminMonitor() {
  return monitorApi.get()
}

/**
 * Replace setTimeout polling with queryClient-based task completion wait.
 * Keeps the same return semantics as the original waitTaskResult:
 * - completed -> returns image urls
 * - failed -> throws sanitized error
 * - timeout -> throws timeout error
 */
export async function waitTaskResultWithQueryClient(
  taskId: string,
  options?: { intervalMs?: number; maxAttempts?: number },
): Promise<string[]> {
  const intervalMs = options?.intervalMs ?? 2000
  const maxAttempts = options?.maxAttempts ?? 180

  for (let attempt = 0; attempt < maxAttempts; attempt += 1) {
    try {
      const task = await queryClient.fetchQuery({
        queryKey: queryKeys.tasks.detail(taskId),
        queryFn: () => fetchTask(taskId),
        staleTime: 0,
      })
      if (task.status === 'completed') {
        return extractImageUrlsFromTaskResult(task.result)
      }
      if (task.status === 'failed') {
        const msg = typeof task.error_msg === 'string' && task.error_msg.trim()
          ? task.error_msg.trim()
          : '生成失败'
        throw new Error(sanitizeApiErrorMessage(msg, '生成失败'))
      }
    } catch (error) {
      if (error instanceof Error && error.message !== '生成失败') {
        throw error
      }
      throw error
    }
    await new Promise(resolve => setTimeout(resolve, intervalMs))
  }
  throw new Error('任务轮询超时，请稍后查看结果')
}
