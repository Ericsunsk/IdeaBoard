import { TaskStatus, type TaskStatus as TaskStatusValue } from '../../shared/lib/taskMetricTokens'

// Pill 直连：与 AdminStatusPillVariant 同构，domain 内自有定义以免反向依赖组件层。
export type TaskStatusPillVariant = 'success' | 'danger' | 'warning' | 'neutral' | 'info'

export interface TaskStatusDefinition {
  variant: TaskStatusPillVariant
  text: string
  color: string
  bannerClass: string
  dotClass: string
}

const DEFAULT_STATUS_COLOR = 'var(--color-muted-foreground)'
const DEFAULT_BANNER_CLASS =
  'bg-[linear-gradient(135deg,var(--color-admin-status-default-glow),var(--color-admin-status-wash))] before:bg-muted-foreground'
const DEFAULT_DOT_CLASS = 'bg-muted-foreground shadow-admin-status-dot-default'

// Pending 与 Events 表 metric 面板同为 info 系（taskMetricTokens），双源不再分叉。
const PENDING_STATUS_COLOR = 'var(--color-info)'
const PENDING_BANNER_CLASS =
  'bg-[linear-gradient(135deg,var(--color-admin-status-pending-glow),var(--color-admin-status-wash))] before:bg-info'
const PENDING_DOT_CLASS = 'bg-info shadow-admin-status-dot-pending'

export const adminTaskStatuses = TaskStatus

const taskStatusDefinitions: Record<TaskStatusValue, TaskStatusDefinition> = {
  [TaskStatus.pending]: {
    variant: 'info',
    text: '等待中',
    color: PENDING_STATUS_COLOR,
    bannerClass: PENDING_BANNER_CLASS,
    dotClass: PENDING_DOT_CLASS,
  },
  [TaskStatus.running]: {
    variant: 'warning',
    text: '执行中',
    color: 'var(--color-warning)',
    bannerClass:
      'bg-[linear-gradient(135deg,var(--color-admin-status-running-glow),var(--color-admin-status-wash))] before:bg-warning',
    dotClass: 'bg-warning shadow-admin-status-dot-running',
  },
  [TaskStatus.completed]: {
    variant: 'success',
    text: '已完成',
    color: 'var(--color-success)',
    bannerClass:
      'bg-[linear-gradient(135deg,var(--color-admin-status-success-glow),var(--color-admin-status-wash))] before:bg-success',
    dotClass: 'bg-success shadow-admin-status-dot-success',
  },
  [TaskStatus.failed]: {
    variant: 'danger',
    text: '失败',
    color: 'var(--color-destructive)',
    bannerClass:
      'bg-[linear-gradient(135deg,var(--color-admin-status-danger-glow),var(--color-admin-status-wash))] before:bg-destructive',
    dotClass: 'bg-destructive shadow-admin-status-dot-danger',
  },
  [TaskStatus.cancelled]: {
    variant: 'neutral',
    text: '已停止',
    color: DEFAULT_STATUS_COLOR,
    bannerClass: DEFAULT_BANNER_CLASS,
    dotClass: DEFAULT_DOT_CLASS,
  },
}

export function getTaskStatusDefinition(status: string): TaskStatusDefinition {
  return (
    taskStatusDefinitions[status as TaskStatusValue] || {
      variant: 'neutral',
      text: status,
      color: DEFAULT_STATUS_COLOR,
      bannerClass: DEFAULT_BANNER_CLASS,
      dotClass: DEFAULT_DOT_CLASS,
    }
  )
}
