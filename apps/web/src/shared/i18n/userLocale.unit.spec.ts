import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  getActiveUserLocale,
  initUserLocale,
  isValidSupportedLocale,
  loadLocaleMessages,
  resolveInitialUserLocale,
  setUserLocale,
  SUPPORTED_LOCALES,
  type SupportedLocale,
  USER_LOCALE_STORAGE_KEY,
} from './userLocale'
import { englishMessages, i18n } from './index'
import { LOCALE_MANIFEST } from './localeManifest'
import { mockAppToast, type AppToastMocks } from '../../test/toast'

let toast: AppToastMocks
vi.mock('../composables/useAppToast')

describe('User Locale Infrastructure (Milestone 1)', () => {
  beforeEach(() => {
    localStorage.clear()
    toast = mockAppToast()
    ;(i18n.global.locale as { value: string }).value = 'en'
    document.documentElement.lang = 'en'
  })

  afterEach(() => {
    localStorage.clear()
    vi.restoreAllMocks()
  })

  describe('Supported Locales & Validation', () => {
    it('defines exactly en, zh-CN, es, ko', () => {
      expect(SUPPORTED_LOCALES).toEqual(['en', 'zh-CN', 'es', 'ko'])
    })

    it('validates supported locales correctly', () => {
      expect(isValidSupportedLocale('en')).toBe(true)
      expect(isValidSupportedLocale('zh-CN')).toBe(true)
      expect(isValidSupportedLocale('es')).toBe(true)
      expect(isValidSupportedLocale('ko')).toBe(true)
      expect(isValidSupportedLocale('fr')).toBe(false)
      expect(isValidSupportedLocale('zh-TW')).toBe(false)
      expect(isValidSupportedLocale(null)).toBe(false)
      expect(isValidSupportedLocale(undefined)).toBe(false)
      expect(isValidSupportedLocale(123)).toBe(false)
    })
  })

  describe('resolveInitialUserLocale', () => {
    describe('Storage precedence', () => {
      it('returns valid storage value over browser languages', () => {
        expect(resolveInitialUserLocale('zh-CN', ['en-US'])).toBe('zh-CN')
        expect(resolveInitialUserLocale('es', ['zh-CN'])).toBe('es')
        expect(resolveInitialUserLocale('ko', ['es-ES'])).toBe('ko')
        expect(resolveInitialUserLocale('en', ['ko-KR'])).toBe('en')
      })

      it('reads from localStorage when storageValue is omitted', () => {
        localStorage.setItem(USER_LOCALE_STORAGE_KEY, 'es')
        expect(resolveInitialUserLocale(undefined, ['zh-CN'])).toBe('es')
      })

      it('ignores invalid storage values and falls through to navigatorLanguages', () => {
        expect(resolveInitialUserLocale('invalid_locale', ['es-MX'])).toBe('es')
        expect(resolveInitialUserLocale('fr', ['ko-KR'])).toBe('ko')
        expect(resolveInitialUserLocale('', ['zh-CN'])).toBe('zh-CN')
      })
    })

    describe('Simplified Chinese resolution', () => {
      it.each([
        [['zh-CN'], 'zh-CN'],
        [['zh-SG'], 'zh-CN'],
        [['zh-Hans'], 'zh-CN'],
        [['zh-Hans-CN'], 'zh-CN'],
        [['zh-Hans-SG'], 'zh-CN'],
        [['ZH-CN'], 'zh-CN'],
      ])('maps %j to %s', (tags, expected) => {
        expect(resolveInitialUserLocale(null, tags)).toBe(expected)
      })
    })

    describe('Traditional Chinese fallback to English', () => {
      it.each([
        [['zh'], 'en'],
        [['zh-TW'], 'en'],
        [['zh-HK'], 'en'],
        [['zh-MO'], 'en'],
        [['zh-Hant'], 'en'],
        [['zh-Hant-TW'], 'en'],
        [['zh-TW', 'zh-HK', 'en-US'], 'en'],
        [['zh-HK', 'es-ES'], 'en'],
      ])('falls back Traditional Chinese tags %j to %s', (tags, expected) => {
        expect(resolveInitialUserLocale(null, tags)).toBe(expected)
      })
    })

    describe('Spanish dialect resolution', () => {
      it.each([
        [['es'], 'es'],
        [['es-ES'], 'es'],
        [['es-MX'], 'es'],
        [['es-419'], 'es'],
        [['es-US'], 'es'],
        [['ES-AR'], 'es'],
      ])('maps Spanish dialect %j to %s', (tags, expected) => {
        expect(resolveInitialUserLocale(null, tags)).toBe(expected)
      })
    })

    describe('Korean dialect resolution', () => {
      it.each([
        [['ko'], 'ko'],
        [['ko-KR'], 'ko'],
        [['ko-KP'], 'ko'],
        [['KO'], 'ko'],
      ])('maps Korean dialect %j to %s', (tags, expected) => {
        expect(resolveInitialUserLocale(null, tags)).toBe(expected)
      })
    })

    describe('English dialect & fallback resolution', () => {
      it.each([
        [['en'], 'en'],
        [['en-US'], 'en'],
        [['en-GB'], 'en'],
        [['fr-FR', 'de-DE'], 'en'],
        [['ja-JP'], 'en'],
        [[], 'en'],
      ])('maps tags %j to %s', (tags, expected) => {
        expect(resolveInitialUserLocale(null, tags)).toBe(expected)
      })

      it('evaluates language preference list in order', () => {
        expect(resolveInitialUserLocale(null, ['fr-FR', 'de-DE', 'es-ES'])).toBe('es')
        expect(resolveInitialUserLocale(null, ['ja-JP', 'ko-KR', 'en-US'])).toBe('ko')
        expect(resolveInitialUserLocale(null, ['fr-FR', 'zh-CN', 'es-ES'])).toBe('zh-CN')
      })
    })
  })

  describe('loadLocaleMessages & caching', () => {
    it('returns synchronous English bundle immediately without dynamic load', async () => {
      const messages = await loadLocaleMessages('en')
      expect(messages).toEqual(englishMessages)
    })

    it('loads non-English bundle dynamically and caches it', async () => {
      const mockBundle = { common: { actions: { cancel: '取消' } } }
      const mockLoader = vi.fn().mockResolvedValue({ default: mockBundle })
      vi.spyOn(LOCALE_MANIFEST['zh-CN'], 'messagesLoader').mockImplementation(mockLoader)

      const firstCall = await loadLocaleMessages('zh-CN')
      expect(mockLoader).toHaveBeenCalledTimes(1)
      expect(firstCall).toEqual(mockBundle)

      const secondCall = await loadLocaleMessages('zh-CN')
      expect(mockLoader).toHaveBeenCalledTimes(1)
      expect(secondCall).toEqual(mockBundle)
    })

    it('shares in-flight load promise when requested concurrently', async () => {
      const mockBundle = { common: { actions: { cancel: 'Cancelar' } } }
      let resolvePromise!: (val: { default: typeof mockBundle }) => void
      const mockLoader = vi.fn().mockImplementation(
        () =>
          new Promise((resolve) => {
            resolvePromise = resolve
          }),
      )
      vi.spyOn(LOCALE_MANIFEST.es, 'messagesLoader').mockImplementation(mockLoader)

      const p1 = loadLocaleMessages('es')
      const p2 = loadLocaleMessages('es')

      expect(mockLoader).toHaveBeenCalledTimes(1)
      resolvePromise({ default: mockBundle })

      const [res1, res2] = await Promise.all([p1, p2])
      expect(res1).toEqual(mockBundle)
      expect(res2).toEqual(mockBundle)
    })

    it('throws when loader fails', async () => {
      const mockLoader = vi.fn().mockRejectedValue(new Error('Network error loading bundle'))
      vi.spyOn(LOCALE_MANIFEST.ko, 'messagesLoader').mockImplementation(mockLoader)

      await expect(loadLocaleMessages('ko')).rejects.toThrow('Network error loading bundle')
    })
  })

  describe('setUserLocale', () => {
    it('atomically switches to English and updates i18n, DOM, and storage', async () => {
      ;(i18n.global.locale as { value: string }).value = 'zh-CN'

      const success = await setUserLocale('en')
      expect(success).toBe(true)
      expect((i18n.global.locale as { value: string }).value).toBe('en')
      expect(document.documentElement.lang).toBe('en')
      expect(localStorage.getItem(USER_LOCALE_STORAGE_KEY)).toBe('en')
    })

    it('atomically switches to a dynamic locale and updates state', async () => {
      const success = await setUserLocale('es')

      expect(success).toBe(true)
      expect(i18n.global.locale.value).toBe('es')
      expect(document.documentElement.lang).toBe('es')
      expect(localStorage.getItem(USER_LOCALE_STORAGE_KEY)).toBe('es')
    })

    it('on load failure: retains current locale, surfaces toast error, does NOT mutate storage', async () => {
      vi.spyOn(LOCALE_MANIFEST.ko, 'messagesLoader').mockRejectedValue(
        new Error('Failed to fetch ko chunk'),
      )

      localStorage.setItem(USER_LOCALE_STORAGE_KEY, 'en')
      i18n.global.locale.value = 'en'
      document.documentElement.lang = 'en'

      const success = await setUserLocale('ko')
      expect(success).toBe(false)
      expect(i18n.global.locale.value).toBe('en')
      expect(document.documentElement.lang).toBe('en')
      expect(localStorage.getItem(USER_LOCALE_STORAGE_KEY)).toBe('en')
      expect(toast.error).toHaveBeenCalledWith('The request could not be completed.')
    })

    it('returns false and ignores unsupported locale', async () => {
      const success = await setUserLocale('fr' as unknown as SupportedLocale)
      expect(success).toBe(false)
      expect(i18n.global.locale.value).toBe('en')
    })
  })

  describe('initUserLocale', () => {
    it('initializes stored dynamic locale before mount', async () => {
      localStorage.setItem(USER_LOCALE_STORAGE_KEY, 'zh-CN')
      const resolved = await initUserLocale()
      expect(resolved).toBe('zh-CN')
      expect(i18n.global.locale.value).toBe('zh-CN')
      expect(document.documentElement.lang).toBe('zh-CN')
      expect(getActiveUserLocale()).toBe('zh-CN')
    })

    it('gracefully falls back to English when stored dynamic locale fails to load during bootstrap', async () => {
      localStorage.setItem(USER_LOCALE_STORAGE_KEY, 'ko')
      vi.spyOn(LOCALE_MANIFEST.ko, 'messagesLoader').mockRejectedValue(new Error('Bundle offline'))

      const resolved = await initUserLocale()
      expect(resolved).toBe('en')
      expect(i18n.global.locale.value).toBe('en')
      expect(document.documentElement.lang).toBe('en')
      // Must not overwrite storage with 'en' or corrupt failed locale
      expect(localStorage.getItem(USER_LOCALE_STORAGE_KEY)).toBe('ko')
    })
  })
})
