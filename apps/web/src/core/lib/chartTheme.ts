import {
  chartFontFamilyStack,
  defineChartNumericScale,
  type ChartNumericScaleInput,
} from './chartPrimitives'

/** Shared token readers and Chart.js factories; callers own palette semantics. */
type ChartTokenStyles = Pick<CSSStyleDeclaration, 'getPropertyValue'> | null

export function readChartToken(styles: ChartTokenStyles, token: string, fallback: string): string {
  const value = styles?.getPropertyValue(token).trim() || ''
  return value && !value.includes('var(') ? value : fallback
}

type SharedChartTooltipTheme = {
  surface: string
  border: string
  text: string
}

type SharedChartScaleTheme = {
  axisText: string
  grid: string
  axisLine: string
}

export function defineSharedChartTooltip(
  theme: SharedChartTooltipTheme,
  overrides: Record<string, unknown> = {},
): Record<string, unknown> {
  return {
    enabled: true,
    backgroundColor: theme.surface,
    borderColor: theme.border,
    borderWidth: 1,
    titleColor: theme.text,
    bodyColor: theme.text,
    titleFont: {
      size: 11,
      weight: 'bold' as const,
      family: chartFontFamilyStack,
    },
    bodyFont: {
      size: 11,
      family: chartFontFamilyStack,
    },
    padding: 10,
    boxWidth: 8,
    boxHeight: 8,
    boxPadding: 6,
    usePointStyle: true,
    displayColors: true,
    ...overrides,
  }
}

type SharedChartNumericScaleInput = Omit<ChartNumericScaleInput, 'tickColor' | 'gridColor'> & {
  gridColor?: string | undefined
}

export function defineSharedChartNumericScale(
  theme: SharedChartScaleTheme,
  input: SharedChartNumericScaleInput,
): ReturnType<typeof defineChartNumericScale> {
  return defineChartNumericScale({
    ...input,
    tickColor: theme.axisText,
    gridColor: input.gridColor ?? theme.grid,
    borderColor: input.borderColor ?? theme.axisLine,
  })
}
