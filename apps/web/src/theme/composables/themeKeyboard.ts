import type { Ref } from 'vue'
import type { StoredThemeMode, ThemeMode } from './themeMode'

export const THEME_SEGMENT_MODES = ['light', 'dark', 'auto'] as const

type ThemeSegmentMode = (typeof THEME_SEGMENT_MODES)[number]

export function createThemeKeyboardHandler(options: {
  mode: Readonly<Ref<StoredThemeMode>>
  setTheme: (mode: ThemeMode) => void
}) {
  return (event: KeyboardEvent) => {
    if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
    event.preventDefault()

    const current = THEME_SEGMENT_MODES.indexOf(options.mode.value as ThemeSegmentMode)
    const next =
      event.key === 'Home'
        ? 0
        : event.key === 'End'
          ? THEME_SEGMENT_MODES.length - 1
          : (current + (event.key === 'ArrowRight' ? 1 : THEME_SEGMENT_MODES.length - 1)) %
            THEME_SEGMENT_MODES.length

    options.setTheme(THEME_SEGMENT_MODES[next] ?? 'auto')
    requestAnimationFrame(() => {
      const group = event.currentTarget as HTMLElement
      ;(group.querySelectorAll('[role="radio"]')[next] as HTMLElement | undefined)?.focus()
    })
  }
}
