import { afterEach, describe, expect, it, vi } from 'vitest'
import { buildUsageTrendChart, resolveUsageChartTheme, type UsageChartTheme } from './chart'

const theme: UsageChartTheme = {
  accent: '#9278e6',
  accentFill: 'rgba(146, 120, 230, 0.1)',
  axisText: '#667085',
  axisLine: '#d7e0eb',
  grid: '#e7edf4',
  surface: '#ffffff',
  text: '#1d2939',
  fontFamily: 'system-ui',
}

describe('buildUsageTrendChart', () => {
  afterEach(() => {
    document.documentElement.style.removeProperty('--motion-enter-duration')
    document.documentElement.style.removeProperty('--motion-enter-easing')
    vi.unstubAllGlobals()
  })

  it('uses Motion Policy timing and disables chart animation under reduced motion', () => {
    document.documentElement.style.setProperty('--motion-enter-duration', '220ms')
    document.documentElement.style.setProperty(
      '--motion-enter-easing',
      'cubic-bezier(0.2, 0.8, 0.2, 1)',
    )
    vi.stubGlobal(
      'matchMedia',
      vi.fn(() => ({ matches: false })),
    )

    expect(buildUsageTrendChart([], theme).options.animation).toMatchObject({ duration: 220 })

    vi.stubGlobal(
      'matchMedia',
      vi.fn(() => ({ matches: true })),
    )
    expect(buildUsageTrendChart([], theme).options.animation).toBe(false)
  })

  it('keeps all 30 server date keys and a single zero-value dataset', () => {
    const trend = buildThirtyDayTrend(0)

    const chart = buildUsageTrendChart(trend, theme)

    expect(chart.data.labels).toHaveLength(30)
    expect(chart.data.labels?.[0]).toBe('06/14')
    expect(chart.data.labels?.[29]).toBe('07/13')
    expect(chart.data.datasets).toHaveLength(1)
    expect(chart.data.datasets[0]).toMatchObject({
      label: 'Ideas used',
      data: Array(30).fill(0),
      hidden: true,
      fill: true,
      tension: 0.38,
      pointRadius: 0,
      pointHoverRadius: 4,
    })
  })

  it('uses stable responsive options, monthly tick density, and full tooltip values', () => {
    const trend = buildThirtyDayTrend(1_234_567)
    const chart = buildUsageTrendChart(trend, theme)
    const options = chart.options as {
      responsive: boolean
      maintainAspectRatio: boolean
      interaction: { mode: string; intersect: boolean }
      plugins: { tooltip: { callbacks: { label: (context: unknown) => string } } }
      scales: {
        x: {
          display: boolean
          ticks: {
            autoSkip: boolean
            callback: (_value: unknown, index: number) => string
          }
        }
        y: { ticks: { callback: (value: number) => string } }
      }
    }

    expect(options).toMatchObject({
      responsive: true,
      maintainAspectRatio: false,
      interaction: { mode: 'index', intersect: false },
      scales: { x: { display: true, ticks: { autoSkip: false } } },
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
    ).toBe('Ideas used: 1,234,567 Ideas')
  })

  it('shows every weekly label and month boundaries for the 365-day range', () => {
    const weekly = buildDateTrend('2026-07-07', 7, 1)
    const weeklyChart = buildUsageTrendChart(weekly, theme)
    const weeklyTicks = getXTickCallback(weeklyChart.options)
    expect(weekly.map((_point, index) => weeklyTicks(null, index))).toEqual([
      '07/07',
      '07/08',
      '07/09',
      '07/10',
      '07/11',
      '07/12',
      '07/13',
    ])

    const yearly = buildDateTrend('2025-07-14', 365, 1)
    const yearlyChart = buildUsageTrendChart(yearly, theme)
    const yearlyTicks = getXTickCallback(yearlyChart.options)
    const septemberFirstIndex = yearly.findIndex((point) => point.date === '2025-09-01')
    const septemberSecondIndex = yearly.findIndex((point) => point.date === '2025-09-02')
    expect(yearlyChart.data.labels).toHaveLength(365)
    expect(yearlyTicks(null, 0)).toBe('07/14')
    expect(yearlyTicks(null, septemberFirstIndex)).toBe('09/01')
    expect(yearlyTicks(null, septemberSecondIndex)).toBe('')
    expect(yearlyTicks(null, 364)).toBe('07/13')
  })
})

describe('resolveUsageChartTheme fallbacks', () => {
  it('mirrors the light token hex when CSS vars cannot resolve', () => {
    expect(resolveUsageChartTheme(null)).toMatchObject({
      accent: '#9278e6',
      accentFill: 'rgba(146, 120, 230, 0.1)',
      axisText: '#62626a',
      axisLine: '#ececec',
      grid: '#ececec',
      surface: '#ffffff',
      text: '#313131',
    })
  })
})

function buildThirtyDayTrend(credits: number) {
  return [
    '06-14',
    '06-15',
    '06-16',
    '06-17',
    '06-18',
    '06-19',
    '06-20',
    '06-21',
    '06-22',
    '06-23',
    '06-24',
    '06-25',
    '06-26',
    '06-27',
    '06-28',
    '06-29',
    '06-30',
    '07-01',
    '07-02',
    '07-03',
    '07-04',
    '07-05',
    '07-06',
    '07-07',
    '07-08',
    '07-09',
    '07-10',
    '07-11',
    '07-12',
    '07-13',
  ].map((date) => ({ date: `2026-${date}`, credits }))
}

function buildDateTrend(start: string, days: number, credits: number) {
  const startDate = new Date(`${start}T00:00:00Z`)
  return Array.from({ length: days }, (_, index) => {
    const date = new Date(startDate.getTime() + index * 24 * 60 * 60 * 1_000)
    return { date: date.toISOString().slice(0, 10), credits }
  })
}

function getXTickCallback(options: unknown) {
  return (
    options as {
      scales: { x: { ticks: { callback: (_value: unknown, index: number) => string } } }
    }
  ).scales.x.ticks.callback
}
