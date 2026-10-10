import { describe, expect, it } from 'vitest'
import { isDarkThemeMode, normalizeThemeMode } from './themeMode'

describe('theme mode semantics', () => {
  it('normalizes the public system alias and unknown persisted values to auto', () => {
    expect(normalizeThemeMode('system')).toBe('auto')
    expect(normalizeThemeMode('unexpected')).toBe('auto')
    expect(normalizeThemeMode(null)).toBe('auto')
  })

  it('resolves dark mode from explicit mode or system preference', () => {
    expect(isDarkThemeMode('dark', false)).toBe(true)
    expect(isDarkThemeMode('light', true)).toBe(false)
    expect(isDarkThemeMode('auto', true)).toBe(true)
    expect(isDarkThemeMode('auto', false)).toBe(false)
  })
})
