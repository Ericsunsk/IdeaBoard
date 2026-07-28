import { calcDurationSeconds, formatDurationSeconds } from '../../../shared/utils/dateTime'
import {
  sanitizeApiErrorMessage,
  sanitizeTaskErrorMessage,
} from '../../../shared/utils/errorMessages'
import { adminTaskStatuses } from './taskMonitorStatus'
import { getSafeTaskResultImageUrl } from './taskResultImages'
import type {
  AdminTaskMonitorDetailModel,
  AdminTaskMonitorListItemModel,
} from './taskMonitorResource'

const RESULT_MAX_STRING_LENGTH = 300
const RESULT_DEBUG_MAX_DEPTH = 4
const RESULT_DEBUG_MAX_KEYS = 40
const RESULT_ARRAY_PREVIEW_LENGTH = 5
const SENSITIVE_KEY_PATTERN =
  /token|secret|password|cookie|authorization|api[-_]?key|signature|credential/i

export type TaskDurationTone = 'success' | 'warning' | 'danger'
export type TaskDurationInfo = {
  seconds: number
  tone: TaskDurationTone
}
export type TaskApiCallInfo = {
  platform: string
}

type TaskMonitorResultRow = AdminTaskMonitorListItemModel | AdminTaskMonitorDetailModel

const taskApiCallCache = new WeakMap<
  TaskMonitorResultRow,
  { result: string; apiType: string; value: TaskApiCallInfo | null }
>()

export function getTaskResultImageUrl(
  row: Pick<TaskMonitorResultRow, 'status' | 'result'>,
): string {
  if (row.status !== adminTaskStatuses.completed || !row.result) return ''
  return getSafeTaskResultImageUrl(row.result)
}

export function getTaskResultDetailImageUrl(
  row: Pick<TaskMonitorResultRow, 'result'> | null | undefined,
): string {
  if (!row?.result) return ''
  return getSafeTaskResultImageUrl(row.result)
}

export function getTaskApiCall(row: TaskMonitorResultRow): TaskApiCallInfo | null {
  const apiType = row.latest_execution?.api_type || ''
  if (!apiType) return null
  const cached = taskApiCallCache.get(row)
  const result = row.result || ''
  if (cached?.result === result && cached.apiType === apiType) return cached.value

  const value: TaskApiCallInfo = { platform: apiType }

  taskApiCallCache.set(row, { result, apiType, value })
  return value
}

export function getTaskDuration(
  row: Pick<TaskMonitorResultRow, 'created_at' | 'completed_at'>,
): TaskDurationInfo | null {
  if (!row.completed_at || !row.created_at) return null
  const seconds = calcDurationSeconds(row.created_at, row.completed_at)
  if (seconds == null) return null
  const tone: TaskDurationTone = seconds > 60 ? 'danger' : seconds > 30 ? 'warning' : 'success'
  return { seconds, tone }
}

export function formatTaskDuration(
  row: Pick<TaskMonitorResultRow, 'created_at' | 'completed_at'>,
): string {
  if (!row.completed_at || !row.created_at) return '-'
  return formatDurationSeconds(row.created_at, row.completed_at)
}

export function getTaskSanitizedError(
  row: Pick<TaskMonitorResultRow, 'status' | 'error_msg'>,
): string {
  if (row.status !== adminTaskStatuses.failed || !row.error_msg) return ''
  return sanitizeTaskErrorMessage(row.error_msg)
}

export function getTaskErrorReason(
  row: Pick<TaskMonitorResultRow, 'status' | 'error_msg' | 'result'>,
): string {
  if (row.error_msg) return sanitizeTaskErrorMessage(row.error_msg)
  if (!row.result) return ''

  const parsed = parseResultSafely(row.result)
  if (isRecord(parsed)) {
    const error = parsed.error
    if (error) return sanitizeTaskErrorMessage(String(error))
    return ''
  }

  if (parsed == null && row.status === adminTaskStatuses.failed)
    return sanitizeTaskErrorMessage(row.result)
  return ''
}

