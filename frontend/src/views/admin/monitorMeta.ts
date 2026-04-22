import { h } from 'vue'
import { NTag } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import type { Task } from '../../api/account'

export interface AccountLoad {
  current_concurrent: number
  max_concurrent: number
}

export interface MonitorStats {
  pending: number
  running: number
  completed: number
  failed: number
}

export interface MonitorState {
  global_queue_len: number
  wait_queue_len: number
  running_tasks: number
  account_loads: Record<number, AccountLoad>
}

type TaskStatusTagType = 'default' | 'warning' | 'success' | 'error'

interface TaskStatusConfig {
  type: TaskStatusTagType
  text: string
  color: string
}

export const TASK_PAGE_SIZE = 20
export const MONITOR_REFRESH_INTERVAL_MS = 5000
export const GLOBAL_QUEUE_CAPACITY = 10000
export const GLOBAL_QUEUE_ALERT_THRESHOLD = 1000
export const RUNNING_TASK_PRESSURE_THRESHOLD = 200
export const ERROR_TEXT_COLOR = '#ef4444'
export const DEFAULT_STATUS_COLOR = '#71717a'

export const taskStatusOptions = [
  { label: '等待中', value: 'pending' },
  { label: '执行中', value: 'running' },
  { label: '已完成', value: 'completed' },
  { label: '失败', value: 'failed' },
]

const taskStatusConfigMap: Record<string, TaskStatusConfig> = {
  pending: { type: 'default', text: '等待中', color: DEFAULT_STATUS_COLOR },
  running: { type: 'warning', text: '执行中', color: '#f59e0b' },
  completed: { type: 'success', text: '已完成', color: '#22c55e' },
  failed: { type: 'error', text: '失败', color: ERROR_TEXT_COLOR },
}

export const emptyMonitorStats: MonitorStats = {
  pending: 0,
  running: 0,
  completed: 0,
  failed: 0,
}

export const emptyMonitorState: MonitorState = {
  global_queue_len: 0,
  wait_queue_len: 0,
  running_tasks: 0,
  account_loads: {},
}

export function getTaskStatusConfig(status: string): TaskStatusConfig {
  return taskStatusConfigMap[status] || { type: 'default', text: status, color: DEFAULT_STATUS_COLOR }
}

export function calculateCappedRate(value: number, threshold: number): number {
  return Math.min(100, (value / threshold) * 100)
}

export function calculateAccountLoadRate(load: AccountLoad): number {
  return (load.current_concurrent / load.max_concurrent) * 100
}

export function isAccountSaturated(load: AccountLoad): boolean {
  return load.current_concurrent >= load.max_concurrent
}

export function createMonitorTaskColumns(): DataTableColumns<Task> {
  return [
    {
      title: '任务ID',
      key: 'task_id',
      width: 200,
      render: (row) => h('span', { style: { fontFamily: 'monospace', fontSize: '12px' } }, row.task_id),
    },
    { title: '账号', key: 'account_id', width: 80, align: 'center' },
    {
      title: '状态',
      key: 'status',
      width: 100,
      render: (row) => {
        const config = getTaskStatusConfig(row.status)
        return h(NTag, { type: config.type, size: 'small' }, () => config.text)
      },
    },
    { title: '重试', key: 'retry_count', width: 60, align: 'center' },
    { title: '创建时间', key: 'created_at', width: 170 },
    {
      title: '错误信息',
      key: 'error_msg',
      ellipsis: { tooltip: true },
      render: (row) =>
        row.error_msg ? h('span', { style: { color: ERROR_TEXT_COLOR, fontSize: '12px' } }, row.error_msg) : '-',
    },
  ]
}
