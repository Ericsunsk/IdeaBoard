import type { ChartType, Plugin } from 'chart.js'
import { markRaw, nextTick, onBeforeUnmount, ref, watch, type Ref } from 'vue'
import {
  chartFontFamilyStack,
  createChartEmptyStatePlugin,
  defineChartNumericScale,
  defineChartOptions,
  formatCompactNumber,
  type ChartOptions,
  withAlpha,
} from '../../../../shared/lib/chartPrimitives'
import { resolveChartAnimation } from '../../../../shared/motion/chartMotion'
import { isDarkModeActive } from '../../../../shared/theme/composables/useTheme'
import { adminDarkThemeTokenFallbacks, adminThemeTokenFallbacks } from './adminThemeTokens'

export const adminChartStrokeWidth = 1
const adminChartPointClipPadding = 6
const adminChartBubbleFillOpacity = 0.3
const adminChartBubbleRadiusMin = 5
const adminChartBubbleRadiusMax = 18
const adminChartBubbleHoverRadiusOffset = 2
const adminChartBubbleClipPadding = adminChartBubbleRadiusMax + adminChartBubbleHoverRadiusOffset

const adminChartFontFamily = chartFontFamilyStack

// Chart.js paints canvas colors and helpers like withAlpha need concrete values.
// Each themed key resolves lazily per access so a theme switch applies the next
// time options are built (no watcher/remount needed beyond Chart.js updates).
function themedChartToken(
  lightKey: keyof typeof adminThemeTokenFallbacks,
  darkKey: keyof typeof adminDarkThemeTokenFallbacks,
): string {
  return isDarkModeActive()
    ? adminDarkThemeTokenFallbacks[darkKey]
    : adminThemeTokenFallbacks[lightKey]
}

export const adminChartTheme = {
  get tooltipBg() {
    return themedChartToken('controlBg', 'tooltipBg')
  },
  get tooltipBorder() {
    return themedChartToken('chartTooltipBorder', 'tooltipBorder')
  },
  get tooltipText() {
    return themedChartToken('chartTooltipText', 'tooltipText')
  },
  get axisLine() {
    return themedChartToken('chartAxisLine', 'axisLine')
  },
  get axisText() {
    return themedChartToken('textSecondary', 'axisText')
  },
  get splitLine() {
    return themedChartToken('chartSplitLine', 'splitLine')
  },
  get legendText() {
    return themedChartToken('chartLegendText', 'legendText')
  },
  get emptyText() {
    return themedChartToken('textTertiary', 'emptyText')
  },
  get cardBg() {
    return themedChartToken('controlBg', 'cardBg')
  },
  get segmentBorder() {
    return themedChartToken('textInverse', 'segmentBorder')
  },
  get mainText() {
    return themedChartToken('textPrimary', 'mainText')
  },
  brand: adminThemeTokenFallbacks.accent,
  slate: adminThemeTokenFallbacks.modelIndigo,
  mist: adminThemeTokenFallbacks.chartMist,
  cyan: adminThemeTokenFallbacks.modelCyan,
  olive: adminThemeTokenFallbacks.chartOlive,
  mauve: adminThemeTokenFallbacks.modelPurple,
  success: adminThemeTokenFallbacks.success,
  warning: adminThemeTokenFallbacks.warning,
  danger: adminThemeTokenFallbacks.danger,
  info: adminThemeTokenFallbacks.info,
}

export const adminChartPlugins = markRaw<Plugin<ChartType>[]>([
  createChartEmptyStatePlugin({
    id: 'adminEmptyState',
    textColor: adminChartTheme.emptyText,
    fontFamily: adminChartFontFamily,
  }),
])

type AdminChartNumericScaleInput = {
  values?: readonly number[]
  max?: number
  min?: number
  headroom?: number
  stacked?: boolean
  stepSize?: number
  suffix?: string
  borderColor?: string
  gridColor?: string
}

export function defineAdminChartOptions(options: ChartOptions): ChartOptions {
  return defineChartOptions(
    {
      ...options,
      animation: resolveChartAnimation('enter'),
    },
    {
      fontFamily: adminChartFontFamily,
      hideScaleTitles: true,
    },
  )
}

export function adminChartLegend(position: 'top' | 'bottom' = 'top') {
  return {
    display: true,
    position,
    align: 'center' as const,
    labels: {
      boxWidth: 10,
      boxHeight: 6,
      usePointStyle: true,
      pointStyle: 'circle' as const,
      color: adminChartTheme.legendText,
      padding: 12,
      font: {
        family: adminChartFontFamily,
        size: 12,
        weight: 500,
      },
    },
  }
}

