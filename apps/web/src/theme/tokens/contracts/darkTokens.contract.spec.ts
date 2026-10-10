import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

type Rgb = readonly [number, number, number]

const darkSheet = readFileSync(join(import.meta.dirname, '../dark.css'), 'utf8')

function readDarkToken(name: string): string {
  const match = darkSheet.match(new RegExp(`${name}:\\s*([^;]+);`))
  if (!match?.[1]) throw new Error(`Token ${name} not found in tokens/dark.css`)
  const value = match[1].trim()
  if (value.startsWith('#')) return value
  const varMatch = value.match(/var\((--[a-z0-9-]+)\)/)
  if (varMatch?.[1]) {
    return readDarkToken(varMatch[1])
  }
  return value
}

const parseHex = (value: string): Rgb => {
  const hex = value.trim().replace('#', '')
  if (!/^[0-9a-f]{6}$/i.test(hex)) {
    throw new Error(`Expected a 6-digit hex color, received "${value}"`)
  }
  return [
    Number.parseInt(hex.slice(0, 2), 16),
    Number.parseInt(hex.slice(2, 4), 16),
    Number.parseInt(hex.slice(4, 6), 16),
  ] as const
}

const channel = (raw: number): number => {
  const c = raw / 255
  return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
}

const relativeLuminance = ([r, g, b]: Rgb): number =>
  0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b)

const contrastRatio = (foreground: string, background: string): number => {
  const a = relativeLuminance(parseHex(foreground))
  const b = relativeLuminance(parseHex(background))
  const [lighter, darker] = a > b ? [a, b] : [b, a]
  return (lighter + 0.05) / (darker + 0.05)
}

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
