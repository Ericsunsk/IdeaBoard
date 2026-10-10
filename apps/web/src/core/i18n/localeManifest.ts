import { englishMessages } from './i18nRuntime'
import type { LocaleBundle, LocaleLoader, SupportedLocale } from './localeCore'

/**
 * Single source of truth mapping each SupportedLocale to its runtime artifacts.
 * Adding a locale touches only this registry — every consumer below derives from here.
 */
export interface LocaleArtifact {
  /** Lazy message-bundle loader; 'en' is bundled synchronously. */
  messagesLoader: LocaleLoader
  /** BCP-47 tag for Intl formatters (dates/numbers). */
  intlTag: string
  /** Native display label for the locale switcher. */
  displayLabel: string
}

export const LOCALE_MANIFEST: Record<SupportedLocale, LocaleArtifact> = {
  en: {
    messagesLoader: () => Promise.resolve(englishMessages as unknown as LocaleBundle),
    intlTag: 'en-US',
    displayLabel: 'English',
  },
  'zh-CN': {
    messagesLoader: () => import('./locales/zh-CN/index.ts'),
    intlTag: 'zh-CN',
    displayLabel: '简体中文',
  },
  es: {
    messagesLoader: () => import('./locales/es/index.ts'),
    intlTag: 'es-ES',
    displayLabel: 'Español',
  },
  ko: {
    messagesLoader: () => import('./locales/ko/index.ts'),
    intlTag: 'ko-KR',
    displayLabel: '한국어',
  },
}
