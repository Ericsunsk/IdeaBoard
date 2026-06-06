import type { Chart, ChartData, ChartOptions, ChartType, Plugin, TooltipItem } from 'chart.js'
import { markRaw, nextTick, onBeforeUnmount, ref, watch, type Ref } from 'vue'

export type AdminChartData = ChartData<ChartType, unknown[], string>
export type AdminChartOptions = ChartOptions<ChartType>
export type AdminTooltipItem = TooltipItem<ChartType>

const adminHiddenScaleTitle = { display: false } as const
export const adminChartStrokeWidth = 1
const adminChartBubbleFillOpacity = 0.3
const adminChartBubbleRadiusMin = 5
const adminChartBubbleRadiusMax = 18
const adminChartBubbleHoverRadiusOffset = 2

export const adminChartTheme = {
  tooltipBg: '#f8fafc',
  tooltipBorder: '#d9e2ef',
  tooltipText: '#0f172a',
  axisLine: '#d5deea',
  axisText: '#667085',
  splitLine: '#edf2f7',
  legendText: '#475569',
  emptyText: '#98a2b3',
  cardBg: '#f8fafc',
  mainText: '#1d2939',
  brand: '#26364f',
  slate: '#4f6f9f',
  mist: '#6f879f',
  cyan: '#3d8b9a',
  olive: '#6f8b71',
  mauve: '#7b668f',
  success: '#2f9e73',
  warning: '#d48830',
  danger: '#d14d5a',
  info: '#4775a8',
}

export const adminChartPlugins = markRaw<Plugin<ChartType>[]>([
  {
    id: 'adminEmptyState',
    afterDraw(chart) {
      if (!isChartDataEmpty(chart)) return

      const { ctx, chartArea } = chart
      if (!chartArea) return

      ctx.save()
      ctx.fillStyle = adminChartTheme.emptyText
      ctx.font = '600 12px Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont'
      ctx.textAlign = 'center'
      ctx.textBaseline = 'middle'
      ctx.fillText(
        '暂无数据',
        (chartArea.left + chartArea.right) / 2,
        (chartArea.top + chartArea.bottom) / 2,
      )
      ctx.restore()
    },
  },
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

export function defineAdminChartData(data: AdminChartData): AdminChartData {
  return markRaw(data)
}

export function defineAdminChartOptions(options: AdminChartOptions): AdminChartOptions {
  return markRaw(withAdminChartDefaults(options))
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
        size: 12,
        weight: 500,
      },
    },
  }
}

export function defineAdminChartCountScale(input: AdminChartNumericScaleInput = {}) {
  return {
    stacked: input.stacked,
    beginAtZero: true,
    max: resolveAdminChartScaleMax(input),
    ticks: {
      color: adminChartTheme.axisText,
      precision: 0,
      stepSize: input.stepSize,
      callback: (value: string | number) => formatAdminChartCompactNumber(Number(value)),
    },
    grid: { color: input.gridColor ?? adminChartTheme.splitLine },
    border: input.borderColor ? { color: input.borderColor } : { display: false },
  }
}

export function defineAdminChartDurationScale(input: AdminChartNumericScaleInput = {}) {
  return {
    beginAtZero: true,
    max: resolveAdminChartScaleMax(input),
    ticks: {
      color: adminChartTheme.axisText,
      stepSize: input.stepSize,
      callback: (value: string | number) => formatAdminChartDurationSeconds(Number(value)),
    },
    grid: { color: input.gridColor ?? adminChartTheme.splitLine },
    border: input.borderColor ? { color: input.borderColor } : { display: false },
  }
}

export function defineAdminChartPercentScale(input: AdminChartNumericScaleInput = {}) {
  return {
    min: input.min ?? 0,
    max: input.max ?? 100,
    beginAtZero: true,
    ticks: {
      color: adminChartTheme.axisText,
      precision: 0,
      stepSize: input.stepSize,
      callback: (value: string | number) => formatAdminChartPercent(Number(value), input.suffix),
    },
    grid: { color: input.gridColor ?? adminChartTheme.splitLine },
    border: input.borderColor ? { color: input.borderColor } : { display: false },
  }
}

export function defineAdminChartRadialCountScale(input: AdminChartNumericScaleInput = {}) {
  return {
    beginAtZero: true,
    max: resolveAdminChartScaleMax(input),
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
      font: { size: 12, weight: 600 },
    },
  }
}

