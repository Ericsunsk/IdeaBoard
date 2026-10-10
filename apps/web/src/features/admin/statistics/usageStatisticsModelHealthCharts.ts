import { computed, type Ref } from 'vue'
import {
  defineChartData,
  type ChartData,
  type ChartOptions,
  type ChartTooltipItem,
} from '../../../core/lib/chartPrimitives'
import type { AdminStatistics } from './resource'
import {
  adminChartTheme,
  defineAdminChartBubblePointStyle,
  defineAdminChartDurationScale,
  defineAdminChartPercentScale,
  defineAdminChartOptions,
  getAdminChartBubbleHoverRadius,
  getAdminChartBubbleRadius,
} from '../shared/lib/chart'
import {
  baseChartOptions,
  baseTooltipOptions,
  defaultModelColors,
  getModelColorByName,
} from './usageStatisticsChartBase'
import { formatCount, formatPercent1 } from './usageStatisticsTooltipText'

type ModelRiskPoint = {
  x: number
  y: number
  name: string
  total: number
  failed: number
  avgSeconds: number
  failureRate: number
}

export function useUsageStatisticsModelHealthCharts(adminStatistics: Ref<AdminStatistics | null>) {
  const modelRiskRows = computed(() =>
    (adminStatistics.value?.model_health || [])
      .filter((item) => item.total > 0)
      .map((item, idx) => ({
        name: item.name === 'Unknown' ? '未识别模型' : item.name,
        total: item.total,
        failed: item.failed,
        // 后端修正必须可见：只保留 || 0 防 NaN/null 击穿 Chart.js scale，不做 0-100 / max(0) 改写。
        failureRate: item.failure_rate || 0,
        avgSeconds: item.avg_seconds || 0,
        color:
          getModelColorByName(item.name) ||
          defaultModelColors[idx % defaultModelColors.length] ||
          adminChartTheme.brand,
      }))
      .slice(0, 12),
  )

  const modelRiskChartData = computed<ChartData>(() => {
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
    return defineChartData({
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

  const modelRiskChartOptions = computed<ChartOptions>(() =>
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
              return `失败率: ${formatPercent1(point.failureRate)}`
            },
            afterLabel: (context) => {
              const point = getModelRiskPoint(context)
              if (!point) return []
              return [
                `平均耗时: ${point.avgSeconds.toFixed(2)}s`,
                `调用量: ${formatCount(point.total)}`,
                `失败: ${formatCount(point.failed)}`,
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

  return {
    modelRiskChartData,
    modelRiskChartOptions,
  }
}

function getModelRiskPoint(context: ChartTooltipItem | undefined) {
  return context?.raw as ModelRiskPoint | undefined
}
