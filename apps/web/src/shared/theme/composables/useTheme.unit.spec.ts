import { describe, expect, it } from 'vitest'
import { isDarkModeActive, useTheme } from './useTheme'

describe('useTheme', () => {
  it('initializes with a valid mode and provides setTheme method', () => {
    const { mode, isDark, setTheme } = useTheme()

    expect(['light', 'dark', 'auto']).toContain(mode.value)
    expect(typeof isDark.value).toBe('boolean')
    expect(typeof setTheme).toBe('function')

    setTheme('dark')
    expect(mode.value).toBe('dark')
    expect(isDark.value).toBe(true)
    expect(isDarkModeActive()).toBe(true)

    setTheme('light')
    expect(mode.value).toBe('light')
    expect(isDark.value).toBe(false)
    expect(isDarkModeActive()).toBe(false)

    setTheme('system')
    expect(mode.value).toBe('auto')
  })
})
