import { queryClient } from './queryClient'
import { queryKeys } from './keys'
import { taskApi } from '../api/task'
import { adminTaskApi } from '../api/adminTask'
import { monitorApi } from '../api/monitor'
import { sanitizeApiErrorMessage } from '../utils/errorMessages'

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
