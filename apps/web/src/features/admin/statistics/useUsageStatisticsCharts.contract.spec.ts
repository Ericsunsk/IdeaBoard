import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { GetAdminStatisticsRange, type AdminStatistics } from './statisticsResource'
import { useUsageStatisticsCharts } from './useUsageStatisticsCharts'

describe('useUsageStatisticsCharts', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date(2026, 6, 6, 12))
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('keeps null statistics as stable zero and empty chart view-models', () => {
    const charts = useUsageStatisticsCharts({
      adminStatistics: ref(null),
      timeRange: ref(GetAdminStatisticsRange['7d']),
    })

    expect(charts.trendChartData.value.labels).toHaveLength(7)
    expect(charts.trendChartData.value.datasets).toEqual([])
    expect(charts.todayCalls.value).toBe(0)
    expect(charts.todayFailed.value).toBe(0)
    expect(charts.todayCallsFoot.value).toBe('无数据')
    expect(charts.todayFailedFoot.value).toBe('无数据')
    expect(charts.todaySuccessRate.value).toBe('0.0%')
    expect(charts.todaySuccessRateFoot.value).toBe('昨日无可比数据')
    expect(charts.monthCallsFoot.value).toBe('累计无调用')
    expect(readDataset(charts.qualityTrendChartData.value, '成功率').data).toEqual(
      Array(7).fill(0),
    )
    expect(charts.bottleneckSourceChartData.value.labels).toEqual([])
    expect(charts.failureCategoryChartData.value.labels).toEqual([])
  })

  it('builds sparse usage trend series and summary footers from the selected range', () => {
    const statistics = makeStatistics({
      summary: {
        total_calls: 100,
        today_calls: 10,
        today_success: 9,
        today_failed: 1,
        today_cancelled: 0,
        yesterday_calls: 5,
        yesterday_success: 3,
        yesterday_failed: 2,
        yesterday_cancelled: 0,
        this_month_calls: 20,
        this_month_cancelled: 0,
        pending: 0,
        running: 0,
        completed: 0,
        failed: 0,
        cancelled: 0,
      },
      trend: [
        {
          date: '2026-06-30',
          label: '06/30',
          total: 2,
          segments: [{ label: 'gpt-image', value: 2 }],
        },
        {
          date: '2026-07-06',
          label: '07/06',
          total: 7,
          segments: [
            { label: 'gpt-image', value: 3 },
            { label: 'seedream', value: 4 },
          ],
        },
      ],
    })

    const charts = useUsageStatisticsCharts({
      adminStatistics: ref(statistics),
      timeRange: ref(GetAdminStatisticsRange['7d']),
    })

    expect(charts.trendChartData.value.labels).toEqual([
      '06/30',
      '07/01',
      '07/02',
      '07/03',
      '07/04',
      '07/05',
      '07/06',
    ])
    expect(readDataset(charts.trendChartData.value, 'gpt-image').data).toEqual([
      2, 0, 0, 0, 0, 0, 3,
    ])
    expect(readDataset(charts.trendChartData.value, 'seedream').data).toEqual([
      0, 0, 0, 0, 0, 0, 4,
    ])
    expect(charts.todayCallsFoot.value).toBe('+100.0% 较昨日')
    expect(charts.todayFailedFoot.value).toBe('-50.0% 低于昨日')
    expect(charts.todaySuccessRate.value).toBe('90.0%')
    expect(charts.todaySuccessRateFoot.value).toBe('+30.0pp 较昨日')
    expect(charts.monthCallsFoot.value).toBe('占总调用 20.0%')
    expect(trendTooltipAfterBody(charts.trendChartOptions.value, 6)).toEqual(['总调用: 7 次'])
  })

  it('uses quality statistics only when provided or when the quality range matches usage', () => {
    const adminStatistics = makeStatistics({
      quality_trend: [
        {
          date: '2026-07-06',
          label: '07/06',
          total: 10,
          success: 6,
          fail: 4,
          success_rate: 0.6,
        },
      ],
    })
    const qualityStatistics = makeStatistics({
      quality_trend: [
        {
          date: '2026-07-06T08',
          label: '08:00',
          total: 10,
          success: 8,
          fail: 2,
          success_rate: 80,
        },
      ],
    })

    const sharedRangeCharts = useUsageStatisticsCharts({
      adminStatistics: ref(adminStatistics),
      timeRange: ref(GetAdminStatisticsRange['7d']),
      qualityTimeRange: ref(GetAdminStatisticsRange['7d']),
    })
    expect(readDataset(sharedRangeCharts.qualityTrendChartData.value, '成功率').data.at(-1)).toBe(
      60,
    )
    expect(readDataset(sharedRangeCharts.qualityTrendChartData.value, '失败率').data.at(-1)).toBe(
      40,
    )

    const unmatchedRangeCharts = useUsageStatisticsCharts({
      adminStatistics: ref(adminStatistics),
      qualityStatistics: ref(null),
      timeRange: ref(GetAdminStatisticsRange['7d']),
      qualityTimeRange: ref(GetAdminStatisticsRange['1d']),
    })
    expect(readDataset(unmatchedRangeCharts.qualityTrendChartData.value, '成功率').data).toEqual(
      Array(6).fill(0),
    )

    const providedQualityCharts = useUsageStatisticsCharts({
      adminStatistics: ref(adminStatistics),
      qualityStatistics: ref(qualityStatistics),
      timeRange: ref(GetAdminStatisticsRange['7d']),
      qualityTimeRange: ref(GetAdminStatisticsRange['1d']),
    })
    expect(readDataset(providedQualityCharts.qualityTrendChartData.value, '成功率').data).toEqual([
      0, 0, 80, 0, 0, 0,
    ])
    expect(readDataset(providedQualityCharts.qualityTrendChartData.value, '失败率').data).toEqual([
      0, 0, 20, 0, 0, 0,
    ])
    expect(qualityTooltipAfterBody(providedQualityCharts.qualityTrendChartOptions.value, 2)).toEqual(
      ['总调用: 10', '成功: 8', '失败: 2'],
    )
  })

  it('normalizes bottleneck and failure category values before charting and tooltip output', () => {
    const charts = useUsageStatisticsCharts({
      adminStatistics: ref(
        makeStatistics({
          bottleneck_sources: [
            {
              source: 'storage',
              label: 'should be filtered',
              total: -2,
              failed: -1,
              avg_latency_ms: -100,
            },
            {
              source: 'provider',
              label: '',
              total: 5,
              failed: -1,
              avg_latency_ms: 2500,
            },
          ],
          failure_categories: [
            {
              category: 'timeout',
              label: 'should be filtered',
              total: -1,
              avg_latency_ms: -200,
            },
            {
              category: 'storage_error',
              label: '',
              total: 3,
              avg_latency_ms: 1200,
            },
          ],
        }),
      ),
      timeRange: ref(GetAdminStatisticsRange['7d']),
    })

    expect(charts.bottleneckSourceChartData.value.labels).toEqual(['上游模型'])
    expect(readDataset(charts.bottleneckSourceChartData.value, '任务量').data).toEqual([5])
    expect(bottleneckTooltipLabel(charts.bottleneckSourceChartOptions.value, 0)).toEqual([
      '任务: 5',
      '失败: 0',
      '平均耗时: 2.50s',
    ])

    expect(charts.failureCategoryChartData.value.labels).toEqual(['存储错误'])
    expect(readDataset(charts.failureCategoryChartData.value, '失败量').data).toEqual([3])
    expect(failureTooltipLabel(charts.failureCategoryChartOptions.value, 0)).toEqual([
      '失败: 3',
      '占比: 100.0%',
      '平均耗时: 1.20s',
    ])
  })
})