export function formatTaskResultSummary(result: string | null | undefined): string {
  if (!result) return ''
  const parsed = parseResultSafely(result)
  if (parsed == null) return sanitizeApiErrorMessage(result, '[REDACTED]')
  if (Array.isArray(parsed)) {
    return JSON.stringify(
      { type: 'array', length: parsed.length, preview: sanitizeValue(parsed, 1) },
      null,
      2,
    )
  }
  if (!isRecord(parsed)) return truncateString(String(parsed))

  const summary: Record<string, unknown> = {}
  const preferredKeys = [
    'status',
    'parsed_output_status',
    'platform',
    'api_type',
    'model',
    'request_id',
    'task_id',
    'correlation_id',
    'code',
    'message',
    'error',
    'failure_stage',
    'failure_category',
    'duration',
    'image_url',
    'raw_provider_output_preview',
  ]
  preferredKeys.forEach((key) => {
    if (key in parsed) summary[key] = sanitizeValue(parsed[key], 0)
  })
  if ('parsed_output' in parsed) summary.parsed_output = sanitizeValue(parsed.parsed_output, 0)
  if ('images' in parsed) summary.images = Array.isArray(parsed.images) ? parsed.images.length : 0
  if (Object.keys(summary).length === 0) {
    summary.keys = Object.keys(parsed).slice(0, 12)
    summary.preview = sanitizeValue(parsed, 1)
  }
  return JSON.stringify(summary, null, 2)
}

export function formatTaskResultDebug(result: string | null | undefined): string {
  if (!result) return ''
  const parsed = parseResultSafely(result)
  if (parsed == null) return sanitizeApiErrorMessage(result, '[REDACTED]')
  return JSON.stringify(sanitizeValue(parsed, 0), null, 2)
}

function parseResultSafely(str: string): unknown | null {
  try {
    return JSON.parse(str)
  } catch {
    return null
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value != null && typeof value === 'object' && !Array.isArray(value)
}

function truncateString(value: string, maxLength = RESULT_MAX_STRING_LENGTH): string {
  if (value.length <= maxLength) return value
  return `${value.slice(0, maxLength)}...(+${value.length - maxLength} chars)`
}

function sanitizeValue(value: unknown, depth: number, keyCountState = { count: 0 }): unknown {
  if (value == null) return value
  if (typeof value === 'string') {
    const sanitized = sanitizeApiErrorMessage(value, '[REDACTED]')
    return sanitized === '[REDACTED]' ? sanitized : truncateString(sanitized)
  }
  if (typeof value === 'number' || typeof value === 'boolean') return value
  if (Array.isArray(value)) {
    if (depth >= RESULT_DEBUG_MAX_DEPTH) return `[array:${value.length}]`
    const preview = value
      .slice(0, RESULT_ARRAY_PREVIEW_LENGTH)
      .map((item) => sanitizeValue(item, depth + 1, keyCountState))
    if (value.length > RESULT_ARRAY_PREVIEW_LENGTH)
      preview.push(`...(+${value.length - RESULT_ARRAY_PREVIEW_LENGTH} items)`)
    return preview
  }
  if (typeof value === 'object') {
    if (depth >= RESULT_DEBUG_MAX_DEPTH) return '[object]'
    const obj = value as Record<string, unknown>
    const entries = Object.entries(obj)
    const sanitized: Record<string, unknown> = {}
    for (const [key, nestedValue] of entries) {
      if (keyCountState.count >= RESULT_DEBUG_MAX_KEYS) {
        sanitized.__truncated__ = `...(+${entries.length - Object.keys(sanitized).length} keys)`
        break
      }
      keyCountState.count += 1
      if (SENSITIVE_KEY_PATTERN.test(key)) sanitized[key] = '[REDACTED]'
      else sanitized[key] = sanitizeValue(nestedValue, depth + 1, keyCountState)
    }
    return sanitized
  }
  return String(value)
}
