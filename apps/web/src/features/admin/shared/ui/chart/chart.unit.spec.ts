import { afterEach, describe, expect, it } from 'vitest'
import { adminThemeTokenFallbacks } from '../../core/adminThemeTokens'
import { adminChartTheme } from './chart'

describe('adminChartTheme', () => {
  afterEach(() => document.documentElement.style.removeProperty('--color-admin-chart-axis-line'))

  it('uses the default dark palette when CSS tokens cannot resolve', () => {
    expect(adminChartTheme.axisLine).toBe(adminThemeTokenFallbacks.chartAxisLine)
    expect(adminChartTheme.splitLine).toBe(adminThemeTokenFallbacks.chartSplitLine)
    expect(adminChartTheme.axisText).toBe(adminThemeTokenFallbacks.textSecondary)
    expect(adminChartTheme.tooltipBg).toBe(adminThemeTokenFallbacks.controlBg)
  })

  it('reads live CSS tokens instead of a separate theme palette', () => {
    document.documentElement.style.setProperty('--color-admin-chart-axis-line', '#e5e7eb')
    expect(adminChartTheme.axisLine).toBe('#e5e7eb')
    document.documentElement.style.setProperty('--color-admin-chart-axis-line', '#3d3d3d')
    expect(adminChartTheme.axisLine).toBe('#3d3d3d')
  })
})
