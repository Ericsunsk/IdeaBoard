import {
  ListAdminTasksStatus,
  type ListAdminTasksStatus as ListAdminTasksStatusValue,
} from '../../../../shared/domain/taskMetricTokens'

type TaskStatusTagSeverity = 'secondary' | 'warn' | 'success' | 'danger'

export interface TaskStatusDefinition {
  severity: TaskStatusTagSeverity
  text: string
  color: string
  bannerClass: string
  dotClass: string
}

const DEFAULT_STATUS_COLOR = 'var(--color-admin-text-3)'
const DEFAULT_BANNER_CLASS =
  'bg-[linear-gradient(135deg,rgba(152,162,179,0.12),rgba(255,255,255,0.74))] before:bg-admin-text-3'
const DEFAULT_DOT_CLASS = 'bg-admin-text-3 shadow-[0_0_0_4px_rgba(152,162,179,0.14)]'

// Pending 与 Events 表 metric 面板同为 info 系（taskMetricTokens），双源不再分叉。
const PENDING_STATUS_COLOR = 'var(--color-admin-info)'
const PENDING_BANNER_CLASS =
  'bg-[linear-gradient(135deg,rgba(71,117,168,0.1),rgba(255,255,255,0.74))] before:bg-admin-info'
const PENDING_DOT_CLASS = 'bg-admin-info shadow-[0_0_0_4px_rgba(71,117,168,0.15)]'

export const adminTaskStatuses = ListAdminTasksStatus

const taskStatusDefinitions: Record<ListAdminTasksStatusValue, TaskStatusDefinition> = {
  [ListAdminTasksStatus.pending]: {
    severity: 'secondary',
    text: '等待中',
    color: PENDING_STATUS_COLOR,
    bannerClass: PENDING_BANNER_CLASS,
    dotClass: PENDING_DOT_CLASS,
  },
  [ListAdminTasksStatus.running]: {
    severity: 'warn',
    text: '执行中',
    color: 'var(--color-admin-warning)',
    bannerClass:
      'bg-[linear-gradient(135deg,rgba(212,136,48,0.1),rgba(255,255,255,0.74))] before:bg-admin-warning',
    dotClass: 'bg-admin-warning shadow-[0_0_0_4px_rgba(212,136,48,0.15)]',
  },
  [ListAdminTasksStatus.completed]: {
    severity: 'success',
    text: '已完成',
    color: 'var(--color-admin-success)',
    bannerClass:
      'bg-[linear-gradient(135deg,rgba(47,158,115,0.1),rgba(255,255,255,0.74))] before:bg-admin-success',
    dotClass: 'bg-admin-success shadow-[0_0_0_4px_rgba(47,158,115,0.14)]',
  },
  [ListAdminTasksStatus.failed]: {
    severity: 'danger',
    text: '失败',
    color: 'var(--color-admin-danger)',
    bannerClass:
      'bg-[linear-gradient(135deg,rgba(209,77,90,0.095),rgba(255,255,255,0.74))] before:bg-admin-danger',
    dotClass: 'bg-admin-danger shadow-[0_0_0_4px_rgba(209,77,90,0.14)]',
  },
  [ListAdminTasksStatus.cancelled]: {
    severity: 'secondary',
    text: '已停止',
    color: DEFAULT_STATUS_COLOR,
    bannerClass: DEFAULT_BANNER_CLASS,
    dotClass: DEFAULT_DOT_CLASS,
  },
}

export function getTaskStatusDefinition(status: string): TaskStatusDefinition {
  return (
    taskStatusDefinitions[status as ListAdminTasksStatusValue] || {
      severity: 'secondary',
      text: status,
      color: DEFAULT_STATUS_COLOR,
      bannerClass: DEFAULT_BANNER_CLASS,
      dotClass: DEFAULT_DOT_CLASS,
    }
  )
}
