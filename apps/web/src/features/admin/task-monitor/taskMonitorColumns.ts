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

export {
  getTaskApiCall,
  getTaskDuration,
  getTaskResultImageUrl,
  getTaskSanitizedError,
} from './taskResultPresentation'
