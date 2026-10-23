import type { ChartOptions } from '@/core/lib/chartPrimitives'
import { adminChartLegend, adminChartTheme } from '../../shared'

export const getDefaultModelColors = () => [
  adminChartTheme.brand,
  adminChartTheme.slate,
  adminChartTheme.cyan,
  adminChartTheme.olive,
  adminChartTheme.mauve,
  adminChartTheme.mist,
]

export function baseChartOptions(): ChartOptions {
  return {
    responsive: true,
    maintainAspectRatio: false,
    resizeDelay: 80,
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

function normalizeModelName(name: string | null | undefined): string {
  return (name || '').trim().toLowerCase()
}

function colorForModel(name: string, palette: string[]) {
  let hash = 0
  for (let i = 0; i < name.length; i++) hash = (hash << 5) - hash + name.charCodeAt(i)
  return palette[Math.abs(hash) % palette.length]
}

export function getModelColorByName(name: string) {
  const key = normalizeModelName(name)
  if (!key) return null
  return colorForModel(key, getDefaultModelColors())
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
  // Fail closed: non-finite input propagates as NaN (chart gap / `—` tooltip)
  // instead of a fabricated 0; only finite values are clamped to >= 0.
  if (value === null || value === undefined) return NaN
  const parsed = Number(value)
  if (!Number.isFinite(parsed)) return NaN
  return Math.max(0, parsed)
}

export function normalizePercent(value: number | null | undefined): number {
  // 后端一律 0-100；错量级必须可见（仅 clamp），不得静默 *100 猜测修复。
  // 非法输入透出 NaN（不断言为 0），由 tooltip 渲染为 `—`。
  if (value === null || value === undefined) return NaN
  const parsed = Number(value)
  if (!Number.isFinite(parsed)) return NaN
  return Math.max(0, Math.min(100, parsed))
}
