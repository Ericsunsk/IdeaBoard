import { computed, type Ref } from 'vue'
import {
  defineChartData,
  getParsedY,
  type ChartData,
  type ChartOptions,
  withAlpha,
} from '../../../shared/lib/chartPrimitives'
import type { AdminStatistics, GetAdminStatisticsRangeValue } from './resource'
import {
  adminChartStrokeWidth,
  adminChartTheme,
  defineAdminChartPercentScale,
  defineAdminChartPointClipStyle,
  defineAdminChartOptions,
} from '../shared/lib/chart'
import { buildAdminTimeRangeBuckets, shouldShowAdminTimeRangeTick } from '../shared/lib/timeRange'
import {
  baseChartOptions,
  baseChartPlugins,
  baseTooltipOptions,
  normalizePercent,
  toNonNegativeNumber,
} from './usageStatisticsChartBase'
import { formatCount, formatPercent1 } from './usageStatisticsTooltipText'
import { monitorChartSpecs, taskMetricDefinitions } from '../shared/lib/taskMetricTokens'

// 系列命名与颜色复用任务监控的状态定义（completed/failed），单源，改名只改一处。
function qualitySeriesStyle(key: 'completed' | 'failed') {
  const metric = taskMetricDefinitions.find((item) => item.key === key)
  return {
    label: metric?.chartLabel ?? (key === 'completed' ? '成功率' : '失败率'),
    color:
      metric?.chartColor ??
      (key === 'completed' ? adminChartTheme.success : adminChartTheme.danger),
  }
}

export function useUsageStatisticsQualityTrendCharts(options: {
  adminStatistics: Ref<AdminStatistics | null>
  timeRange: Ref<GetAdminStatisticsRangeValue>
}) {
  const qualityTrendSource = computed(() => options.adminStatistics.value)
  const qualityTrendRows = computed(() => {
    const trend = qualityTrendSource.value?.quality_trend || []
    const trendMap = new Map(trend.map((item) => [item.date, item]))

    return buildAdminTimeRangeBuckets(options.timeRange.value).map((bucket) => {
      const item = trendMap.get(bucket.date)
      if (item == null) {
        // Sparse backend series: a missing day means no activity, not broken
        // data — render an explicit zero day (failureRate 0, not 100%).
        // Present-but-invalid values below propagate NaN instead (no fake 0).
        return {
          label: bucket.label,
          total: 0,
          success: 0,
          fail: 0,
          successRate: 0,
          failureRate: 0,
        }
      }
      const total = toNonNegativeNumber(item?.total)
      const success = toNonNegativeNumber(item?.success)
      const fail = toNonNegativeNumber(item?.fail)
      // success_rate 后端已是 0-100（分母 success+fail）；失败率同分母，只能 100 - success_rate，不能用 fail/total 重算。
      const successRate = normalizePercent(item?.success_rate)
      const attempted = success + fail
      return {
        label: item?.label || bucket.label,
        total,
        success,
        fail,
        successRate,
        // 后端分母是 success+fail（total 还含 cancelled）：只看 attempted，
        // 全取消的天显示双零而非 100% 失败。非法输入透出 NaN，不断言为 0。
        failureRate:
          !Number.isFinite(attempted) || !Number.isFinite(successRate)
            ? NaN
            : attempted > 0
              ? 100 - successRate
              : 0,
      }
    })
  })

  const qualityTrendChartData = computed<ChartData>(() =>
    defineChartData({
      labels: qualityTrendRows.value.map((item) => item.label),
      datasets: (['completed', 'failed'] as const).map((key) => {
        const style = qualitySeriesStyle(key)
        const isSuccess = key === 'completed'
        return {
          type: 'line' as const,
          label: style.label,
          data: qualityTrendRows.value.map((item) =>
            isSuccess ? item.successRate : item.failureRate,
          ),
          borderColor: style.color,
          backgroundColor: withAlpha(style.color, isSuccess ? 0.12 : 0.08),
          borderWidth: adminChartStrokeWidth,
          ...defineAdminChartPointClipStyle(),
          fill: true,
          tension: 0.36,
          // 线上不画点：hover 才显示；点的实心样式与 hover 大小对齐任务监控。
          pointRadius: 0,
          pointHoverRadius: monitorChartSpecs.pointHoverRadius,
          pointBackgroundColor: style.color,
          pointBorderColor: 'transparent',
          pointBorderWidth: 0,
          yAxisID: 'y',
        }
      }),
    }),
  )

  const qualityTrendChartOptions = computed<ChartOptions>(() =>
    defineAdminChartOptions({
      ...baseChartOptions(),
      interaction: { mode: 'index', intersect: false },
      plugins: {
        ...baseChartPlugins(),
        tooltip: {
          ...baseTooltipOptions(),
          callbacks: {
            label: (context) => {
              const label = context.dataset.label || ''
              const value = getParsedY(context)
              return `${label}: ${formatPercent1(value)}`
            },
            afterBody: (items) => {
              const index = items[0]?.dataIndex ?? -1
              const row = qualityTrendRows.value[index]
              if (!row) return []
              return [
                `总调用: ${formatCount(row.total)}`,
                `成功: ${formatCount(row.success)}`,
                `失败: ${formatCount(row.fail)}`,
              ]
            },
          },
        },
      },
      scales: {
        x: {
          ticks: {
            color: adminChartTheme.axisText,
            maxRotation: 0,
            autoSkip: false,
            callback: (_value, index) =>
              shouldShowAdminTimeRangeTick({
                index,
                count: qualityTrendRows.value.length,
                range: options.timeRange.value,
                label: qualityTrendRows.value[index]?.label,
              })
                ? qualityTrendRows.value[index]?.label || ''
                : '',
          },
          grid: { display: false },
          border: { color: adminChartTheme.axisLine },
        },
        y: {
          ...defineAdminChartPercentScale(),
        },
      },
    }),
  )

  return {
    qualityTrendChartData,
    qualityTrendChartOptions,
  }
}
