import type { Task } from '../../../../shared/api/task'
import { ListAdminTasksStatus } from '../../../../shared/api/generated-query/model'
import { calcDurationSeconds, formatAdminDateTime } from '../../../../shared/utils/dateTime'
import { sanitizeTaskErrorMessage } from '../../../../shared/utils/errorMessages'
import { getSafeTaskResultImageUrl } from './taskResultImages'

type TaskDurationTone = 'success' | 'warning' | 'danger'
type TaskApiCallInfo = {
  platform: string
}

type MonitorTaskRow = Task & {
  username?: string
  email?: string
  user?: {
    username?: string
    email?: string
  }
}

const taskApiCallCache = new WeakMap<Task, { result: string; value: TaskApiCallInfo | null }>()

export function formatTaskCreatedAt(row: Task) {
  return formatAdminDateTime(row.created_at, row.created_at)
}

export function getTaskResultImageUrl(row: Task) {
  if (row.status !== ListAdminTasksStatus.completed || !row.result) return ''
  return getSafeTaskResultImageUrl(row.result)
}

export function getTaskUserIdentifier(row: Task) {
  const taskRow = row as MonitorTaskRow
  return (
    taskRow.username ||
    taskRow.email ||
    taskRow.user?.username ||
    taskRow.user?.email ||
    taskRow.user_id
  )
}

export function getTaskApiCall(row: Task) {
  if (!row.result) return null
  const cached = taskApiCallCache.get(row)
  if (cached?.result === row.result) return cached.value

  let value: TaskApiCallInfo | null = null
  try {
    const result = JSON.parse(row.result) as { platform?: unknown }
    if (result.platform) {
      value = {
        platform: String(result.platform),
      }
    }
  } catch {
    value = null
  }
  taskApiCallCache.set(row, { result: row.result, value })
  return value
}

export function getTaskDuration(row: Task) {
  if (!row.completed_at || !row.created_at) return null
  const seconds = calcDurationSeconds(row.created_at, row.completed_at)
  if (seconds == null) return null
  const tone: TaskDurationTone = seconds > 60 ? 'danger' : seconds > 30 ? 'warning' : 'success'
  return { seconds, tone }
}

export function getTaskSanitizedError(row: Task) {
  if (row.status !== ListAdminTasksStatus.failed || !row.error_msg) return ''
  return sanitizeTaskErrorMessage(row.error_msg)
}
