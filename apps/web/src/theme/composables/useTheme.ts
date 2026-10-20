import { useColorMode } from '@vueuse/core'
import { computed, ref, type ComputedRef } from 'vue'
import {
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
    const isDarkApplied = ref(true)
    const mode = useColorMode<StoredThemeMode>({
      selector: 'html',
      attribute: 'class',
      modes: {
        dark: '',
        light: 'light',
      },
      storageKey: THEME_STORAGE_KEY,
      initialValue: 'dark',
      emitAuto: true,
      onChanged(resolvedMode, apply) {
        apply(resolvedMode)
        // Canvas renderers must read tokens after the root palette changes.
        isDarkApplied.value = resolvedMode === 'dark'
      },
    })
    mode.value = normalizeThemeMode(mode.value)
    sharedMode = mode
    sharedIsDark = computed(() => isDarkApplied.value)
  }
  return { mode: sharedMode, isDark: sharedIsDark }
}

export function isDarkModeActive(): boolean {
  const { isDark } = getThemeState()
  return isDark.value
}

export function useTheme() {
  const { mode, isDark } = getThemeState()

  function setTheme(newMode: ThemeMode) {
    mode.value = normalizeThemeMode(newMode)
  }

  return {
    mode,
    isDark,
    setTheme,
  }
}