function makeStatistics(overrides: Partial<AdminStatistics> = {}): AdminStatistics {
  const base: AdminStatistics = {
    summary: {
      total_calls: 0,
      today_calls: 0,
      today_success: 0,
      today_failed: 0,
      today_cancelled: 0,
      yesterday_calls: 0,
      yesterday_success: 0,
      yesterday_failed: 0,
      yesterday_cancelled: 0,
      this_month_calls: 0,
      this_month_cancelled: 0,
      pending: 0,
      running: 0,
      completed: 0,
      failed: 0,
      cancelled: 0,
      ...overrides.summary,
    },
    trend: [],
    model_health: [],
    quality_trend: [],
    slow_models: [],
    bottleneck_sources: [],
    failure_categories: [],
    backfill_status: {
      range_start: '',
      range_end: '',
      missing_days: 0,
      backfilled_days: 0,
    },
  }

  return {
    ...base,
    ...overrides,
    summary: {
      ...base.summary,
      ...overrides.summary,
    },
    backfill_status: {
      ...base.backfill_status,
      ...overrides.backfill_status,
    },
  }
}

function readDataset(chartData: { datasets: unknown[] }, label: string) {
  const dataset = chartData.datasets.find((item) => (item as { label?: string }).label === label)
  if (!dataset) throw new Error(`missing dataset: ${label}`)
  return dataset as { data: unknown[] }
}

function trendTooltipAfterBody(options: unknown, dataIndex: number) {
  return tooltipCallbacks(options).afterBody?.([
    { dataIndex, dataset: { label: 'gpt-image' } },
  ]) as string[]
}

function qualityTooltipAfterBody(options: unknown, dataIndex: number) {
  return tooltipCallbacks(options).afterBody?.([{ dataIndex }]) as string[]
}

function bottleneckTooltipLabel(options: unknown, dataIndex: number) {
  return tooltipCallbacks(options).label?.({ dataIndex }) as string[]
}

function failureTooltipLabel(options: unknown, dataIndex: number) {
  return tooltipCallbacks(options).label?.({ dataIndex }) as string[]
}

function tooltipCallbacks(options: unknown) {
  return (
    options as {
      plugins: {
        tooltip: {
          callbacks: Record<string, (context: unknown) => unknown>
        }
      }
    }
  ).plugins.tooltip.callbacks
}
