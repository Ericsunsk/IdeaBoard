import { describe, expect, it, test } from 'vitest'
import {
  countLeavesByNamespace,
  diffLocaleBundles,
  leafPaths,
  localeStructureWarnings,
  maxLeafSegments,
} from './localeParity'
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
      const diff = diffLocaleBundles(
        enBundle as Record<string, unknown>,
        BUNDLES[locale] as Record<string, unknown>,
      )
      expect(diff.equal, `[${locale}] should match en exactly`).toBe(true)
      expect(diff.missing).toEqual([])
      expect(diff.extra).toEqual([])
      expect(diff.shapeMismatches).toEqual([])
    }
  })
})

describe('Locale bundle structure limits (P0 warn-only, enforced by lint:locale-parity)', () => {
  test('measures nesting depth as leaf-path segments', () => {
    expect(maxLeafSegments({})).toBe(0)
    expect(maxLeafSegments({ canvas: { retouch: { brush: { size: 'Size' } } } })).toBe(4)
  })

  test('counts leaves per top-level namespace', () => {
    expect(countLeavesByNamespace({ a: { x: 'x', y: 'y' }, b: { z: 'z' } })).toEqual({
      a: 2,
      b: 1,
    })
  })

  test('does not warn at the nesting and namespace limits', () => {
    const bundle = {
      canvas: { retouch: { brush: { size: 'Size' } } },
      library: Object.fromEntries(
        Array.from({ length: 300 }, (_, index) => [`key${index}`, 'value']),
      ),
    }
    expect(localeStructureWarnings(bundle)).toEqual([])
  })

  test('warns for excess nesting without changing parity', () => {
    const bundle = { canvas: { retouch: { brush: { size: { label: 'Size' } } } } }
    expect(localeStructureWarnings(bundle)).toEqual([
      '[locale-structure:warn] en nesting depth 5 exceeds 4 path segments; split deep trees into a new namespace.',
    ])
    expect(diffLocaleBundles(bundle, bundle).equal).toBe(true)
  })

  test('warns for an oversized namespace without changing parity', () => {
    const bundle = {
      canvas: Object.fromEntries(
        Array.from({ length: 301 }, (_, index) => [`key${index}`, 'value']),
      ),
    }
    expect(localeStructureWarnings(bundle)).toEqual([
      '[locale-structure:warn] en.canvas holds 301 leaves (limit 300); split the god-key namespace.',
    ])
    expect(diffLocaleBundles(bundle, bundle).equal).toBe(true)
  })
})
