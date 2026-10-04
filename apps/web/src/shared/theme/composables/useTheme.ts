import { useColorMode, usePreferredDark } from '@vueuse/core'
import { computed, type ComputedRef } from 'vue'

export type ThemeMode = 'light' | 'dark' | 'auto' | 'system'

export const THEME_STORAGE_KEY = 'ideaboard-color-mode'

type StoredColorMode = 'light' | 'dark' | 'auto'
type ColorModeReturn = ReturnType<typeof useColorMode<StoredColorMode>>
let sharedMode: ColorModeReturn | null = null
let sharedIsDark: ComputedRef<boolean> | null = null

function getThemeState(): { mode: ColorModeReturn; isDark: ComputedRef<boolean> } {
  if (!sharedMode || !sharedIsDark) {
    const preferredDark = usePreferredDark()
    const mode = useColorMode<StoredColorMode>({
      selector: 'html',
      attribute: 'class',
      modes: {
        ['dark']: 'dark',
        light: '',
      },
      storageKey: THEME_STORAGE_KEY,
      emitAuto: true,
    })
    sharedMode = mode
    sharedIsDark = computed(() => {
      if (mode.value === 'dark') return true
      if (mode.value === 'light') return false
      return preferredDark.value
    })
  }
  return { mode: sharedMode, isDark: sharedIsDark }
}

export function isDarkModeActive(): boolean {
  if (typeof document !== 'undefined' && document.documentElement.classList.contains('dark')) {
    return true
  }
  const { isDark } = getThemeState()
  return isDark.value
}

export function useTheme() {
  const { mode, isDark } = getThemeState()

  function setTheme(newMode: ThemeMode) {
    if (newMode === 'system') {
      mode.value = 'auto'
    } else {
      mode.value = newMode
    }
  }

  return {
    mode,
    isDark,
    setTheme,
  }
}

