import { describe, expect, it } from 'vitest'
import { normalizeThemeMode } from './themeMode'

describe('theme mode semantics', () => {
  it('normalizes system mode and defaults missing or unknown values to dark', () => {
    expect(normalizeThemeMode('system')).toBe('auto')
    expect(normalizeThemeMode('unexpected')).toBe('dark')
    expect(normalizeThemeMode(null)).toBe('dark')
  })
})
