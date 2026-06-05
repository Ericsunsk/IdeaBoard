import type { Chart, ChartData, ChartOptions, ChartType, Plugin, TooltipItem } from 'chart.js'
import { markRaw, nextTick, onBeforeUnmount, ref, watch, type Ref } from 'vue'

export type AdminChartData = ChartData<ChartType, unknown[], string>
export type AdminChartOptions = ChartOptions<ChartType>
export type AdminTooltipItem = TooltipItem<ChartType>

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

export function defineAdminChartData(data: AdminChartData): AdminChartData {
  return markRaw(data)
}

export function defineAdminChartOptions(options: AdminChartOptions): AdminChartOptions {
  return markRaw(options)
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
