import { describe, expect, it } from 'vitest'
import { isDarkModeActive, useTheme } from './useTheme'

describe('useTheme', () => {
  it('applies setTheme transitions across dark, light, and system', () => {
    const { mode, isDark, setTheme } = useTheme()

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
