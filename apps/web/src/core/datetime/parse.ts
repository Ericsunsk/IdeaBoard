/** Returns a valid date or null; callers own missing/invalid display copy. */
export function parseDateGuard(input: string | Date | null | undefined): Date | null {
  if (input == null) return null
  const date = typeof input === 'string' ? new Date(input) : input
  if (!(date instanceof Date) || Number.isNaN(date.getTime())) return null
  return date
}
