import { computed, type Ref } from 'vue'
import type { AdminStatistics } from './statisticsResource'
import {
  adminChartTheme,
  defineAdminChartBubbleDatasetStyle,
  defineAdminChartBubblePointStyle,
  defineAdminChartData,
  defineAdminChartDurationScale,
  defineAdminChartOptions,
  defineAdminChartPercentScale,
  getAdminChartBubbleHoverRadius,
  getAdminChartBubbleRadius,
  type AdminChartData,
  type AdminChartOptions,
  type AdminTooltipItem,
} from '../shared/lib/adminChart'
import {
  baseChartOptions,
  baseChartPlugins,
  baseTooltipOptions,
  colorForModel,
  defaultModelColors,
  normalizeModelName,
  toNonNegativeInteger,
  toNonNegativeNumber,
} from './usageStatisticsChartBase'

type ModelRiskPoint = {
  x: number
  y: number
  name: string
  total: number
  failed: number
  avgSeconds: number
  failureRate: number
}

type SlowModelPoint = {
  x: number
  y: number
  r: number
  name: string
  p95Seconds: number
  avgSeconds: number
  samples: number
}

export function useUsageStatisticsModelHealthCharts(
  adminStatistics: Ref<AdminStatistics | null>,
) {
  function getModelColorByName(name: string) {
    const key = normalizeModelName(name)
    if (!key) return null
    return colorForModel(key, defaultModelColors)
  }

  const modelRiskRows = computed(() =>
    (adminStatistics.value?.model_health || [])
      .filter((item) => item.total > 0)
      .map((item, idx) => ({
        name: item.name === 'Unknown' ? '未识别模型' : item.name,
        total: item.total,
        failed: item.failed,
        failureRate: Math.max(0, Math.min(100, item.failure_rate || 0)),
        avgSeconds: Math.max(0, item.avg_seconds || 0),
        color:
          getModelColorByName(item.name) ||
          defaultModelColors[idx % defaultModelColors.length] ||
          adminChartTheme.brand,
      }))
      .slice(0, 12),
  )

  const modelRiskChartData = computed<AdminChartData>(() => {
    const maxTotal = Math.max(1, ...modelRiskRows.value.map((item) => item.total))
    const colors = modelRiskRows.value.map((item) => item.color)
    const points = modelRiskRows.value.map<ModelRiskPoint>((item) => ({
      x: item.avgSeconds,
      y: item.failureRate,
      name: item.name,
      total: item.total,
      failed: item.failed,
      avgSeconds: item.avgSeconds,
      failureRate: item.failureRate,
    }))
    return defineAdminChartData({
      labels: modelRiskRows.value.map((item) => item.name),
      datasets: [
        {
          type: 'scatter',
          label: '模型风险',
          data: points,
          pointRadius: modelRiskRows.value.map((item) =>
            getAdminChartBubbleRadius(item.total, maxTotal),
          ),
          pointHoverRadius: modelRiskRows.value.map((item) =>
            getAdminChartBubbleHoverRadius(item.total, maxTotal),
          ),
          ...defineAdminChartBubblePointStyle(colors),
        },
      ],
    })
  })

  const modelRiskChartOptions = computed<AdminChartOptions>(() =>
    defineAdminChartOptions({
      ...baseChartOptions(),
      interaction: { mode: 'nearest', intersect: true },
      plugins: {
        legend: { display: false },
        tooltip: {
          ...baseTooltipOptions(),
          callbacks: {
            title: (items) => getModelRiskPoint(items[0])?.name || '未知模型',
            label: (context) => {
              const point = getModelRiskPoint(context)
              if (!point) return ''
              return `失败率: ${point.failureRate.toFixed(1)}%`
            },
            afterLabel: (context) => {
              const point = getModelRiskPoint(context)
              if (!point) return []
              return [
                `平均耗时: ${point.avgSeconds.toFixed(2)}s`,
                `调用量: ${point.total.toLocaleString()}`,
                `失败: ${point.failed.toLocaleString()}`,
              ]
            },
          },
        },
      },
      scales: {
        x: {
          ...defineAdminChartDurationScale({
            values: modelRiskRows.value.map((row) => row.avgSeconds),
            min: 1,
            headroom: 1.15,
          }),
        },
        y: {
          ...defineAdminChartPercentScale(),
        },
      },
    }),
  )

  const slowModelRows = computed(() =>
    (adminStatistics.value?.slow_models || [])
      .map((item, idx) => {
        const modelName = item.name || '未知模型'
        const p95Seconds = toNonNegativeNumber(item.p95_seconds)
        const avgSeconds = toNonNegativeNumber(item.avg_seconds)
        const samples = toNonNegativeInteger(item.samples)
        return {
          name: modelName,
          p95Seconds,
          avgSeconds,
          samples,
          color:
            getModelColorByName(modelName) ||
            defaultModelColors[idx % defaultModelColors.length] ||
            adminChartTheme.brand,
        }
      })
      .sort((a, b) => b.p95Seconds - a.p95Seconds)
      .slice(0, 6),
  )

  const slowModelsChartData = computed<AdminChartData>(() => {
    const maxSamples = Math.max(1, ...slowModelRows.value.map((row) => row.samples))
    const colors = slowModelRows.value.map((row) => row.color)

    return defineAdminChartData({
      labels: slowModelRows.value.map((row) => row.name),
      datasets: [
        {
          type: 'bubble',
          label: '模型尾延迟',
          data: slowModelRows.value.map<SlowModelPoint>((row) => ({
            x: row.p95Seconds,
            y: row.avgSeconds,
            r: getAdminChartBubbleRadius(row.samples, maxSamples),
            name: row.name,
            p95Seconds: row.p95Seconds,
            avgSeconds: row.avgSeconds,
            samples: row.samples,
          })),
          ...defineAdminChartBubbleDatasetStyle(colors),
        },
      ],
    })
  })

  const slowModelsChartOptions = computed<AdminChartOptions>(() =>
    defineAdminChartOptions({
      ...baseChartOptions(),
      layout: { padding: { top: 10, right: 18 } },
      plugins: {
        ...baseChartPlugins(),
        legend: { display: false },
        tooltip: {
          ...baseTooltipOptions(),
          callbacks: {
            title: (items) => getSlowModelPoint(items[0])?.name || '',
            label: (context) => {
              const point = getSlowModelPoint(context)
              if (!point) return []
              return [
                `P95: ${point.p95Seconds.toFixed(2)}s`,
                `平均: ${point.avgSeconds.toFixed(2)}s`,
                `样本: ${point.samples.toLocaleString()}`,
              ]
            },
          },
        },
      },
      scales: {
        x: {
          ...defineAdminChartDurationScale({
            values: slowModelRows.value.map((row) => row.p95Seconds),
            min: 1,
            headroom: 1.15,
            borderColor: adminChartTheme.axisLine,
          }),
        },
        y: {
          ...defineAdminChartDurationScale({
            values: slowModelRows.value.map((row) => row.avgSeconds),
            min: 1,
            headroom: 1.15,
          }),
        },
      },
    }),
  )

  return {
    modelRiskChartData,
    modelRiskChartOptions,
    slowModelsChartData,
    slowModelsChartOptions,
  }
}

function getModelRiskPoint(context: AdminTooltipItem | undefined) {
  return context?.raw as ModelRiskPoint | undefined
}

function getSlowModelPoint(context: AdminTooltipItem | undefined) {
  return context?.raw as SlowModelPoint | undefined
}
