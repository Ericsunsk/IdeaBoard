import { describe, expect, it } from 'vitest'
import { resolveChartAnimation } from './chartMotion'

describe('Chart Motion adapter', () => {
  it('maps semantic entry timing to Chart.js animation', () => {
    expect(
      resolveChartAnimation('enter', {
        preference: 'no-preference',
        readCssVariable: (name) => (name === '--motion-enter-duration' ? '220ms' : ''),
      }),
    ).toEqual({ duration: 220 })
  })

  it('disables Chart.js animation under reduced motion', () => {
    expect(
      resolveChartAnimation('enter', {
        preference: 'reduce',
        readCssVariable: () => '220ms',
      }),
    ).toBe(false)
  })
})
