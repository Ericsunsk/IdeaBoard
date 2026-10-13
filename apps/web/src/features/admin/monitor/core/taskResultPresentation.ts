import { calcDurationSeconds, formatDurationSeconds } from '../../../../core/utils/dateTime'
import {
  sanitizeApiErrorMessage,
  sanitizeTaskErrorMessage,
} from '../../../../core/utils/errorMessages'
import { i18n } from '../../../../core/i18n'
import { adminTaskStatuses } from './status'
import { getSafeTaskResultImageUrl, getSafeTaskResultImageUrls } from './taskResultImages'
import { isRecord } from '../../../../core/utils/guards'
import type { AdminMonitorDetailModel, AdminMonitorListItemModel } from '../resource'

const RESULT_MAX_STRING_LENGTH = 300
const RESULT_DEBUG_MAX_DEPTH = 4
const RESULT_DEBUG_MAX_KEYS = 40
const RESULT_ARRAY_PREVIEW_LENGTH = 5
const SENSITIVE_KEY_PATTERN =
  /token|secret|password|cookie|authorization|api[-_]?key|signature|credential/i

type TaskDurationTone = 'success' | 'warning' | 'danger'
type TaskDurationInfo = {
  seconds: number
  tone: TaskDurationTone
}
type MonitorResultRow = AdminMonitorListItemModel | AdminMonitorDetailModel

export function getTaskResultImageUrl(row: Pick<MonitorResultRow, 'status' | 'result'>): string {
  return getTaskResultImageUrls(row)[0] ?? ''
}

export function getTaskResultDetailImageUrl(
  row: Pick<MonitorResultRow, 'result'> | null | undefined,
): string {
  if (!row?.result) return ''
  return getSafeTaskResultImageUrl(row.result)
}

// 表格 Result 列：最多展示前 3 张，超出部分由调用方用 +N 提示。
export const TASK_RESULT_PREVIEW_COUNT = 3

export function getTaskResultImageUrls(row: Pick<MonitorResultRow, 'status' | 'result'>): string[] {
  if (row.status !== adminTaskStatuses.completed || !row.result) return []
  return getSafeTaskResultImageUrls(row.result)
}

export function getTaskDuration(
  row: Pick<MonitorResultRow, 'created_at' | 'completed_at'>,
): TaskDurationInfo | null {
  if (!row.completed_at || !row.created_at) return null
  const seconds = calcDurationSeconds(row.created_at, row.completed_at)
  if (seconds == null) return null
  const tone: TaskDurationTone = seconds > 60 ? 'danger' : seconds > 30 ? 'warning' : 'success'
  return { seconds, tone }
}

export function formatTaskDuration(
  row: Pick<MonitorResultRow, 'created_at' | 'completed_at'>,
): string {
  if (!row.completed_at || !row.created_at) return '-'
  return formatDurationSeconds(row.created_at, row.completed_at)
}

export function getTaskSanitizedError(row: Pick<MonitorResultRow, 'status' | 'error_msg'>): string {
  if (row.status !== adminTaskStatuses.failed || !row.error_msg) return ''
  return sanitizeTaskErrorMessage(row.error_msg, taskErrorFallback())
}

export function getTaskErrorReason(
  row: Pick<MonitorResultRow, 'status' | 'error_msg' | 'result'>,
): string {
  if (row.error_msg) return sanitizeTaskErrorMessage(row.error_msg, taskErrorFallback())
  if (!row.result) return ''

  const parsed = parseResultSafely(row.result)
  if (isRecord(parsed)) {
    const error = parsed.error
    if (error) return sanitizeTaskErrorMessage(String(error), taskErrorFallback())
    return ''
  }

  if (parsed == null && row.status === adminTaskStatuses.failed)
    return sanitizeTaskErrorMessage(row.result, taskErrorFallback())
  return ''
}

/** Fallback copy is unchanged (same key, now resolved by the caller). */
function taskErrorFallback(): string {
  return i18n.global.t('canvas.task.failedGeneric')
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
