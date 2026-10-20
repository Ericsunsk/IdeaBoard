export const THEME_STORAGE_KEY = 'ideaboard-color-mode'

export type ThemeMode = 'light' | 'dark' | 'auto' | 'system'
export type StoredThemeMode = 'light' | 'dark' | 'auto'

/** Normalize the public alias used by the menu to VueUse's persisted values. */
export function normalizeThemeMode(mode: ThemeMode | string | null | undefined): StoredThemeMode {
  if (mode === 'light') return 'light'
  if (mode === 'auto' || mode === 'system') return 'auto'
  return 'dark'
}
