import { useColorMode, usePreferredDark } from '@vueuse/core'
import { computed, type ComputedRef } from 'vue'
import {
  isDarkThemeMode,
  normalizeThemeMode,
  THEME_STORAGE_KEY,
  type StoredThemeMode,
  type ThemeMode,
} from './themeMode'

export { THEME_STORAGE_KEY, type ThemeMode } from './themeMode'

type ColorModeReturn = ReturnType<typeof useColorMode<StoredThemeMode>>
let sharedMode: ColorModeReturn | null = null
let sharedIsDark: ComputedRef<boolean> | null = null

function getThemeState(): { mode: ColorModeReturn; isDark: ComputedRef<boolean> } {
  if (!sharedMode || !sharedIsDark) {
    const preferredDark = usePreferredDark()
    const mode = useColorMode<StoredThemeMode>({
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
      return isDarkThemeMode(mode.value, preferredDark.value)
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
      mode.value = normalizeThemeMode(newMode)
    } else {
      mode.value = normalizeThemeMode(newMode)
    }
  }

  return {
    mode,
    isDark,
    setTheme,
  }
}
