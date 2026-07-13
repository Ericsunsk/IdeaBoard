import type {
  ChartData as ChartJsData,
  ChartOptions as ChartJsOptions,
  ChartType,
  TooltipItem,
} from 'chart.js'
import { markRaw } from 'vue'

export type ChartData = ChartJsData<ChartType, unknown[], string>
export type ChartOptions = ChartJsOptions<ChartType>
export type ChartTooltipItem = TooltipItem<ChartType>

export type ChartNumericScaleInput = {
  values?: readonly number[]
  max?: number
  min?: number
  axisMin?: number
  headroom?: number
  stacked?: boolean
  precision?: number
  stepSize?: number
  tickColor: string
  gridColor: string
  borderColor?: string
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
  const value = hexColor.replace('#', '')
  const bigint = Number.parseInt(value.length === 3 ? expandShortHex(value) : value, 16)
  const red = (bigint >> 16) & 255
  const green = (bigint >> 8) & 255
  const blue = bigint & 255
  return `rgba(${red}, ${green}, ${blue}, ${opacity})`
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
