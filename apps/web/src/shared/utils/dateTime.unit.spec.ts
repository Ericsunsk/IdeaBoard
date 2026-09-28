import { describe, expect, it } from 'vitest'
import { formatDottedIsoDate } from './dateTime'

describe('formatDottedIsoDate', () => {
  it('renders the verbatim ISO date part with dots and no timezone shift', () => {
    expect(formatDottedIsoDate('2026-07-14T23:59:59+14:00')).toBe('2026.07.14')
    expect(formatDottedIsoDate('2026-07-14 10:00:00')).toBe('2026.07.14')
    expect(formatDottedIsoDate('2026-07-14')).toBe('2026.07.14')
  })

  it('falls back for missing or malformed input', () => {
    expect(formatDottedIsoDate(undefined)).toBe('-')
    expect(formatDottedIsoDate('   ')).toBe('-')
    expect(formatDottedIsoDate('not-a-date')).toBe('not.a.date')
  })
})
