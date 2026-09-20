import { computed, type Ref } from 'vue'
import {
  defineChartData,
  type ChartData,
  type ChartOptions,
} from '../../../shared/lib/chartPrimitives'
import type { AdminStatistics } from './resource'
import {
  adminChartStrokeWidth,
  adminChartTheme,
  defineAdminChartRadialCountScale,
  defineAdminChartOptions,
} from '../shared/lib/chart'
import {
  baseChartOptions,
  baseChartPlugins,
  baseTooltipOptions,
  defaultModelColors,
  toNonNegativeNumber,
} from './usageStatisticsChartBase'
import { formatCount } from './usageStatisticsTooltipText'

export function useUsageStatisticsBreakdownCharts(adminStatistics: Ref<AdminStatistics | null>) {
  const bottleneckSourceRows = computed(() =>
    (adminStatistics.value?.bottleneck_sources || [])
      .map((item, idx) => ({
        source: item.source || 'unknown',
        label: item.label || bottleneckSourceFallbackLabel(item.source),
        total: toNonNegativeNumber(item.total),
        failed: toNonNegativeNumber(item.failed),
        avgLatencyMS: toNonNegativeNumber(item.avg_latency_ms),
        color: bottleneckSourceColor(item.source, idx),
      }))
      .filter((item) => item.total > 0)
      .sort((a, b) => b.total - a.total)
      .slice(0, 6),
  )

  const bottleneckSourceChartData = computed<ChartData>(() =>
    defineChartData({
      labels: bottleneckSourceRows.value.map((item) => item.label),
      datasets: [
        {
          type: 'polarArea',
          label: '任务量',
          data: bottleneckSourceRows.value.map((item) => item.total),
          backgroundColor: bottleneckSourceRows.value.map((item) => item.color),
          borderColor: adminChartTheme.segmentBorder,
          borderWidth: adminChartStrokeWidth,
          hoverOffset: 4,
        },
      ],
    }),
  )

  const bottleneckSourceChartOptions = computed<ChartOptions>(() =>
    defineAdminChartOptions({
      ...baseChartOptions(),
      plugins: {
        ...baseChartPlugins(),
        legend: { display: false },
        tooltip: {
          ...baseTooltipOptions(),
          callbacks: {
            label: (context) => {
              const point = bottleneckSourceRows.value[context.dataIndex]
              if (!point) return ''
              return [
                `任务: ${formatCount(point.total)}`,
                `失败: ${formatCount(point.failed)}`,
                `平均耗时: ${(point.avgLatencyMS / 1000).toFixed(2)}s`,
              ]
            },
          },
        },
      },
      scales: {
        r: {
          ...defineAdminChartRadialCountScale({
            values: bottleneckSourceRows.value.map((row) => row.total),
            min: 5,
            headroom: 1.1,
          }),
        },
      },
    } as ChartOptions),
  )

  return {
    bottleneckSourceChartData,
    bottleneckSourceChartOptions,
  }
}

function bottleneckSourceFallbackLabel(source: string | undefined) {
  const labels: Record<string, string> = {
    queue: '队列等待',
    worker: 'Worker 调度',
    provider: '上游模型',
    storage: '存储链路',
    finalize: '结果落库',
    input: '输入校验',
    unknown: '未知来源',
  }
  return labels[source || 'unknown'] ?? labels.unknown ?? '未知来源'
}

function bottleneckSourceColor(source: string | undefined, index: number) {
  const colors: Record<string, string> = {
    queue: adminChartTheme.cyan,
    worker: adminChartTheme.slate,
    provider: adminChartTheme.brand,
    storage: adminChartTheme.danger,
    finalize: adminChartTheme.olive,
    input: adminChartTheme.warning,
    unknown: adminChartTheme.mist,
  }
  return (
    colors[source || 'unknown'] ??
    defaultModelColors[index % defaultModelColors.length] ??
    adminChartTheme.mist
  )
}
