export const SUPPORTED_LOCALES = ['en', 'zh-CN', 'es', 'ko'] as const
export type SupportedLocale = (typeof SUPPORTED_LOCALES)[number]
export const USER_LOCALE_STORAGE_KEY = 'ideaboard.user-locale'

export type LocaleBundle = Record<string, unknown>
export type LocaleLoader = () => Promise<{ default?: LocaleBundle } | LocaleBundle>
