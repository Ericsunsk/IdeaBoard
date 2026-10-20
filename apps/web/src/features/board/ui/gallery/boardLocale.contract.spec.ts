import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { i18n, setUserLocale, type SupportedLocale } from '../../../../core/i18n'
import { mockAppToast } from '../../../../test/toast'

vi.mock('../../../../core/composables/useAppToast')

const ALL_LOCALES: SupportedLocale[] = ['en', 'zh-CN', 'es', 'ko']

// Every leaf under the gallery boards namespace (no plural forms exist here).
const BOARD_KEYS = [
  'title',
  'subtitle',
  'newCanvas',
  'searchPlaceholder',
  'searchLabel',
  'sortLabel',
  'sortRecent',
  'sortName',
  'createNewCanvas',
  'startFromEmpty',
  'noMatchingCanvas',
  'untitledCanvas',
  'moreActions',
  'duplicate',
  'edited',
  'scopePersonal',
  'scopeTeam',
  'accessPrivate',
  'accessWorkspace',
  'share',
] as const

const TITLES: Record<SupportedLocale, string> = {
  en: 'Welcome to IdeaBoard!',
  'zh-CN': '欢迎来到 IdeaBoard！',
  es: '¡Te damos la bienvenida a IdeaBoard!',
  ko: 'IdeaBoard에 오신 것을 환영합니다!',
}

function translate(key: string, named?: Record<string, string>): string {
  return String(named ? i18n.global.t(key, named) : i18n.global.t(key))
}

describe('board locale contract', () => {
  beforeEach(() => {
    localStorage.clear()
    mockAppToast()
  })

  afterEach(() => {
    localStorage.clear()
    vi.restoreAllMocks()
    ;(i18n.global.locale as { value: string }).value = 'en'
    document.documentElement.lang = 'en'
  })

  it('provides every gallery key in all four locales', async () => {
    for (const locale of ALL_LOCALES) {
      await expect(setUserLocale(locale)).resolves.toBe(true)
      for (const key of BOARD_KEYS) {
        const current = translate(`board.${key}`, { date: 'today' })
        // Runtime missing-key assertion: the key resolves to copy, never
        // the raw key path (which vue-i18n returns for missing keys).
        expect(current).not.toBe(`board.${key}`)
      }
      expect(translate('board.title')).toBe(TITLES[locale])
    }
  })

  it('keeps four distinct title copies (no English-fallback collapse)', async () => {
    const rendered: string[] = []
    for (const locale of ALL_LOCALES) {
      await expect(setUserLocale(locale)).resolves.toBe(true)
      rendered.push(translate('board.title'))
    }
    expect(new Set(rendered).size).toBe(ALL_LOCALES.length)
  })

  it('interpolates the last-edited date', async () => {
    await setUserLocale('en')
    expect(translate('board.edited', { date: '2026-10-11' })).toBe('Edited 2026-10-11')
  })

  it('falls back to English with a complete board bundle', () => {
    const fallback = i18n.global.fallbackLocale as unknown
    const resolved =
      typeof fallback === 'string' ? fallback : (fallback as { value?: unknown })?.value
    expect(resolved).toBe('en')
    for (const key of BOARD_KEYS) {
      expect(i18n.global.te(`board.${key}`, 'en')).toBe(true)
    }
  })
})
