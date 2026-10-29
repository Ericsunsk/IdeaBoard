import {
  chartFontFamilyStack,
  createChartEmptyStatePlugin,
  defineChartData,
  defineChartOptions,
  formatCompactNumber,
  getParsedY,
  type ChartData,
  type ChartOptions,
  type ChartTooltipItem,
} from '../../../core/lib/chartPrimitives'
import {
  defineSharedChartNumericScale,
  defineSharedChartTooltip,
  readChartToken,
} from '../../../core/lib/chartTheme'
import { isDarkModeActive } from '../../../theme/composables/useTheme'
import {
  resolveChartTimeTickDensity,
  shouldShowChartTimeTick,
} from '../../../core/lib/chartTimeRange'
import { resolveChartAnimation } from '../../../core/motion/chartMotion'
import { formatIdeaAmount } from '../../../core/i18n/userIdeaCurrency'
import { i18n } from '../../../core/i18n'

import type { UsageChartTheme, UsageTrendPoint } from '../core/chart'

export function createUsageChartPlugins(readTheme: () => UsageChartTheme) {
  return [
    createChartEmptyStatePlugin({
      id: 'userUsageEmptyState',
      get textColor() {
        return readTheme().axisText
      },
      get fontFamily() {
        return readTheme().fontFamily
      },
    }),
  ]
}

// Concrete canvas colors when CSS tokens cannot resolve (SSR or before boot).
const darkThemeFallback: UsageChartTheme = {
  accent: '#fcfcfc',
  accentFill: 'rgba(255, 255, 255, 0.2)',
  axisText: '#a3a3a3',
  axisLine: 'rgba(255, 255, 255, 0.08)',
  grid: 'rgba(255, 255, 255, 0.08)',
  surface: 'rgba(20, 20, 20, 0.85)',
  text: '#f5f5f5',
  fontFamily: chartFontFamilyStack,
}

const lightThemeFallback: UsageChartTheme = {
  accent: '#9278e6',
  accentFill: 'rgb(146 120 230 / 10%)',
  axisText: '#737373',
  axisLine: '#e4e4e7',
  grid: '#e4e4e7',
  surface: '#ffffff',
  text: '#171717',
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
        tooltip: defineSharedChartTooltip(
          { surface: theme.surface, border: theme.axisLine, text: theme.text },
          {
            bodyFont: { size: 12 },
            titleFont: { size: 12, weight: 700 },
            displayColors: false,
            callbacks: {
              label: (context: ChartTooltipItem) =>
                i18n.global.t('account.usage.tooltipLabel', {
                  amount: formatIdeaAmount(getParsedY(context)),
                }),
            },
          },
        ),
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
        y: defineSharedChartNumericScale(theme, {
          values,
          min: 10,
          headroom: 1.15,
          precision: 0,
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
  // Track theme changes before reading the applied root palette.
  isDarkModeActive()
  const fallback = readDocumentStylesThemeIsDark() ? darkThemeFallback : lightThemeFallback
  return {
    accent: readChartToken(styles, '--color-brand-creative-accent', fallback.accent),
    accentFill: readChartToken(styles, '--color-brand-creative-accent-ring', fallback.accentFill),
    axisText: readChartToken(styles, '--muted-foreground', fallback.axisText),
    axisLine: readChartToken(styles, '--border', fallback.axisLine),
    grid: readChartToken(styles, '--border', fallback.grid),
    surface: readChartToken(styles, '--card', fallback.surface),
    text: readChartToken(styles, '--foreground', fallback.text),
    fontFamily: fallback.fontFamily,
  }
}

function readDocumentStylesThemeIsDark(): boolean {
  if (typeof document === 'undefined') return true
  return !document.documentElement.classList.contains('light')
}

function readDocumentStyles() {
  if (typeof document === 'undefined' || typeof getComputedStyle === 'undefined') return null
  return getComputedStyle(document.documentElement)
}

function formatTrendDateLabel(date: string) {
  const match = /^\d{4}-(\d{2})-(\d{2})$/.exec(date)
  return match ? `${match[1]}/${match[2]}` : date
}

function normalizeCredits(value: number) {
  return Number.isSafeInteger(value) && value >= 0 ? value : 0
}
