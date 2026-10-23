import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { diffLocaleBundles, leafPaths } from './localeParity'
import { i18n } from './index'
import { getActiveUserLocale, setUserLocale, type SupportedLocale } from './userLocale'
import { mockAppToast } from '../../test/toast'
import enBundle from './locales/en/index'
import zhCNBundle from './locales/zh-CN/index'
import esBundle from './locales/es/index'
import koBundle from './locales/ko/index'

vi.mock('../../core/composables/useAppToast')

const ALL_LOCALES: SupportedLocale[] = ['en', 'zh-CN', 'es', 'ko']

const BUNDLES = {
  en: enBundle,
  'zh-CN': zhCNBundle,
  es: esBundle,
  ko: koBundle,
} as const

const INSUFFICIENT_CREDITS_COPY: Record<SupportedLocale, string> = {
  en: 'You need more Ideas to run this action.',
  'zh-CN': '需要更多 Ideas 才能执行此操作。',
  es: 'Necesita más Ideas para ejecutar esta acción.',
  ko: '이 작업을 실행하려면 더 많은 Ideas가 필요합니다.',
}

describe('errorCodes four-locale parity (backend code unique source)', () => {
  beforeEach(() => {
    localStorage.clear()
    mockAppToast()
    ;(i18n.global.locale as { value: string }).value = 'en'
    document.documentElement.lang = 'en'
  })

  afterEach(() => {
    localStorage.clear()
    vi.restoreAllMocks()
    ;(i18n.global.locale as { value: string }).value = 'en'
    document.documentElement.lang = 'en'
  })

  it('keeps every errorCodes key structurally identical across locales', () => {
    const enKeys = leafPaths(enBundle.errorCodes).sort()
    expect(enKeys.length).toBeGreaterThan(0)
    expect(enKeys).toContain('errorCodes.insufficient_credits'.split('.').slice(1).join('.'))

    for (const locale of ALL_LOCALES) {
      if (locale === 'en') continue
      const diff = diffLocaleBundles(
        enBundle.errorCodes as Record<string, unknown>,
        BUNDLES[locale].errorCodes as Record<string, unknown>,
      )
      expect(diff.equal, `[${locale}] errorCodes should match en exactly`).toBe(true)
      expect(diff.missing).toEqual([])
      expect(diff.extra).toEqual([])
      expect(diff.shapeMismatches).toEqual([])
    }
  })

  it('renders insufficient_credits in the active locale without English fallback', async () => {
    const rendered: string[] = []
    for (const locale of ALL_LOCALES) {
      await expect(setUserLocale(locale)).resolves.toBe(true)
      expect(getActiveUserLocale()).toBe(locale)
      const label = String(i18n.global.t('errorCodes.insufficient_credits'))
      expect(label).toBe(INSUFFICIENT_CREDITS_COPY[locale])
      rendered.push(label)
    }
    expect(new Set(rendered).size).toBe(ALL_LOCALES.length)
  })
})
