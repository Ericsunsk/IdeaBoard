import type {
  Chart,
  ChartData as ChartJsData,
  ChartOptions as ChartJsOptions,
  ChartType,
  Plugin,
  TooltipItem,
} from 'chart.js'
import { markRaw } from 'vue'
import { i18n } from '../i18n'

export type ChartData = ChartJsData<ChartType, unknown[], string>

// Canvas 拿不到 CSS var：字体栈必须用 concrete value，与 Tier 1
// --primitive-font-inter-variable 同源（Inter Variable + CJK fallback chain +
// system stack）。改这里时同步改 primitives.css，两处保持语义一致（CSS 侧可换行，TS 侧单行）。
export const chartFontFamilyStack =
  "'Inter Variable', 'PingFang SC', 'Microsoft YaHei', 'Noto Sans CJK SC', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif"
export type ChartOptions = ChartJsOptions<ChartType>
export type ChartTooltipItem = TooltipItem<ChartType>

export type ChartEmptyStatePluginInput = {
  id: string
  textColor: string
  fontFamily: string
  text?: string
}

export function createChartEmptyStatePlugin(input: ChartEmptyStatePluginInput): Plugin<ChartType> {
  return {
    id: input.id,
    afterDraw(chart) {
      if (!isChartDataEmpty(chart)) return

      const { ctx, chartArea } = chart
      if (!chartArea) return

      ctx.save()
      ctx.fillStyle = input.textColor
      ctx.font = `600 12px ${input.fontFamily}`
      ctx.textAlign = 'center'
      ctx.textBaseline = 'middle'
      ctx.fillText(
        input.text ?? i18n.global.t('common.states.noData'),
        (chartArea.left + chartArea.right) / 2,
        (chartArea.top + chartArea.bottom) / 2,
      )
      ctx.restore()
    },
  }
}

export type ChartNumericScaleInput = {
  values?: readonly number[] | undefined
  max?: number | undefined
  min?: number | undefined
  axisMin?: number | undefined
  headroom?: number | undefined
  stacked?: boolean | undefined
  precision?: number | undefined
  stepSize?: number | undefined
  tickColor: string
  gridColor: string
  borderColor?: string | undefined
  formatTick: (value: number) => string
}

export type ChartTypographyOptions = {
  fontFamily: string
  hideScaleTitles?: boolean
}

export function defineChartData(data: ChartData): ChartData {
  return markRaw(data)
}

export function defineChartOptions(
  options: ChartOptions,
  typography?: ChartTypographyOptions,
): ChartOptions {
  if (typography) applyChartTypography(options, typography)
  return markRaw(options)
}

export function defineChartNumericScale(input: ChartNumericScaleInput) {
  return {
    stacked: input.stacked,
    ...(input.axisMin == null ? {} : { min: input.axisMin }),
    beginAtZero: true,
    max: resolveChartScaleMax(input),
    ticks: {
      color: input.tickColor,
      precision: input.precision,
      stepSize: input.stepSize,
      callback: (value: string | number) => input.formatTick(Number(value)),
    },
    grid: { color: input.gridColor },
    border: input.borderColor ? { color: input.borderColor } : { display: false },
  }
}

