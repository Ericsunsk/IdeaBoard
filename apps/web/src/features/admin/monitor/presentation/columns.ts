import type { Component } from 'vue'
import { getServiceIcon } from '../../../../shared/domain/serviceIcons'
import type { TaskMetricDefinition } from '../../shared/lib/taskMetricTokens'
import { getTaskMetricByStatus } from '../../shared/lib/taskMetricTokens'
import { ADMIN_IDENTITY_EMPTY } from '../../shared/lib/cellPlaceholders'
import { ADMIN_TABLE_MORE_COLUMN_WIDTH } from '../../shared/lib/tableColumns'
import type { AdminMonitorListItemModel } from '../infrastructure/resource'
import { TASK_RESULT_PREVIEW_COUNT, getTaskResultImageUrls } from '../domain/taskResultPresentation'
import { toGmt8Date } from '../application/useMonitorTimeline'

// 表格时间戳：精确到秒的 GMT+8 时间（与表头标注一致）。
export function formatTaskExactTimestamp(data: AdminMonitorListItemModel) {
  if (!data.created_at) return ADMIN_IDENTITY_EMPTY
  const date = new Date(data.created_at)
  if (isNaN(date.getTime())) return String(data.created_at)

  const gmt8 = toGmt8Date(date)

  const yyyy = gmt8.getFullYear()
  const mm = String(gmt8.getMonth() + 1).padStart(2, '0')
  const dd = String(gmt8.getDate()).padStart(2, '0')
  const hh = String(gmt8.getHours()).padStart(2, '0')
  const min = String(gmt8.getMinutes()).padStart(2, '0')
  const ss = String(gmt8.getSeconds()).padStart(2, '0')

  return `${yyyy}-${mm}-${dd} ${hh}:${min}:${ss}`
}

// Events 表各列定宽（单源，调列宽只改这里）。
export const adminMonitorColumnWidths = {
  timestamp: '190px',
  duration: '90px',
  status: '105px',
  service: '140px',
  model: '156px',
  message: '200px',
  credits: '110px',
  result: '140px',
  actions: '100px',
  more: ADMIN_TABLE_MORE_COLUMN_WIDTH,
} as const

// Model 列展示最终解析模型 (#852)：优先 attempt 事实归因的
// final_resolved_model，回退到 latest_execution（快照选中模型）。
export function getTaskModelLabel(row: AdminMonitorListItemModel) {
  return row.final_resolved_model || row.latest_execution?.resolved_model || ''
}

// Service 列展示 service + 请求清晰度：图标 + slug + resolution。
export function getTaskServiceSlug(row: AdminMonitorListItemModel) {
  return row.service || ''
}

export function getTaskServiceResolution(row: AdminMonitorListItemModel) {
  return row.requested_resolution || ''
}

// Service 列图标单源已上移 shared/domain：此处保留别名，调用方不动。
export const getTaskServiceIcon = getServiceIcon

// Message 列展示上限：超出截断加省略号，完整原文由调用方放 title。
export const TASK_MESSAGE_MAX_LENGTH = 120

// Service 列已独立展示 service：Message 仅剥离与本行 service 一致的前缀
//（如 "render: ..."），用户原文其它内容原样保留。
export function getTaskMessageText(row: AdminMonitorListItemModel) {
  const prompt = row.prompt || ''
  const service = getTaskServiceSlug(row).trim().toLowerCase()
  if (!service) return truncateTaskMessage(prompt)
  const match = new RegExp(`^${escapeRegExp(service)}\\s*[:：\\-\\/]?\\s*`, 'i').exec(prompt)
  if (!match) return truncateTaskMessage(prompt)
  const stripped = prompt.slice(match[0].length)
  return truncateTaskMessage(stripped || prompt)
}

function truncateTaskMessage(value: string) {
  if (value.length <= TASK_MESSAGE_MAX_LENGTH) return value
  return `${value.slice(0, TASK_MESSAGE_MAX_LENGTH)} ...`
}

function escapeRegExp(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

// Result 列预览：取前 N 张，超出部分返回 overflow 计数由调用方展示 +N。
export function getTaskResultPreview(row: AdminMonitorListItemModel): {
  urls: string[]
  overflow: number
} {
  const all = getTaskResultImageUrls(row)
  return {
    urls: all.slice(0, TASK_RESULT_PREVIEW_COUNT),
    overflow: Math.max(0, all.length - TASK_RESULT_PREVIEW_COUNT),
  }
}

export type AdminTaskRowView = {
  metric: TaskMetricDefinition | undefined
  serviceSlug: string
  serviceIcon: Component | undefined
  message: string
  preview: { urls: string[]; overflow: number }
}

// 行级展示 memo：同一行对象在多次渲染间只计算一次（query 返回新对象即失效重算）。
const rowViewCache = new WeakMap<AdminMonitorListItemModel, AdminTaskRowView>()

export function getTaskRowView(row: AdminMonitorListItemModel): AdminTaskRowView {
  const cached = rowViewCache.get(row)
  if (cached) return cached
  const serviceSlug = getTaskServiceSlug(row)
  const view: AdminTaskRowView = {
    metric: getTaskMetricByStatus(row.status),
    serviceSlug,
    serviceIcon: getTaskServiceIcon(serviceSlug),
    message: getTaskMessageText(row),
    preview: getTaskResultPreview(row),
  }
  rowViewCache.set(row, view)
  return view
}

export function getTaskPriorityLabel(row: Pick<AdminMonitorListItemModel, 'priority'>) {
  const priority = Number(row.priority || 2)
  return `P${Number.isFinite(priority) ? priority : 2}`
}
