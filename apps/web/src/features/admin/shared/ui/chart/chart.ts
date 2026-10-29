import type { ChartType, Plugin } from 'chart.js'
import { markRaw, nextTick, onBeforeUnmount, ref, watch, type Ref } from 'vue'
import {
  chartFontFamilyStack,
  createChartEmptyStatePlugin,
  defineChartOptions,
  formatCompactNumber,
  type ChartOptions,
  withAlpha,
} from '@/core/lib/chartPrimitives'
import {
  defineSharedChartNumericScale,
  defineSharedChartTooltip,
  readChartToken,
} from '@/core/lib/chartTheme'
import { resolveChartAnimation } from '@/core/motion/chartMotion'
import { isDarkModeActive } from '@/theme/composables/useTheme'
import { adminThemeTokenFallbacks, adminThemeTokenVars } from '../../core/adminThemeTokens'

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
function themedChartToken(key: keyof typeof adminThemeTokenFallbacks): string {
  // Keep computed chart options dependent on the shared theme selection.
  isDarkModeActive()
  const styles = typeof document === 'undefined' ? null : getComputedStyle(document.documentElement)
  return readChartToken(styles, adminThemeTokenVars[key], adminThemeTokenFallbacks[key])
}

export const adminChartTheme = {
  get tooltipBg() {
    return themedChartToken('controlBg')
  },
  get tooltipBorder() {
    return themedChartToken('chartTooltipBorder')
  },
  get tooltipText() {
    return themedChartToken('chartTooltipText')
  },
  get axisLine() {
    return themedChartToken('chartAxisLine')
  },
  get axisText() {
    return themedChartToken('textSecondary')
  },
  get splitLine() {
    return themedChartToken('chartSplitLine')
  },
  get legendText() {
    return themedChartToken('chartLegendText')
  },
  get emptyText() {
    return themedChartToken('textTertiary')
  },
  get cardBg() {
    return themedChartToken('controlBg')
  },
  get segmentBorder() {
    return themedChartToken('textInverse')
  },
  get mainText() {
    return themedChartToken('textPrimary')
  },
  get brand() {
    return themedChartToken('accent')
  },
  get slate() {
    return themedChartToken('modelIndigo')
  },
  get mist() {
    return themedChartToken('chartMist')
  },
  get cyan() {
    return themedChartToken('modelCyan')
  },
  get olive() {
    return themedChartToken('chartOlive')
  },
  get mauve() {
    return themedChartToken('modelPurple')
  },
  get success() {
    return themedChartToken('success')
  },
  get warning() {
    return themedChartToken('warning')
  },
  get danger() {
    return themedChartToken('danger')
  },
  get info() {
    return themedChartToken('info')
  },
}

export const adminChartPlugins = markRaw<Plugin<ChartType>[]>([
  createChartEmptyStatePlugin({
    id: 'adminEmptyState',
    get textColor() {
      return adminChartTheme.emptyText
    },
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
  return defineSharedChartNumericScale(
    { axisText: adminChartTheme.axisText, grid: adminChartTheme.splitLine, axisLine: '' },
    {
      stacked: input.stacked,
      values: input.values,
      max: input.max,
      min: input.min,
      headroom: input.headroom,
      precision: 0,
      stepSize: input.stepSize,
      gridColor: input.gridColor,
      borderColor: input.borderColor,
      formatTick: formatCompactNumber,
    },
  )
}

export function defineAdminChartDurationScale(input: AdminChartNumericScaleInput = {}) {
  return defineSharedChartNumericScale(
    { axisText: adminChartTheme.axisText, grid: adminChartTheme.splitLine, axisLine: '' },
    {
      values: input.values,
      max: input.max,
      min: input.min,
      headroom: input.headroom,
      stepSize: input.stepSize,
      gridColor: input.gridColor,
      borderColor: input.borderColor,
      formatTick: formatAdminChartDurationSeconds,
    },
  )
}

export function defineAdminChartPercentScale(input: AdminChartNumericScaleInput = {}) {
  return defineSharedChartNumericScale(
    { axisText: adminChartTheme.axisText, grid: adminChartTheme.splitLine, axisLine: '' },
    {
      axisMin: input.min ?? 0,
      max: input.max ?? 100,
      precision: 0,
      stepSize: input.stepSize,
      gridColor: input.gridColor,
      borderColor: input.borderColor,
      formatTick: (value) => formatAdminChartPercent(value, input.suffix),
    },
  )
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
    ...defineSharedChartTooltip(
      {
        surface: adminChartTheme.tooltipBg,
        border: adminChartTheme.tooltipBorder,
        text: adminChartTheme.tooltipText,
      },
      {
        titleColor: adminChartTheme.mainText,
      },
    ),
    cornerRadius: 8,
    ...options,
  }
}