export function defineAdminChartCountScale(input: AdminChartNumericScaleInput = {}) {
  return defineChartNumericScale({
    stacked: input.stacked,
    values: input.values,
    max: input.max,
    min: input.min,
    headroom: input.headroom,
    precision: 0,
    stepSize: input.stepSize,
    tickColor: adminChartTheme.axisText,
    gridColor: input.gridColor ?? adminChartTheme.splitLine,
    borderColor: input.borderColor,
    formatTick: formatCompactNumber,
  })
}

export function defineAdminChartDurationScale(input: AdminChartNumericScaleInput = {}) {
  return defineChartNumericScale({
    values: input.values,
    max: input.max,
    min: input.min,
    headroom: input.headroom,
    stepSize: input.stepSize,
    tickColor: adminChartTheme.axisText,
    gridColor: input.gridColor ?? adminChartTheme.splitLine,
    borderColor: input.borderColor,
    formatTick: formatAdminChartDurationSeconds,
  })
}

export function defineAdminChartPercentScale(input: AdminChartNumericScaleInput = {}) {
  return defineChartNumericScale({
    axisMin: input.min ?? 0,
    max: input.max ?? 100,
    precision: 0,
    stepSize: input.stepSize,
    tickColor: adminChartTheme.axisText,
    gridColor: input.gridColor ?? adminChartTheme.splitLine,
    borderColor: input.borderColor,
    formatTick: (value) => formatAdminChartPercent(value, input.suffix),
  })
}

export function defineAdminChartPointClipStyle() {
  return {
    clip: adminChartPointClipPadding,
  }
}

export function defineAdminChartBubblePointStyle(color: string | string[]) {
  return {
    pointBackgroundColor: mapChartColor(color, (value) =>
      withAlpha(value, adminChartBubbleFillOpacity),
    ),
    pointBorderColor: color,
    pointBorderWidth: adminChartStrokeWidth,
    pointHoverBorderWidth: adminChartStrokeWidth,
    clip: adminChartBubbleClipPadding,
  }
}

export function getAdminChartBubbleRadius(value: number, maxValue: number) {
  if (value <= 0 || maxValue <= 0) return 0
  const ratio = Math.min(1, Math.max(0, value / maxValue))
  return (
    adminChartBubbleRadiusMin +
    Math.sqrt(ratio) * (adminChartBubbleRadiusMax - adminChartBubbleRadiusMin)
  )
}

export function getAdminChartBubbleHoverRadius(value: number, maxValue: number) {
  const radius = getAdminChartBubbleRadius(value, maxValue)
  return radius > 0 ? radius + adminChartBubbleHoverRadiusOffset : 0
}

function formatAdminChartDurationSeconds(value: number) {
  return `${toFiniteChartNumber(value).toFixed(1)}s`
}

function formatAdminChartPercent(value: number, suffix = '%') {
  return `${Math.round(toFiniteChartNumber(value))}${suffix}`
}

export function useAdminChartReady(isDataSettled: Ref<boolean>) {
  const isChartReady = ref(false)
  let frame = 0

  watch(
    isDataSettled,
    async (isSettled) => {
      if (!isSettled || isChartReady.value) return
      await nextTick()

      if (typeof window === 'undefined') {
        isChartReady.value = true
        return
      }

      frame = window.requestAnimationFrame(() => {
        isChartReady.value = true
      })
    },
    { immediate: true },
  )

  onBeforeUnmount(() => {
    if (frame) window.cancelAnimationFrame(frame)
  })

  return isChartReady
}

function mapChartColor(color: string | string[], mapper: (value: string) => string) {
  return Array.isArray(color) ? color.map(mapper) : mapper(color)
}

function toFiniteChartNumber(value: number) {
  return Number.isFinite(value) ? value : 0
}

export function defineAdminChartTooltip(options: Record<string, unknown> = {}) {
  return {
    enabled: true,
    backgroundColor: adminChartTheme.tooltipBg,
    borderColor: adminChartTheme.tooltipBorder,
    borderWidth: 1,
    titleColor: adminChartTheme.mainText,
    bodyColor: adminChartTheme.tooltipText,
    titleFont: {
      size: 11,
      weight: 'bold' as const,
      family: adminChartFontFamily,
    },
    bodyFont: {
      size: 11,
      family: adminChartFontFamily,
    },
    padding: 10,
    cornerRadius: 8,
    boxWidth: 8,
    boxHeight: 8,
    boxPadding: 6,
    usePointStyle: true,
    displayColors: true,
    ...options,
  }
}
