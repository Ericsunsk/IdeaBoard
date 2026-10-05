import { readLocalStorageJson, writeLocalStorageJson } from '../../../../shared/utils/storage'

const PREVIEW_THEME_STORAGE_KEY = 'ideaboard.boardPreviewThemeMap'

let inMemoryThemeMap: Record<string, 'dark' | 'light'> | null = null

function isPreviewTheme(value: unknown): value is 'dark' | 'light' {
  return value === 'dark' || value === 'light'
}

function parseThemeMap(value: unknown): Record<string, 'dark' | 'light'> | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {}
  return Object.fromEntries(
    Object.entries(value).filter(([, theme]) => isPreviewTheme(theme)),
  ) as Record<string, 'dark' | 'light'>
}

function loadThemeMap(): Record<string, 'dark' | 'light'> {
  if (inMemoryThemeMap) return inMemoryThemeMap
  inMemoryThemeMap = readLocalStorageJson(PREVIEW_THEME_STORAGE_KEY, parseThemeMap) ?? {}
  return inMemoryThemeMap
}

export function getStoredPreviewTheme(boardId: string): 'dark' | 'light' | null {
  const map = loadThemeMap()
  return map[boardId] ?? null
}

export function setStoredPreviewTheme(boardId: string, theme: 'dark' | 'light'): void {
  const map = loadThemeMap()
  map[boardId] = theme
  writeLocalStorageJson(PREVIEW_THEME_STORAGE_KEY, map)
}
