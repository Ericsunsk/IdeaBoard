import { beforeEach, describe, expect, it, vi } from 'vitest'

const PREVIEW_THEME_STORAGE_KEY = 'ideaboard.boardPreviewThemeMap'

describe('boardPreviewThemeStorage', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.resetModules()
  })

  it('filters malformed persisted theme entries', async () => {
    localStorage.setItem(
      PREVIEW_THEME_STORAGE_KEY,
      JSON.stringify({ boardA: 'sepia', boardB: 'dark', boardC: 1 }),
    )
    const { getStoredPreviewTheme } = await import('./boardPreviewThemeStorage')

    expect(getStoredPreviewTheme('boardA')).toBeNull()
    expect(getStoredPreviewTheme('boardB')).toBe('dark')
    expect(getStoredPreviewTheme('boardC')).toBeNull()
  })

  it('persists valid theme updates through shared storage helpers', async () => {
    const { getStoredPreviewTheme, setStoredPreviewTheme } =
      await import('./boardPreviewThemeStorage')

    setStoredPreviewTheme('boardA', 'light')

    expect(getStoredPreviewTheme('boardA')).toBe('light')
    expect(JSON.parse(localStorage.getItem(PREVIEW_THEME_STORAGE_KEY) ?? '{}')).toEqual({
      boardA: 'light',
    })
  })
})
