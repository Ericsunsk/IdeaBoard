import type { Task } from '../../../../shared/api/task'
import { calcDurationSeconds, formatAdminDateTime } from '../../../../shared/utils/dateTime'
import { sanitizeTaskErrorMessage } from '../../../../shared/utils/errorMessages'
import { getSafeTaskResultImageUrl } from './taskResultImages'

type TaskStatusTagSeverity = 'secondary' | 'warn' | 'success' | 'danger'
type TaskDurationTone = 'success' | 'warning' | 'danger'

interface TaskStatusConfig {
  severity: TaskStatusTagSeverity
  text: string
  color: string
}

type MonitorTaskRow = Task & {
  username?: string
  email?: string
  user?: {
    username?: string
    email?: string
  }
}

export const ERROR_TEXT_COLOR = 'var(--color-admin-danger)'
export const DEFAULT_STATUS_COLOR = 'var(--color-admin-text-3)'

export const taskStatusOptions = [
  { label: '等待中', value: 'pending' },
  { label: '执行中', value: 'running' },
  { label: '已完成', value: 'completed' },
  { label: '失败', value: 'failed' },
]

const taskStatusConfigMap: Record<string, TaskStatusConfig> = {
  pending: { severity: 'secondary', text: '等待中', color: DEFAULT_STATUS_COLOR },
  running: { severity: 'warn', text: '执行中', color: 'var(--color-admin-warning)' },
  completed: { severity: 'success', text: '已完成', color: 'var(--color-admin-success)' },
  failed: { severity: 'danger', text: '失败', color: ERROR_TEXT_COLOR },
}

export function getTaskStatusConfig(status: string): TaskStatusConfig {
  return (
    taskStatusConfigMap[status] || {
      severity: 'secondary',
      text: status,
      color: DEFAULT_STATUS_COLOR,
    }
  )
}

export function formatTaskCreatedAt(row: Task) {
  return formatAdminDateTime(row.created_at, row.created_at)
}

export function getTaskResultImageUrl(row: Task) {
  if (row.status !== 'completed' || !row.result) return ''
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
  try {
    const result = JSON.parse(row.result) as { platform?: unknown; model_set?: unknown }
    if (!result.platform) return null
    return {
      platform: String(result.platform),
      modelSet: result.model_set ? String(result.model_set) : '',
    }
  } catch {
    return null
  }
}

export function getTaskDuration(row: Task) {
  if (!row.completed_at || !row.created_at) return null
  const seconds = calcDurationSeconds(row.created_at, row.completed_at)
  if (seconds == null) return null
  const tone: TaskDurationTone = seconds > 60 ? 'danger' : seconds > 30 ? 'warning' : 'success'
  return { seconds, tone }
}

export function getTaskSanitizedError(row: Task) {
  if (row.status !== 'failed' || !row.error_msg) return ''
  return sanitizeTaskErrorMessage(row.error_msg)
}
