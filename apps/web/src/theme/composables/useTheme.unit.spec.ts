import { describe, expect, it } from 'vitest'
import { nextTick } from 'vue'
import { isDarkModeActive, useTheme } from './useTheme'

describe('useTheme', () => {
  it('defaults to dark without adding a theme class', () => {
    const { mode, isDark } = useTheme()
    expect(mode.value).toBe('dark')
    expect(isDark.value).toBe(true)
    expect(document.documentElement.classList.contains('dark')).toBe(false)
    expect(document.documentElement.classList.contains('light')).toBe(false)
  })

  it('applies setTheme transitions across dark, light, and system', async () => {
    const { mode, isDark, setTheme } = useTheme()

    setTheme('dark')
    expect(mode.value).toBe('dark')
    expect(isDark.value).toBe(true)
    expect(isDarkModeActive()).toBe(true)

    setTheme('light')
    await nextTick()
    expect(mode.value).toBe('light')
    expect(isDark.value).toBe(false)
    expect(isDarkModeActive()).toBe(false)
    expect(document.documentElement.classList.contains('light')).toBe(true)

    setTheme('dark')
    await nextTick()
    expect(document.documentElement.classList.contains('light')).toBe(false)
    expect(document.documentElement.classList.contains('dark')).toBe(false)

    setTheme('system')
    await nextTick()
    expect(mode.value).toBe('auto')
    const isSystemDark = window.matchMedia('(prefers-color-scheme: dark)').matches
    expect(isDark.value).toBe(isSystemDark)
    expect(document.documentElement.classList.contains('light')).toBe(!isSystemDark)
  })
})
