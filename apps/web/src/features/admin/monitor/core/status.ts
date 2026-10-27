import { TaskStatus, type TaskStatus as TaskStatusValue } from '../../shared'

// Pill 直连：与 StatusBadge `tone` 同构，domain 内自有定义以免反向依赖组件层。
// 状态字面量经 shared/single-source 指向 v1 contract（见 taskMetricTokens），
// monitor 与 metric 不再各持一份。
type TaskStatusTone = 'success' | 'danger' | 'warning' | 'neutral' | 'info'

export interface TaskStatusDefinition {
  tone: TaskStatusTone
  text: string
  color: string
  bannerClass: string
  dotClass: string
}

const DEFAULT_STATUS_COLOR = 'var(--muted-foreground)'
const DEFAULT_BANNER_CLASS =
  'bg-[linear-gradient(135deg,var(--color-admin-status-default-glow),var(--color-admin-status-wash))] before:bg-muted-foreground'
const DEFAULT_DOT_CLASS = 'bg-muted-foreground shadow-admin-status-dot-default'

// Pending 与 Events 表 metric 面板同为 info 系（taskMetricTokens），双源不再分叉。
const PENDING_STATUS_COLOR = 'var(--info)'
const PENDING_BANNER_CLASS =
  'bg-[linear-gradient(135deg,var(--color-admin-status-pending-glow),var(--color-admin-status-wash))] before:bg-info'
const PENDING_DOT_CLASS = 'bg-info shadow-admin-status-dot-pending'

export const adminTaskStatuses = TaskStatus

const taskStatusDefinitions: Record<TaskStatusValue, TaskStatusDefinition> = {
  [TaskStatus.pending]: {
    tone: 'info',
    text: '等待中',
    color: PENDING_STATUS_COLOR,
    bannerClass: PENDING_BANNER_CLASS,
    dotClass: PENDING_DOT_CLASS,
  },
  [TaskStatus.running]: {
    tone: 'warning',
    text: '执行中',
    color: 'var(--warning)',
    bannerClass:
      'bg-[linear-gradient(135deg,var(--color-admin-status-running-glow),var(--color-admin-status-wash))] before:bg-warning',
    dotClass: 'bg-warning shadow-admin-status-dot-running',
  },
  [TaskStatus.completed]: {
    tone: 'success',
    text: '已完成',
    color: 'var(--success)',
    bannerClass:
      'bg-[linear-gradient(135deg,var(--color-admin-status-success-glow),var(--color-admin-status-wash))] before:bg-success',
    dotClass: 'bg-success shadow-admin-status-dot-success',
  },
  [TaskStatus.failed]: {
    tone: 'danger',
    text: '失败',
    color: 'var(--destructive)',
    bannerClass:
      'bg-[linear-gradient(135deg,var(--color-admin-status-danger-glow),var(--color-admin-status-wash))] before:bg-destructive',
    dotClass: 'bg-destructive shadow-admin-status-dot-danger',
  },
  [TaskStatus.cancelled]: {
    tone: 'neutral',
    text: '已停止',
    color: DEFAULT_STATUS_COLOR,
    bannerClass: DEFAULT_BANNER_CLASS,
    dotClass: DEFAULT_DOT_CLASS,
  },
}

export function getTaskStatusDefinition(status: string): TaskStatusDefinition {
  return (
    taskStatusDefinitions[status as TaskStatusValue] || {
      tone: 'neutral',
      text: status,
      color: DEFAULT_STATUS_COLOR,
      bannerClass: DEFAULT_BANNER_CLASS,
      dotClass: DEFAULT_DOT_CLASS,
    }
  )
}
