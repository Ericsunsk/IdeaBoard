import {
  chartFontFamilyStack,
  createChartEmptyStatePlugin,
  defineChartData,
  defineChartNumericScale,
  defineChartOptions,
  formatCompactNumber,
  getParsedY,
  type ChartData,
  type ChartOptions,
} from '../../../core/lib/chartPrimitives'
import {
  resolveChartTimeTickDensity,
  shouldShowChartTimeTick,
} from '../../../core/lib/chartTimeRange'
import { resolveChartAnimation } from '../../../core/motion/chartMotion'
import { formatIdeaAmount } from '../../../core/i18n/userIdeaCurrency'
import { i18n } from '../../../core/i18n'

import type { UsageChartTheme, UsageTrendPoint } from '../core/chart'

export function createUsageChartPlugins(theme: UsageChartTheme) {
  return [
    createChartEmptyStatePlugin({
      id: 'userUsageEmptyState',
      textColor: theme.axisText,
      fontFamily: theme.fontFamily,
    }),
  ]
}

/*
 * Static fallbacks mirror the light token hex behind each readColor() call
 * (accent/ring, muted-foreground, border ×2, card, foreground) so charts
 * render the token palette even when CSS vars cannot resolve (SSR/tests).
 */
const defaultTheme: UsageChartTheme = {
  accent: '#9278e6',
  accentFill: 'rgba(146, 120, 230, 0.1)',
  axisText: '#62626a',
  axisLine: '#ececec',
  grid: '#ececec',
  surface: '#ffffff',
  text: '#313131',
  fontFamily: chartFontFamilyStack,
}

export function buildUsageTrendChart(
  trend: readonly UsageTrendPoint[],
  theme: UsageChartTheme,
): { data: ChartData; options: ChartOptions } {
  const labels = trend.map((point) => formatTrendDateLabel(point.date))
  const values = trend.map((point) => normalizeCredits(point.credits))

  const data = defineChartData({
    labels,
    datasets: [
      {
        type: 'line',
        label: i18n.global.t('account.usage.chartTitle'),
        data: values,
        hidden: values.every((value) => value <= 0),
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
      animation: resolveChartAnimation('enter'),
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
            label: (context) =>
              i18n.global.t('account.usage.tooltipLabel', {
                amount: formatIdeaAmount(getParsedY(context)),
              }),
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
                density: resolveChartTimeTickDensity(labels.length),
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
          formatTick: formatCompactNumber,
        }),
      },
    },
    { fontFamily: theme.fontFamily, hideScaleTitles: true },
  )

  return { data, options }
}

export function resolveUsageChartTheme(
  styles: Pick<CSSStyleDeclaration, 'getPropertyValue'> | null = readDocumentStyles(),
): UsageChartTheme {
  return {
    accent: readColor(styles, '--color-brand-creative-accent', defaultTheme.accent),
    accentFill: readColor(styles, '--color-brand-creative-accent-ring', defaultTheme.accentFill),
    axisText: readColor(styles, '--color-muted-foreground', defaultTheme.axisText),
    axisLine: readColor(styles, '--color-border', defaultTheme.axisLine),
    grid: readColor(styles, '--color-border', defaultTheme.grid),
    surface: readColor(styles, '--color-card', defaultTheme.surface),
    text: readColor(styles, '--color-foreground', defaultTheme.text),
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
