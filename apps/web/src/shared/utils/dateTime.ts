import { getLocaleDateFormatter } from '../i18n/formatters'
import { getActiveUserLocale } from '../i18n/userLocale'

function toDate(input: string | Date): Date | null {
  const d = typeof input === 'string' ? new Date(input) : input
  return Number.isNaN(d.getTime()) ? null : d
}

/**
 * Canonical `YYYY-MM-DD` key (locale-invariant, e.g. chart buckets, stable labels).
 * Local calendar fields keep it stable without `Intl` overhead.
 */
export function formatDateOnly(input: string | Date): string {
  const d = toDate(input)
  if (!d) return ''
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

/** Locale-aware admin timestamp. Falls back when the input is invalid. */
export function formatAdminDateTime(input: string | Date, fallback = '-'): string {
  const d = toDate(input)
  if (!d) return fallback
  return getLocaleDateFormatter(getActiveUserLocale(), {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(d)
}

export function calcDurationSeconds(start: string | Date, end: string | Date): number | null {
  const startTime = typeof start === 'string' ? new Date(start).getTime() : start.getTime()
  const endTime = typeof end === 'string' ? new Date(end).getTime() : end.getTime()
  if (isNaN(startTime) || isNaN(endTime) || endTime < startTime) return null
  return Math.round(Math.max(0, endTime - startTime) / 1000)
}

export function formatDurationSeconds(start: string | Date, end: string | Date): string {
  const seconds = calcDurationSeconds(start, end)
  return seconds == null ? '-' : `${seconds}s`
}
