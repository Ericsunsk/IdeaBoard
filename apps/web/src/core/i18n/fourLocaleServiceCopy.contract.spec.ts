import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { formatLocaleNumber, toIntlLocale } from './formatters'
import { LOCALE_MANIFEST } from './localeManifest'
import { i18n } from './index'
import {
  getActiveUserLocale,
  setUserLocale,
  SUPPORTED_LOCALES,
  USER_LOCALE_STORAGE_KEY,
  type SupportedLocale,
} from './userLocale'
import { mockAppToast } from '../../test/toast'

vi.mock('../../core/composables/useAppToast')

const ALL_LOCALES: SupportedLocale[] = ['en', 'zh-CN', 'es', 'ko']

const SERVICE_LABELS: Record<SupportedLocale, string> = {
  en: 'You need more Ideas to run this action.',
  'zh-CN': '需要更多 Ideas 才能执行此操作。',
  es: 'Necesita más Ideas para ejecutar esta acción.',
  ko: '이 작업을 실행하려면 더 많은 Ideas가 필요합니다.',
}

describe('four-locale service copy (i18n/four-locale-service-copy)', () => {
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

  it('covers exactly en, zh-CN, es, and ko', () => {
    expect(SUPPORTED_LOCALES).toEqual(['en', 'zh-CN', 'es', 'ko'])
  })

  it('switches every locale at call time with locale, lang, and storage in sync', async () => {
    for (const locale of ALL_LOCALES) {
      await expect(setUserLocale(locale)).resolves.toBe(true)
      expect(getActiveUserLocale()).toBe(locale)
      expect(document.documentElement.lang).toBe(locale)
      expect(localStorage.getItem(USER_LOCALE_STORAGE_KEY)).toBe(locale)
    }
  })

  it('renders every relocated billing service label in the active locale', async () => {
    const rendered: string[] = []
    for (const locale of ALL_LOCALES) {
      await expect(setUserLocale(locale)).resolves.toBe(true)
      // Canonical source: `errorCodes.insufficient_credits` (backend code unique source).
      const label = String(i18n.global.t('errorCodes.insufficient_credits'))
      expect(label).toBe(SERVICE_LABELS[locale])
      rendered.push(label)
    }
    // Every switch must actually change the visible copy; an English fallback
    // for a missing locale bundle would collapse these to one string.
    expect(new Set(rendered).size).toBe(ALL_LOCALES.length)
  })

  it('interpolates service-copy placeholders in the active locale', async () => {
    await expect(setUserLocale('zh-CN')).resolves.toBe(true)
    expect(
      String(
        i18n.global.t('billing.insufficientBalanceDetail', { required: '10', available: '3' }),
      ),
    ).toBe('Ideas 不足（需要 10，当前拥有 3）。')

    await expect(setUserLocale('es')).resolves.toBe(true)
    expect(
      String(
        i18n.global.t('billing.insufficientBalanceDetail', { required: '10', available: '3' }),
      ),
    ).toBe('Ideas insuficientes (necesita 10, tiene 3).')
  })

  it('formats service numbers per locale at call time without touching the i18n runtime', () => {
    // No setUserLocale/loadLocaleMessages call here: the explicit-locale path
    // rides Intl only, so the active locale must stay exactly as it was.
    expect(formatLocaleNumber(1280.5, undefined, 'en')).toBe('1,280.5')
    expect(formatLocaleNumber(1280.5, undefined, 'zh-CN')).toBe('1,280.5')
    expect(formatLocaleNumber(1280.5, undefined, 'es')).toBe('1280,5')
    expect(formatLocaleNumber(1280.5, undefined, 'ko')).toBe('1,280.5')
    expect(formatLocaleNumber(1280000, undefined, 'en')).toBe('1,280,000')
    expect(formatLocaleNumber(1280000, undefined, 'es')).toBe('1.280.000')
    expect((i18n.global.locale as { value: string }).value).toBe('en')
  })

  it('maps every supported locale to a stable Intl tag and display label', () => {
    expect(toIntlLocale('en')).toBe('en-US')
    expect(toIntlLocale('zh-CN')).toBe('zh-CN')
    expect(toIntlLocale('es')).toBe('es-ES')
    expect(toIntlLocale('ko')).toBe('ko-KR')

    const labels = ALL_LOCALES.map((locale) => LOCALE_MANIFEST[locale].displayLabel)
    expect(labels).toEqual(['English', '简体中文', 'Español', '한국어'])
  })
})
