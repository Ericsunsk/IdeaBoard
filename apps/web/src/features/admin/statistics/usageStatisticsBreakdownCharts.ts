import { computed, type Ref } from 'vue'
import {
  defineChartData,
  type ChartData,
  type ChartOptions,
  withAlpha,
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
                `任务: ${point.total.toLocaleString()}`,
                `失败: ${point.failed.toLocaleString()}`,
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

  const failureCategoryRows = computed(() => {
    const rows = (adminStatistics.value?.failure_categories || [])
      .map((item, idx) => ({
        category: item.category || 'unknown',
        label: item.label || failureCategoryFallbackLabel(item.category),
        total: toNonNegativeNumber(item.total),
        avgLatencyMS: toNonNegativeNumber(item.avg_latency_ms),
        color: failureCategoryColor(item.category, idx),
      }))
      .filter((item) => item.total > 0)
      .sort((a, b) => b.total - a.total)
      .slice(0, 6)
    const totalFailures = rows.reduce((sum, item) => sum + item.total, 0)
    return rows.map((item) => ({
      ...item,
      sharePercent: totalFailures > 0 ? (item.total / totalFailures) * 100 : 0,
    }))
  })

  const failureCategoryChartData = computed<ChartData>(() =>
    defineChartData({
      labels: failureCategoryRows.value.map((item) => item.label),
      datasets: [
        {
          type: 'polarArea',
          label: '失败量',
          data: failureCategoryRows.value.map((item) => item.total),
          backgroundColor: failureCategoryRows.value.map((item) => withAlpha(item.color, 0.62)),
          borderColor: failureCategoryRows.value.map((item) => item.color),
          borderWidth: adminChartStrokeWidth,
        },
      ],
    }),
  )

  const failureCategoryChartOptions = computed<ChartOptions>(() =>
    defineAdminChartOptions({
      ...baseChartOptions(),
      plugins: {
        ...baseChartPlugins(),
        legend: { display: false },
        tooltip: {
          ...baseTooltipOptions(),
          callbacks: {
            label: (context) => {
              const point = failureCategoryRows.value[context.dataIndex]
              if (!point) return ''
              return [
                `失败: ${point.total.toLocaleString()}`,
                `占比: ${point.sharePercent.toFixed(1)}%`,
                `平均耗时: ${(point.avgLatencyMS / 1000).toFixed(2)}s`,
              ]
            },
          },
        },
      },
      scales: {
        r: {
          ...defineAdminChartRadialCountScale({
            values: failureCategoryRows.value.map((row) => row.total),
            min: 5,
            headroom: 1.1,
          }),
        },
      },
    }),
  )

  return {
    bottleneckSourceChartData,
    bottleneckSourceChartOptions,
    failureCategoryChartData,
    failureCategoryChartOptions,
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

function failureCategoryFallbackLabel(category: string | undefined) {
  const labels: Record<string, string> = {
    input_invalid: '输入无效',
    auth: '认证失败',
    quota: '额度限制',
    timeout: '执行超时',
    provider_error: '上游错误',
    storage_error: '存储错误',
    finalize_error: '结果落库',
    cancelled: '任务取消',
    unknown: '未知错误',
  }
  return labels[category || 'unknown'] ?? labels.unknown ?? '未知错误'
}

function failureCategoryColor(category: string | undefined, index: number) {
  const colors: Record<string, string> = {
    input_invalid: adminChartTheme.warning,
    auth: adminChartTheme.mauve,
    quota: adminChartTheme.olive,
    timeout: adminChartTheme.danger,
    provider_error: adminChartTheme.brand,
    storage_error: adminChartTheme.cyan,
    finalize_error: adminChartTheme.slate,
    cancelled: adminChartTheme.mist,
    unknown: adminChartTheme.emptyText,
  }
  return (
    colors[category || 'unknown'] ??
    defaultModelColors[index % defaultModelColors.length] ??
    adminChartTheme.emptyText
  )
}
