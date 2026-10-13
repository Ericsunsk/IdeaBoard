import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { useTheme } from '../../../../../theme/composables/useTheme'
import { adminThemeTokenFallbacks } from '../../core/adminThemeTokens'
import { adminChartTheme } from './chart'

describe('adminChartTheme', () => {
  beforeEach(() => {
    useTheme().setTheme('light')
  })

  afterEach(() => {
    useTheme().setTheme('light')
  })

  it('uses standard light mode axis line and split line by default', () => {
    expect(adminChartTheme.axisLine).toBe(adminThemeTokenFallbacks.chartAxisLine)
    expect(adminChartTheme.splitLine).toBe(adminThemeTokenFallbacks.chartSplitLine)
  })

  it('swaps axis line and split line colors in dark mode', () => {
    useTheme().setTheme('dark')

    expect(adminChartTheme.axisLine).toBe('#3f3f46')
    expect(adminChartTheme.splitLine).toBe('#27272a')
    expect(adminChartTheme.axisText).toBe('#a1a1aa')
    expect(adminChartTheme.tooltipBg).toBe('#18181b')
  })
})
