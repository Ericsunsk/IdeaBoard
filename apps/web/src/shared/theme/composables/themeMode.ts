export const THEME_STORAGE_KEY = 'ideaboard-color-mode'

export type ThemeMode = 'light' | 'dark' | 'auto' | 'system'
export type StoredThemeMode = 'light' | 'dark' | 'auto'

/** Normalize the public alias used by the menu to VueUse's persisted values. */
export function normalizeThemeMode(mode: ThemeMode | string | null | undefined): StoredThemeMode {
  if (mode === 'dark') return 'dark'
  if (mode === 'light') return 'light'
  return 'auto'
}

export function isDarkThemeMode(mode: StoredThemeMode, prefersDark: boolean): boolean {
  if (mode === 'dark') return true
  if (mode === 'light') return false
  return prefersDark
}
