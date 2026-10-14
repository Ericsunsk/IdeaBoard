import { describe, expect, it } from 'vitest'
import { resolveMediaDisplayTier } from './displayTier'

describe('resolveMediaDisplayTier', () => {
  it.each([
    [128, 1, 'thumb'],
    [256, 1, 'thumb'],
    [257, 1, 'preview'],
    [320, 1, 'preview'],
    [640, 1, 'preview'],
    [641, 1, 'large-preview'],
    [1280, 1, 'large-preview'],
    [1281, 1, 'original'],
    [4096, 1, 'original'],
    // display width × dpr selects the tier, not CSS width alone.
    [128, 2, 'thumb'],
    [129, 2, 'preview'],
    [200, 2, 'preview'],
    [321, 2, 'large-preview'],
    [640, 2, 'large-preview'],
    [641, 2, 'original'],
    [280, 2, 'preview'],
    [480, 3, 'original'],
  ] as Array<[number, number, string]>)(
    'maps %ipx CSS at %idpr to %s',
    (displayWidth, dpr, tier) => {
      expect(resolveMediaDisplayTier(displayWidth * dpr)).toBe(tier)
    },
  )

  it('falls back to large-preview for unknown widths', () => {
    for (const value of [undefined, null, Number.NaN, 0, -40, 'wide']) {
      expect(resolveMediaDisplayTier(value)).toBe('large-preview')
    }
  })
})
