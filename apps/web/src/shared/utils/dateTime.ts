/** Date/time formatting utilities — no external deps. */

export function formatDateOnly(input: string | Date): string {
  const d = typeof input === 'string' ? new Date(input) : input
  if (isNaN(d.getTime())) return ''
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

export function formatDateTimeCompact(input: string | Date): string {
  const d = typeof input === 'string' ? new Date(input) : input
  if (isNaN(d.getTime())) return ''
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  const h = String(d.getHours()).padStart(2, '0')
  const min = String(d.getMinutes()).padStart(2, '0')
  return `${y}-${m}-${day} ${h}:${min}`
}

export function formatAdminDateTime(input: string | Date, fallback = '-'): string {
  const d = typeof input === 'string' ? new Date(input) : input
  if (isNaN(d.getTime())) return fallback
  const h = String(d.getHours()).padStart(2, '0')
  const min = String(d.getMinutes()).padStart(2, '0')
  return `${d.getFullYear()}.${d.getMonth() + 1}.${d.getDate()}-${h}:${min}`
}

export function calcDurationMs(start: number, end = Date.now()): number {
  return Math.max(0, end - start)
}

export function calcDurationSeconds(start: string | Date, end: string | Date): number | null {
  const startTime = typeof start === 'string' ? new Date(start).getTime() : start.getTime()
  const endTime = typeof end === 'string' ? new Date(end).getTime() : end.getTime()
  if (isNaN(startTime) || isNaN(endTime) || endTime < startTime) return null
  return Math.round(calcDurationMs(startTime, endTime) / 1000)
}

export function formatDurationSeconds(start: string | Date, end: string | Date): string {
  const seconds = calcDurationSeconds(start, end)
  return seconds == null ? '-' : `${seconds}s`
}
