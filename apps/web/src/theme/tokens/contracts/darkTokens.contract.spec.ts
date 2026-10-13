// @vitest-environment node
import { describe, expect, it } from 'vitest'
import {
  contrastOf,
  parseOwnerDecls,
  resolveAlphaColor,
  scopeValues,
} from '../../../../scripts/checkTokenGraph'

type Rgb = readonly [number, number, number]

/** Read authored dark values with base-layer declarations taking precedence. */
const tokenValues = scopeValues(parseOwnerDecls(), 'dark')

function tokenValue(name: string): string {
  const value = tokenValues.get(name)?.value
  if (value === undefined) throw new Error(`Token ${name} is not assigned in dark mode`)
  return value
}

function readDarkToken(name: string): string {
  const color = resolveAlphaColor(tokenValue(name), tokenValue)
  if (color.alpha !== 1) throw new Error(`Expected opaque token ${name}`)
  return `#${color.rgb.map((channel) => Math.round(channel).toString(16).padStart(2, '0')).join('')}`
}

const parseHex = (value: string): Rgb => {
  const color = resolveAlphaColor(value, tokenValue)
  if (color.alpha !== 1) throw new Error(`Expected an opaque color, received "${value}"`)
  return color.rgb
}

const channel = (raw: number): number => {
  const c = raw / 255
  return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
}

const relativeLuminance = ([r, g, b]: Rgb): number =>
  0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b)

const contrastRatio = (foreground: string, background: string): number =>
  contrastOf(parseHex(foreground), parseHex(background))

describe('Dark Tokens Contrast & Hierarchy Contract', () => {
  const surfaces = [
    { id: 'page-bg', value: readDarkToken('--color-admin-bg') },
    { id: 'card-strong', value: readDarkToken('--color-admin-card-strong') },
    { id: 'surface-muted', value: readDarkToken('--color-admin-surface-muted') },
    { id: 'surface-hover', value: readDarkToken('--color-admin-surface-hover') },
  ]

  const text1 = readDarkToken('--color-admin-text-1')
  const textLabel = readDarkToken('--color-admin-text-label')
  const text2 = readDarkToken('--color-admin-text-2')
  const text3 = readDarkToken('--color-admin-text-3')

  it('declares monotonic luminance hierarchy for text roles', () => {
    const lum1 = relativeLuminance(parseHex(text1))
    const lumLabel = relativeLuminance(parseHex(textLabel))
    const lum2 = relativeLuminance(parseHex(text2))
    const lum3 = relativeLuminance(parseHex(text3))

    expect(lum1).toBeGreaterThan(lumLabel)
    expect(lumLabel).toBeGreaterThan(lum2)
    expect(lum2).toBeGreaterThan(lum3)
  })

  it('guarantees text-1 achieves WCAG AAA (>= 7.0:1) on all dark surfaces', () => {
    for (const surface of surfaces) {
      const ratio = contrastRatio(text1, surface.value)
      expect(ratio, `text-1 on ${surface.id}`).toBeGreaterThanOrEqual(7.0)
    }
  })

  it('guarantees text-label and text-2 achieve WCAG AA (>= 4.5:1) on all dark surfaces', () => {
    for (const surface of surfaces) {
      const labelRatio = contrastRatio(textLabel, surface.value)
      expect(labelRatio, `text-label on ${surface.id}`).toBeGreaterThanOrEqual(4.5)

      const text2Ratio = contrastRatio(text2, surface.value)
      expect(text2Ratio, `text-2 on ${surface.id}`).toBeGreaterThanOrEqual(4.5)
    }
  })
})
