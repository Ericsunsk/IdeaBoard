import { formatAdminDateTime } from '../../../shared/utils/dateTime'
import type { AdminTaskMonitorListItemModel } from './taskMonitorResource'

export function formatTaskCreatedAt(row: AdminTaskMonitorListItemModel) {
  return formatAdminDateTime(row.created_at, row.created_at)
}

export function getTaskUserIdentifier(row: AdminTaskMonitorListItemModel) {
  return row.user_id
}

export function getTaskAccountLabel(row: AdminTaskMonitorListItemModel) {
  return row.latest_execution ? String(row.latest_execution.account_id) : ''
}

export function getTaskEndpointLabel(row: AdminTaskMonitorListItemModel) {
  return row.latest_execution?.provider_endpoint_name || ''
}

export function getTaskModelLabel(row: AdminTaskMonitorListItemModel) {
  return row.latest_execution?.resolved_model || ''
}

export function getTaskAPITypeLabel(row: AdminTaskMonitorListItemModel) {
  return row.latest_execution?.api_type || ''
}

export function getTaskPriorityLabel(row: Pick<AdminTaskMonitorListItemModel, 'priority'>) {
  const priority = Number(row.priority || 2)
  return `P${Number.isFinite(priority) ? priority : 2}`
}

export {
  getTaskApiCall,
  getTaskDuration,
  getTaskResultImageUrl,
  getTaskSanitizedError,
} from './taskResultPresentation'
