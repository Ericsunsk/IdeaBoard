import { afterEach, describe, expect, it, vi } from 'vitest'
import { adminChartTheme, defineAdminChartOptions } from './chart'

afterEach(() => {
  document.documentElement.style.removeProperty('--motion-enter-duration')
  document.documentElement.style.removeProperty('--color-admin-chart-axis-line')
  vi.unstubAllGlobals()
})

describe('defineAdminChartOptions', () => {
  it('uses Motion Policy timing for every Admin chart', () => {
    document.documentElement.style.setProperty('--motion-enter-duration', '220ms')
    vi.stubGlobal(
      'matchMedia',
      vi.fn(() => ({ matches: false })),
    )

    expect(defineAdminChartOptions({ animation: { duration: 999 } }).animation).toEqual({
      duration: 220,
    })
  })

  it('keeps Admin chart data immediately readable under reduced motion', () => {
    document.documentElement.style.setProperty('--motion-enter-duration', '220ms')
    vi.stubGlobal(
      'matchMedia',
      vi.fn(() => ({ matches: true })),
    )

    expect(defineAdminChartOptions({}).animation).toBe(false)
  })
})

describe('adminChartTheme', () => {
  it('reads live CSS tokens instead of a separate theme palette', () => {
    document.documentElement.style.setProperty('--color-admin-chart-axis-line', '#e5e7eb')
    expect(adminChartTheme.axisLine).toBe('#e5e7eb')
    document.documentElement.style.setProperty('--color-admin-chart-axis-line', '#3d3d3d')
    expect(adminChartTheme.axisLine).toBe('#3d3d3d')
  })

  it('keeps hue-bearing chart roles resolvable (hue smoke)', () => {
    expect(adminChartTheme.brand).not.toBe('')
    expect(adminChartTheme.cyan).not.toBe('')
  })
})
