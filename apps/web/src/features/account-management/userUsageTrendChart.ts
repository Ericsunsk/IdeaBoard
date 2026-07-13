import {
  defineChartData,
  defineChartNumericScale,
  defineChartOptions,
  getParsedY,
  type ChartData,
  type ChartOptions,
} from '../../shared/lib/chartPrimitives'
import { shouldShowChartTimeTick, type ChartTimeTickDensity } from '../../shared/lib/chartTimeRange'

export type UserUsageTrendPoint = {
  date: string
  credits: number
}

export type UserUsageChartTheme = {
  accent: string
  accentFill: string
  axisText: string
  axisLine: string
  grid: string
  surface: string
  text: string
  fontFamily: string
}

const defaultTheme: UserUsageChartTheme = {
  accent: '#9278e6',
  accentFill: 'rgba(146, 120, 230, 0.1)',
  axisText: '#667085',
  axisLine: '#d7e0eb',
  grid: '#e7edf4',
  surface: '#ffffff',
  text: '#1d2939',
  fontFamily:
    "-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif",
}

export function buildUserUsageTrendChart(
  trend: readonly UserUsageTrendPoint[],
  theme: UserUsageChartTheme,
): { data: ChartData; options: ChartOptions } {
  const labels = trend.map((point) => formatTrendDateLabel(point.date))
  const values = trend.map((point) => normalizeCredits(point.credits))

  const data = defineChartData({
    labels,
    datasets: [
      {
        type: 'line',
        label: '点子消耗',
        data: values,
        borderColor: theme.accent,
        backgroundColor: theme.accentFill,
        fill: true,
        tension: 0.38,
        borderWidth: 1,
        clip: 6,
        pointRadius: 0,
        pointHoverRadius: 4,
        pointBackgroundColor: theme.accent,
      },
    ],
  })

  const options = defineChartOptions(
    {
      responsive: true,
      maintainAspectRatio: false,
      resizeDelay: 80,
      animation: { duration: 260 },
      interaction: { mode: 'index', intersect: false },
      plugins: {
        legend: { display: false },
        tooltip: {
          backgroundColor: theme.surface,
          borderColor: theme.axisLine,
          borderWidth: 1,
          titleColor: theme.text,
          bodyColor: theme.text,
          bodyFont: { size: 12 },
          titleFont: { size: 12, weight: 700 },
          padding: 10,
          displayColors: false,
          callbacks: {
            label: (context) => `点子消耗：${formatFullCredits(getParsedY(context))} 点子`,
          },
        },
      },
      scales: {
        x: {
          display: true,
          ticks: {
            color: theme.axisText,
            maxRotation: 0,
            autoSkip: false,
            callback: (_value, index) =>
              shouldShowChartTimeTick({
                index,
                count: labels.length,
                label: labels[index],
                density: usageTrendTickDensity(labels.length),
              })
                ? labels[index] || ''
                : '',
          },
          grid: { color: theme.grid },
          border: { color: theme.axisLine },
        },
        y: defineChartNumericScale({
          values,
          min: 10,
          headroom: 1.15,
          precision: 0,
          tickColor: theme.axisText,
          gridColor: theme.grid,
          borderColor: theme.axisLine,
          formatTick: formatCompactCredits,
        }),
      },
    },
    { fontFamily: theme.fontFamily, hideScaleTitles: true },
  )

  return { data, options }
}

function usageTrendTickDensity(count: number): ChartTimeTickDensity {
  if (count <= 7) return { mode: 'all' }
  if (count >= 365) return { mode: 'month-start' }
  return { mode: 'interval', interval: 6 }
}

export function resolveUserUsageChartTheme(
  styles: Pick<CSSStyleDeclaration, 'getPropertyValue'> | null = readDocumentStyles(),
): UserUsageChartTheme {
  return {
    accent: readColor(styles, '--color-ib-creative-accent', defaultTheme.accent),
    accentFill: readColor(styles, '--color-ib-creative-accent-ring', defaultTheme.accentFill),
    axisText: readColor(styles, '--color-ib-text-muted', defaultTheme.axisText),
    axisLine: readColor(styles, '--color-ib-border', defaultTheme.axisLine),
    grid: readColor(styles, '--color-ib-border-soft', defaultTheme.grid),
    surface: readColor(styles, '--color-ib-surface', defaultTheme.surface),
    text: readColor(styles, '--color-ib-text', defaultTheme.text),
    fontFamily: defaultTheme.fontFamily,
  }
}

function readDocumentStyles() {
  if (typeof document === 'undefined' || typeof getComputedStyle === 'undefined') return null
  return getComputedStyle(document.documentElement)
}

function readColor(
  styles: Pick<CSSStyleDeclaration, 'getPropertyValue'> | null,
  token: string,
  fallback: string,
) {
  const value = styles?.getPropertyValue(token).trim() || ''
  return value && !value.includes('var(') ? value : fallback
}

function formatTrendDateLabel(date: string) {
  const match = /^\d{4}-(\d{2})-(\d{2})$/.exec(date)
  return match ? `${match[1]}/${match[2]}` : date
}

function normalizeCredits(value: number) {
  return Number.isSafeInteger(value) && value >= 0 ? value : 0
}

function formatFullCredits(value: number) {
  return Math.max(0, Math.round(Number.isFinite(value) ? value : 0)).toLocaleString('zh-CN')
}

function formatCompactCredits(value: number) {
  const rounded = Math.max(0, Math.round(Number.isFinite(value) ? value : 0))
  if (rounded >= 1_000_000) return formatCompactUnit(rounded / 1_000_000, 'm')
  if (rounded >= 1_000) return formatCompactUnit(rounded / 1_000, 'k')
  return rounded.toString()
}

function formatCompactUnit(value: number, suffix: string) {
  return `${Number.isInteger(value) ? value : value.toFixed(1)}${suffix}`
}