export function withAlpha(hexColor: string, opacity: number) {
  const value = hexColor.trim().replace(/^#/, '')
  const normalized = value.length === 3 ? expandShortHex(value) : value
  // Chart.js paints canvas pixels, so colors must resolve to concrete values
  // here (CSS vars / color-mix are not available). Invalid input falls back
  // to the raw string instead of producing `rgba(NaN, …)`.
  if (!/^[0-9a-fA-F]{6}$/.test(normalized)) return hexColor
  const bigint = Number.parseInt(normalized, 16)
  const red = (bigint >> 16) & 255
  const green = (bigint >> 8) & 255
  const blue = bigint & 255
  const alpha = Number.isFinite(opacity) ? Math.min(1, Math.max(0, opacity)) : 1
  return `rgba(${red}, ${green}, ${blue}, ${alpha})`
}

export function getParsedY(context: ChartTooltipItem) {
  const parsed = context.parsed as { y?: number } | number | undefined
  if (typeof parsed === 'number') return parsed
  return Number(parsed?.y ?? context.raw ?? 0)
}

export function resolveChartScaleMax(
  input: Pick<ChartNumericScaleInput, 'values' | 'max' | 'min' | 'headroom'>,
) {
  if (input.max != null) return input.max
  if (!input.values) return undefined

  const peak = Math.max(0, ...input.values.map(toFiniteChartNumber))
  const headroom = input.headroom ?? 1
  const min = input.min ?? 0
  return ceilToNiceChartMax(Math.max(min, peak * headroom))
}

function ceilToNiceChartMax(value: number) {
  if (value <= 0) return 10
  const magnitude = 10 ** Math.floor(Math.log10(value))
  const normalized = value / magnitude
  const step = [1, 1.5, 2, 3, 5, 10].find((item) => normalized <= item) || 10
  return step * magnitude
}

function toFiniteChartNumber(value: number) {
  return Number.isFinite(value) ? value : 0
}

// 紧凑数字（k/m 后缀）：轴刻度单源。非负钳制——轴上不展示负数，
// 垃圾输入归零而不是透出；精确值展示用 formatCount（千分位全数字）。
export function formatCompactNumber(value: number): string {
  const rounded = Math.max(0, Math.round(toFiniteChartNumber(value)))
  if (rounded >= 1_000_000) return formatCompactUnit(rounded / 1_000_000, 'm')
  if (rounded >= 1_000) return formatCompactUnit(rounded / 1_000, 'k')
  return rounded.toString()
}

function formatCompactUnit(value: number, suffix: string): string {
  return `${Number.isInteger(value) ? value : value.toFixed(1)}${suffix}`
}

function expandShortHex(value: string) {
  return value
    .split('')
    .map((char) => char + char)
    .join('')
}

type ChartFontOptions = {
  family?: string
  size?: number
  weight?: string | number
}

type ChartScaleTypographyOptions = {
  title?: { display?: boolean }
  ticks?: {
    font?: ChartFontOptions
  }
  pointLabels?: {
    font?: ChartFontOptions
  }
}

type ChartPluginTypographyOptions = {
  legend?:
    | false
    | {
        labels?: {
          font?: ChartFontOptions
        }
      }
  tooltip?:
    | false
    | {
        bodyFont?: ChartFontOptions
        titleFont?: ChartFontOptions
        footerFont?: ChartFontOptions
      }
}

function applyChartTypography(options: ChartOptions, typography: ChartTypographyOptions) {
  applyChartPluginTypography(options, typography.fontFamily)

  if (!options.scales) return

  Object.values(options.scales).forEach((scaleOptions) => {
    if (!scaleOptions || typeof scaleOptions !== 'object') return
    const scale = scaleOptions as ChartScaleTypographyOptions
    if (typography.hideScaleTitles) {
      scale.title = { ...scale.title, display: false }
    }
    scale.ticks = {
      ...scale.ticks,
      font: withChartFont(scale.ticks?.font, typography.fontFamily),
    }
    if (scale.pointLabels) {
      scale.pointLabels = {
        ...scale.pointLabels,
        font: withChartFont(scale.pointLabels.font, typography.fontFamily),
      }
    }
  })
}

function applyChartPluginTypography(options: ChartOptions, fontFamily: string) {
  if (!options.plugins || typeof options.plugins !== 'object') return

  const plugins = options.plugins as ChartPluginTypographyOptions
  if (plugins.legend && typeof plugins.legend === 'object' && plugins.legend.labels) {
    plugins.legend.labels.font = withChartFont(plugins.legend.labels.font, fontFamily)
  }
  if (plugins.tooltip && typeof plugins.tooltip === 'object') {
    plugins.tooltip.bodyFont = withChartFont(plugins.tooltip.bodyFont, fontFamily)
    plugins.tooltip.titleFont = withChartFont(plugins.tooltip.titleFont, fontFamily)
    plugins.tooltip.footerFont = withChartFont(plugins.tooltip.footerFont, fontFamily)
  }
}

function withChartFont(font: ChartFontOptions | undefined, fontFamily: string): ChartFontOptions {
  return {
    ...font,
    family: fontFamily,
  }
}

function isChartDataEmpty(chart: Chart<ChartType>) {
  return chart.data.datasets.every((dataset) =>
    dataset.data.every((item) => getChartDatumMagnitude(item) <= 0),
  )
}

function getChartDatumMagnitude(item: unknown): number {
  if (typeof item === 'number') return Math.abs(item)
  if (!item || typeof item !== 'object') return 0

  const record = item as Record<string, unknown>
  const x = Number(record.x ?? 0)
  const y = Number(record.y ?? 0)
  const value = Number(record.value ?? 0)
  return Math.abs(x) + Math.abs(y) + Math.abs(value)
}
