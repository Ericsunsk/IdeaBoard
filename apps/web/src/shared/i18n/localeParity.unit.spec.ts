import { describe, expect, it } from 'vitest'
import { diffLocaleBundles, leafPaths } from './localeParity'
import { SUPPORTED_LOCALES } from './localeCore'
import enBundle from './locales/en/index'
import zhCNBundle from './locales/zh-CN/index'
import esBundle from './locales/es/index'
import koBundle from './locales/ko/index'

const BUNDLES = {
  en: enBundle,
  'zh-CN': zhCNBundle,
  es: esBundle,
  ko: koBundle,
} as const

describe('Locale bundle parity (runtime surface)', () => {
  it('exposes the canonical locale set', () => {
    expect(SUPPORTED_LOCALES).toEqual(['en', 'zh-CN', 'es', 'ko'])
  })

  it('keeps every non-English bundle structurally identical to en', () => {
    const enKeys = leafPaths(enBundle).sort()
    expect(enKeys.length).toBeGreaterThan(0)

    for (const locale of SUPPORTED_LOCALES) {
      if (locale === 'en') continue
      const diff = diffLocaleBundles(enBundle as Record<string, unknown>, BUNDLES[locale] as Record<string, unknown>)
      expect(diff.equal, `[${locale}] should match en exactly`).toBe(true)
      expect(diff.missing).toEqual([])
      expect(diff.extra).toEqual([])
      expect(diff.shapeMismatches).toEqual([])
    }
  })
})
