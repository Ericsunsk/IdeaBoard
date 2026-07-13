import { describe, expect, it } from 'vitest'
import { buildUserUsageTrendChart, type UserUsageChartTheme } from './userUsageTrendChart'

const theme: UserUsageChartTheme = {
  accent: '#9278e6',
  accentFill: 'rgba(146, 120, 230, 0.1)',
  axisText: '#667085',
  axisLine: '#d7e0eb',
  grid: '#e7edf4',
  surface: '#ffffff',
  text: '#1d2939',
  fontFamily: 'system-ui',
}

describe('buildUserUsageTrendChart', () => {
  it('keeps all 30 server date keys and a single zero-value dataset', () => {
    const trend = buildThirtyDayTrend(0)

    const chart = buildUserUsageTrendChart(trend, theme)

    expect(chart.data.labels).toHaveLength(30)
    expect(chart.data.labels?.[0]).toBe('06/14')
    expect(chart.data.labels?.[29]).toBe('07/13')
    expect(chart.data.datasets).toHaveLength(1)
    expect(chart.data.datasets[0]).toMatchObject({
      label: '点子消耗',
      data: Array(30).fill(0),
      fill: true,
      tension: 0.38,
      pointRadius: 0,
      pointHoverRadius: 4,
    })
  })

  it('uses stable responsive options, monthly tick density, and full tooltip values', () => {
    const trend = buildThirtyDayTrend(1_234_567)
    const chart = buildUserUsageTrendChart(trend, theme)
    const options = chart.options as {
      responsive: boolean
      maintainAspectRatio: boolean
      interaction: { mode: string; intersect: boolean }
      plugins: { tooltip: { callbacks: { label: (context: unknown) => string } } }
      scales: {
        x: { ticks: { callback: (_value: unknown, index: number) => string } }
        y: { ticks: { callback: (value: number) => string } }
      }
    }

    expect(options).toMatchObject({
      responsive: true,
      maintainAspectRatio: false,
      interaction: { mode: 'index', intersect: false },
    })
    expect(options.scales.x.ticks.callback(null, 0)).toBe('06/14')
    expect(options.scales.x.ticks.callback(null, 1)).toBe('')
    expect(options.scales.x.ticks.callback(null, 6)).toBe('06/20')
    expect(options.scales.x.ticks.callback(null, 29)).toBe('07/13')
    expect(options.scales.y.ticks.callback(1_200)).toBe('1.2k')
    expect(
      options.plugins.tooltip.callbacks.label({
        parsed: { y: 1_234_567 },
        raw: 1_234_567,
      }),
    ).toBe('点子消耗：1,234,567 点子')
  })
})

function buildThirtyDayTrend(credits: number) {
  return [
    '06-14', '06-15', '06-16', '06-17', '06-18', '06-19', '06-20', '06-21', '06-22',
    '06-23', '06-24', '06-25', '06-26', '06-27', '06-28', '06-29', '06-30', '07-01',
    '07-02', '07-03', '07-04', '07-05', '07-06', '07-07', '07-08', '07-09', '07-10',
    '07-11', '07-12', '07-13',
  ].map((date) => ({ date: `2026-${date}`, credits }))
}
