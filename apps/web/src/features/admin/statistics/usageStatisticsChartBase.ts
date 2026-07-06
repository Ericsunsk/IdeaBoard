import {
  adminChartLegend,
  adminChartTheme,
  type AdminChartOptions,
} from '../shared/lib/adminChart'

export const defaultModelColors = [
  adminChartTheme.brand,
  adminChartTheme.slate,
  adminChartTheme.cyan,
  adminChartTheme.olive,
  adminChartTheme.mauve,
  adminChartTheme.mist,
]

export function baseChartOptions(): AdminChartOptions {
  return {
    responsive: true,
    maintainAspectRatio: false,
    resizeDelay: 80,
    animation: { duration: 260 },
    plugins: baseChartPlugins(),
  }
}

export function baseChartPlugins() {
  return {
    legend: adminChartLegend(),
    tooltip: baseTooltipOptions(),
  }
}

export function baseTooltipOptions() {
  return {
    backgroundColor: adminChartTheme.tooltipBg,
    borderColor: adminChartTheme.tooltipBorder,
    borderWidth: 1,
    titleColor: adminChartTheme.tooltipText,
    bodyColor: adminChartTheme.tooltipText,
    bodyFont: { size: 12 },
    titleFont: { size: 12, weight: 700 },
    padding: 10,
    displayColors: true,
  }
}

export function normalizeModelName(name: string | null | undefined): string {
  return (name || '').trim().toLowerCase()
}

export function colorForModel(name: string, palette: string[]) {
  let hash = 0
  for (let i = 0; i < name.length; i++) hash = (hash << 5) - hash + name.charCodeAt(i)
  return palette[Math.abs(hash) % palette.length]
}

export function buildDeltaFoot(current: number, previous: number, label: string) {
  if (previous === 0) return current === 0 ? '无数据' : `${current}（${label}无可比数据）`
  const diff = current - previous
  const ratio = Math.abs((diff / previous) * 100).toFixed(1)
  if (diff === 0) return `0.0% 持平 ${label}`
  return `${diff > 0 ? '+' : '-'}${ratio}% ${diff > 0 ? '较' : '低于'}${label}`
}

export function formatSignedPercentPoint(value: number) {
  if (value === 0) return '0.0pp'
  return `${value > 0 ? '+' : '-'}${Math.abs(value).toFixed(1)}pp`
}

export function formatPercentValue(value: number, total: number) {
  if (total <= 0) return '0.0%'
  return `${((value / total) * 100).toFixed(1)}%`
}

export function toNonNegativeNumber(value: number | null | undefined): number {
  const parsed = Number(value) || 0
  return Math.max(0, parsed)
}

export function toNonNegativeInteger(value: number | null | undefined): number {
  return Math.round(toNonNegativeNumber(value))
}

export function normalizePercent(value: number | null | undefined): number {
  const parsed = Number(value) || 0
  const percent = parsed <= 1 ? parsed * 100 : parsed
  return Math.max(0, Math.min(100, percent))
}
