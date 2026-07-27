import type { Task } from '../../../shared/api/task'
import { formatAdminDateTime } from '../../../shared/utils/dateTime'

type MonitorTaskRow = Task & {
  username?: string
  email?: string
  user?: {
    username?: string
    email?: string
  }
}

export function formatTaskCreatedAt(row: Task) {
  return formatAdminDateTime(row.created_at, row.created_at)
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

export function getTaskEndpointLabel(row: Task) {
  return row.provider_endpoint_name || formatIDLabel('Endpoint', row.assigned_provider_endpoint_id)
}

export function getTaskCredentialLabel(row: Task) {
  if (row.credential_fingerprint) return row.credential_fingerprint
  return formatIDLabel('Credential', row.assigned_credential_id)
}

export function getTaskModelLabel(row: Task) {
  return row.resolved_model || ''
}

export function getTaskPriorityLabel(row: Task) {
  const priority = Number(row.priority || 2)
  return `P${Number.isFinite(priority) ? priority : 2}`
}

function formatIDLabel(prefix: string, id: number | null | undefined) {
  return id ? `${prefix} #${id}` : ''
}

export {
  getTaskApiCall,
  getTaskDuration,
  getTaskResultImageUrl,
  getTaskSanitizedError,
} from './taskResultPresentation'
