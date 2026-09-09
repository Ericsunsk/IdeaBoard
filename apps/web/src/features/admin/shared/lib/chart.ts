import type { ChartType, Plugin } from 'chart.js'
import { markRaw, nextTick, onBeforeUnmount, ref, watch, type Ref } from 'vue'
import {
  createChartEmptyStatePlugin,
  defineChartNumericScale,
  defineChartOptions,
  resolveChartScaleMax,
  type ChartOptions,
  withAlpha,
} from '../../../../shared/lib/chartPrimitives'
import { resolveChartAnimation } from '../../../../shared/motion/chartMotion'
import { adminThemeTokenFallbacks } from '../../../../shared/theme/adminThemeTokens'

export const adminChartStrokeWidth = 1
export const adminChartPointClipPadding = 6
const adminChartBubbleFillOpacity = 0.3
const adminChartBubbleRadiusMin = 5
const adminChartBubbleRadiusMax = 18
const adminChartBubbleHoverRadiusOffset = 2
export const adminChartBubbleClipPadding =
  adminChartBubbleRadiusMax + adminChartBubbleHoverRadiusOffset

const adminChartFontFamily =
  "-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif"

// Chart.js paints canvas colors and helpers like withAlpha need concrete values.
export const adminChartTheme = {
  tooltipBg: adminThemeTokenFallbacks.controlBg,
  tooltipBorder: adminThemeTokenFallbacks.chartTooltipBorder,
  tooltipText: adminThemeTokenFallbacks.chartTooltipText,
  axisLine: adminThemeTokenFallbacks.chartAxisLine,
  axisText: adminThemeTokenFallbacks.textSecondary,
  splitLine: adminThemeTokenFallbacks.chartSplitLine,
  legendText: adminThemeTokenFallbacks.chartLegendText,
  emptyText: adminThemeTokenFallbacks.textTertiary,
  cardBg: adminThemeTokenFallbacks.controlBg,
  segmentBorder: adminThemeTokenFallbacks.textInverse,
  mainText: adminThemeTokenFallbacks.textPrimary,
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
    formatTick: formatAdminChartCompactNumber,
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

export function defineAdminChartRadialCountScale(input: AdminChartNumericScaleInput = {}) {
  return {
    beginAtZero: true,
    max: resolveChartScaleMax(input),
    ticks: {
      backdropColor: 'transparent',
      color: adminChartTheme.axisText,
      precision: 0,
      stepSize: input.stepSize,
      callback: (value: string | number) => formatAdminChartCompactNumber(Number(value)),
    },
    grid: { color: input.gridColor ?? adminChartTheme.splitLine },
  }
}

export function defineAdminChartRadialPercentScale(input: AdminChartNumericScaleInput = {}) {
  return {
    min: input.min ?? 0,
    max: input.max ?? 100,
    beginAtZero: true,
    ticks: {
      backdropColor: 'transparent',
      color: adminChartTheme.axisText,
      precision: 0,
      stepSize: input.stepSize,
      callback: (value: string | number) => formatAdminChartPercent(Number(value), input.suffix),
    },
    grid: { color: input.gridColor ?? adminChartTheme.splitLine },
    angleLines: { color: input.gridColor ?? adminChartTheme.splitLine },
    pointLabels: {
      color: adminChartTheme.axisText,
      font: { family: adminChartFontFamily, size: 12, weight: 600 },
    },
  }
}

export function defineAdminChartPointClipStyle() {
  return {
    clip: adminChartPointClipPadding,
  }
}

export function defineAdminChartBubbleDatasetStyle(color: string | string[]) {
  return {
    backgroundColor: mapChartColor(color, (value) => withAlpha(value, adminChartBubbleFillOpacity)),
    borderColor: color,
    borderWidth: adminChartStrokeWidth,
    clip: adminChartBubbleClipPadding,
    hoverBorderWidth: adminChartStrokeWidth,
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

function formatAdminChartCompactNumber(value: number) {
  const rounded = Math.round(toFiniteChartNumber(value))
  if (rounded >= 1_000_000) return formatCompactUnit(rounded / 1_000_000, 'm')
  if (rounded >= 1_000) return formatCompactUnit(rounded / 1_000, 'k')
  return rounded.toString()
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

function formatCompactUnit(value: number, unit: string) {
  const text = value >= 10 ? Math.round(value).toString() : value.toFixed(1).replace(/\.0$/, '')
  return `${text}${unit}`
}

function toFiniteChartNumber(value: number) {
  return Number.isFinite(value) ? value : 0
}
