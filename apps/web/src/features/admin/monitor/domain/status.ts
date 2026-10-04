import { TaskStatus, type TaskStatus as TaskStatusValue } from '../../shared/lib/taskMetricTokens'

type TaskStatusTagSeverity = 'secondary' | 'warn' | 'success' | 'danger'

export interface TaskStatusDefinition {
  severity: TaskStatusTagSeverity
  text: string
  color: string
  bannerClass: string
  dotClass: string
}

const DEFAULT_STATUS_COLOR = 'var(--color-muted-foreground)'
const DEFAULT_BANNER_CLASS =
  'bg-[linear-gradient(135deg,rgba(152,162,179,0.12),rgba(255,255,255,0.74))] before:bg-muted-foreground'
const DEFAULT_DOT_CLASS = 'bg-muted-foreground shadow-[0_0_0_4px_rgba(152,162,179,0.14)]'

// Pending 与 Events 表 metric 面板同为 info 系（taskMetricTokens），双源不再分叉。
const PENDING_STATUS_COLOR = 'var(--color-info)'
const PENDING_BANNER_CLASS =
  'bg-[linear-gradient(135deg,rgba(71,117,168,0.1),rgba(255,255,255,0.74))] before:bg-info'
const PENDING_DOT_CLASS = 'bg-info shadow-[0_0_0_4px_rgba(71,117,168,0.15)]'

export const adminTaskStatuses = TaskStatus

const taskStatusDefinitions: Record<TaskStatusValue, TaskStatusDefinition> = {
  [TaskStatus.pending]: {
    severity: 'secondary',
    text: '等待中',
    color: PENDING_STATUS_COLOR,
    bannerClass: PENDING_BANNER_CLASS,
    dotClass: PENDING_DOT_CLASS,
  },
  [TaskStatus.running]: {
    severity: 'warn',
    text: '执行中',
    color: 'var(--color-warning)',
    bannerClass:
      'bg-[linear-gradient(135deg,rgba(164,105,37,0.1),rgba(255,255,255,0.74))] before:bg-warning',
    dotClass: 'bg-warning shadow-[0_0_0_4px_rgba(164,105,37,0.15)]',
  },
  [TaskStatus.completed]: {
    severity: 'success',
    text: '已完成',
    color: 'var(--color-success)',
    bannerClass:
      'bg-[linear-gradient(135deg,rgba(40,134,97,0.1),rgba(255,255,255,0.74))] before:bg-success',
    dotClass: 'bg-success shadow-[0_0_0_4px_rgba(40,134,97,0.14)]',
  },
  [TaskStatus.failed]: {
    severity: 'danger',
    text: '失败',
    color: 'var(--color-destructive)',
    bannerClass:
      'bg-[linear-gradient(135deg,rgba(201,74,86,0.095),rgba(255,255,255,0.74))] before:bg-destructive',
    dotClass: 'bg-destructive shadow-[0_0_0_4px_rgba(201,74,86,0.14)]',
  },
  [TaskStatus.cancelled]: {
    severity: 'secondary',
    text: '已停止',
    color: DEFAULT_STATUS_COLOR,
    bannerClass: DEFAULT_BANNER_CLASS,
    dotClass: DEFAULT_DOT_CLASS,
  },
}

export function getTaskStatusDefinition(status: string): TaskStatusDefinition {
  return (
    taskStatusDefinitions[status as TaskStatusValue] || {
      severity: 'secondary',
      text: status,
      color: DEFAULT_STATUS_COLOR,
      bannerClass: DEFAULT_BANNER_CLASS,
      dotClass: DEFAULT_DOT_CLASS,
    }
  )
}
