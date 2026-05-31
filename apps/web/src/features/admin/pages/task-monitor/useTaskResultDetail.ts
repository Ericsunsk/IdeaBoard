import {
  sanitizeTaskErrorMessage,
  sanitizeApiErrorMessage,
} from '../../../../shared/utils/errorMessages'
import { formatAdminDateTime, formatDurationSeconds } from '../../../../shared/utils/dateTime'
import type { Task } from '../../../../shared/api/task'

const RESULT_MAX_STRING_LENGTH = 300
const RESULT_DEBUG_MAX_DEPTH = 4
const RESULT_DEBUG_MAX_KEYS = 40
const RESULT_ARRAY_PREVIEW_LENGTH = 5
const SENSITIVE_KEY_PATTERN =
  /token|secret|password|cookie|authorization|api[-_]?key|signature|credential/i

export function useTaskResultDetail(options: {
  message: { success: (content: string) => void; error: (content: string) => void }
}) {
  const detailModalVisible = ref(false)
  const detailTask = ref<Task | null>(null)
  const showDebugResult = ref(false)

  const resultImageUrl = computed(() => {
    if (!detailTask.value?.result) return ''
    try {
      const r = JSON.parse(detailTask.value.result)
      return r.image_url || ''
    } catch {
      return ''
    }
  })

  const resultSummaryText = computed(() => {
    if (!detailTask.value?.result) return ''
    const parsed = parseResultSafely(detailTask.value.result)
    if (parsed == null) return sanitizeApiErrorMessage(detailTask.value.result, '[REDACTED]')
    if (Array.isArray(parsed)) {
      return JSON.stringify(
        { type: 'array', length: parsed.length, preview: sanitizeValue(parsed, 1) },
        null,
        2,
      )
    }
    if (typeof parsed !== 'object') return truncateString(String(parsed))

    const summary: Record<string, unknown> = {}
    const source = parsed as Record<string, unknown>
    const preferredKeys = [
      'status',
      'platform',
      'model_set',
      'model',
      'request_id',
      'task_id',
      'code',
      'message',
      'error',
      'duration',
      'image_url',
    ]
    preferredKeys.forEach((key) => {
      if (key in source) summary[key] = sanitizeValue(source[key], 0)
    })
    if ('images' in source) summary.images = Array.isArray(source.images) ? source.images.length : 0
    if (Object.keys(summary).length === 0) {
      summary.keys = Object.keys(source).slice(0, 12)
      summary.preview = sanitizeValue(source, 1)
    }
    return JSON.stringify(summary, null, 2)
  })

  const resultDebugText = computed(() => {
    if (!detailTask.value?.result) return ''
    const parsed = parseResultSafely(detailTask.value.result)
    if (parsed == null) return sanitizeApiErrorMessage(detailTask.value.result, '[REDACTED]')
    return JSON.stringify(sanitizeValue(parsed, 0), null, 2)
  })

  function openResultImage(url: string) {
    if (!url) return
    window.open(url, '_blank', 'noopener,noreferrer')
  }

  function copyToClipboard(text: string) {
    if (!text) return
    navigator.clipboard
      .writeText(text)
      .then(() => options.message.success('已复制到剪贴板'))
      .catch(() => {
        options.message.error('复制失败')
      })
  }

  function formatDateTime(timeStr: string | null | undefined): string {
    if (!timeStr) return '-'
    return formatAdminDateTime(timeStr, timeStr)
  }

  function showDetail(row: Task) {
    detailTask.value = row
    showDebugResult.value = false
    detailModalVisible.value = true
  }

  function getErrorReason(row: Task): string {
    if (row.error_msg) return sanitizeTaskErrorMessage(row.error_msg)
    if (!row.result) return ''
    try {
      const r = JSON.parse(row.result)
      if (r.error) return sanitizeTaskErrorMessage(r.error)
      if (r.status === 'failed' && r.error) return sanitizeTaskErrorMessage(r.error)
    } catch {
      if (row.status === 'failed') return sanitizeTaskErrorMessage(row.result)
    }
    return ''
  }

  function calcDuration(row: Task): string {
    if (!row.completed_at || !row.created_at) return '-'
    return formatDurationSeconds(row.created_at, row.completed_at)
  }

  return {
    detailModalVisible,
    detailTask,
    showDebugResult,
    resultImageUrl,
    resultSummaryText,
    resultDebugText,
    openResultImage,
    copyToClipboard,
    formatDateTime,
    showDetail,
    getErrorReason,
    calcDuration,
  }
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
    const result: Record<string, unknown> = {}
    for (const [k, v] of entries) {
      if (keyCountState.count >= RESULT_DEBUG_MAX_KEYS) {
        result.__truncated__ = `...(+${entries.length - Object.keys(result).length} keys)`
        break
      }
      keyCountState.count += 1
      if (SENSITIVE_KEY_PATTERN.test(k)) result[k] = '[REDACTED]'
      else result[k] = sanitizeValue(v, depth + 1, keyCountState)
    }
    return result
  }
  return String(value)
}