export function defineAdminChartBubbleDatasetStyle(color: string | string[]) {
  return {
    backgroundColor: mapChartColor(color, (value) => withAlpha(value, adminChartBubbleFillOpacity)),
    borderColor: color,
    borderWidth: adminChartStrokeWidth,
    clip: false as const,
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

export function getAdminChartNiceScaleMax(
  values: readonly number[],
  options: Pick<AdminChartNumericScaleInput, 'min' | 'headroom'> = {},
) {
  const peak = Math.max(0, ...values.map(toFiniteChartNumber))
  return getAdminChartNiceMax(peak, options)
}

export function getAdminChartNiceMax(
  value: number,
  options: Pick<AdminChartNumericScaleInput, 'min' | 'headroom'> = {},
) {
  const headroom = options.headroom ?? 1
  const min = options.min ?? 0
  return ceilToNiceChartMax(Math.max(min, toFiniteChartNumber(value) * headroom))
}

export function formatAdminChartCompactNumber(value: number) {
  const rounded = Math.round(toFiniteChartNumber(value))
  if (rounded >= 1_000_000) return formatCompactUnit(rounded / 1_000_000, 'm')
  if (rounded >= 1_000) return formatCompactUnit(rounded / 1_000, 'k')
  return rounded.toString()
}

export function formatAdminChartDurationSeconds(value: number) {
  return `${toFiniteChartNumber(value).toFixed(1)}s`
}

export function formatAdminChartPercent(value: number, suffix = '%') {
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

export function withAlpha(hexColor: string, opacity: number) {
  const value = hexColor.replace('#', '')
  const bigint = Number.parseInt(value.length === 3 ? expandShortHex(value) : value, 16)
  const red = (bigint >> 16) & 255
  const green = (bigint >> 8) & 255
  const blue = bigint & 255
  return `rgba(${red}, ${green}, ${blue}, ${opacity})`
}

export function getParsedY(context: AdminTooltipItem) {
  const parsed = context.parsed as { y?: number } | number | undefined
  if (typeof parsed === 'number') return parsed
  return Number(parsed?.y ?? context.raw ?? 0)
}

function expandShortHex(value: string) {
  return value
    .split('')
    .map((char) => char + char)
    .join('')
}

function mapChartColor(color: string | string[], mapper: (value: string) => string) {
  return Array.isArray(color) ? color.map(mapper) : mapper(color)
}

function resolveAdminChartScaleMax(input: AdminChartNumericScaleInput) {
  if (input.max != null) return input.max
  if (!input.values) return undefined
  return getAdminChartNiceScaleMax(input.values, {
    min: input.min,
    headroom: input.headroom,
  })
}

function ceilToNiceChartMax(value: number) {
  if (value <= 0) return 10
  const magnitude = 10 ** Math.floor(Math.log10(value))
  const normalized = value / magnitude
  const step = [1, 1.5, 2, 3, 5, 10].find((item) => normalized <= item) || 10
  return step * magnitude
}

function formatCompactUnit(value: number, unit: string) {
  const text = value >= 10 ? Math.round(value).toString() : value.toFixed(1).replace(/\.0$/, '')
  return `${text}${unit}`
}

function toFiniteChartNumber(value: number) {
  return Number.isFinite(value) ? value : 0
}

function withAdminChartDefaults(options: AdminChartOptions): AdminChartOptions {
  if (!options.scales) return options

  Object.values(options.scales).forEach((scaleOptions) => {
    if (!scaleOptions || typeof scaleOptions !== 'object') return
    const scale = scaleOptions as { title?: { display?: boolean } }
    scale.title = { ...scale.title, ...adminHiddenScaleTitle }
  })

  return options
}

function isChartDataEmpty(chart: Chart<ChartType>) {
  return chart.data.datasets.every((dataset) =>
    dataset.data.every((item) => getDatumMagnitude(item) <= 0),
  )
}

function getDatumMagnitude(item: unknown): number {
  if (typeof item === 'number') return Math.abs(item)
  if (!item || typeof item !== 'object') return 0

  const record = item as Record<string, unknown>
  const x = Number(record.x ?? 0)
  const y = Number(record.y ?? 0)
  const value = Number(record.value ?? 0)
  return Math.abs(x) + Math.abs(y) + Math.abs(value)
}
