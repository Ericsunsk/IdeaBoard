import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { GetAdminStatisticsRange, type AdminStatistics } from './resource'
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
    expect(readDataset(charts.qualityTrendChartData.value, 'Success').data).toEqual(
      Array(7).fill(0),
    )
    expect(charts.bottleneckSourceChartData.value.labels).toEqual([])
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
    expect(readDataset(charts.trendChartData.value, 'seedream').data).toEqual([0, 0, 0, 0, 0, 0, 4])
    expect(charts.todayCallsFoot.value).toBe('+100.0% 较昨日')
    expect(charts.todayFailedFoot.value).toBe('-50.0% 低于昨日')
    expect(charts.todaySuccessRate.value).toBe('90.0%')
    expect(charts.todaySuccessRateFoot.value).toBe('+30.0pp 较昨日')
    expect(charts.monthCallsFoot.value).toBe('占总调用 20.0%')
    expect(trendTooltipAfterBody(charts.trendChartOptions.value, 6)).toEqual(['总调用: 7 次'])

    const trendOptions = charts.trendChartOptions.value as {
      responsive: boolean
      maintainAspectRatio: boolean
      scales: {
        x: {
          title: { display: boolean }
          ticks: { font: { family: string } }
        }
        y: {
          max: number
          title: { display: boolean }
          ticks: {
            callback: (value: number) => string
            font: { family: string }
          }
        }
      }
    }
    expect(trendOptions).toMatchObject({
      responsive: true,
      maintainAspectRatio: false,
      scales: {
        x: { title: { display: false } },
        y: { max: 10, title: { display: false } },
      },
    })
    expect(trendOptions.scales.y.ticks.callback(1_200)).toBe('1.2k')
    expect(trendOptions.scales.x.ticks.font.family).toContain('Segoe UI')
    expect(trendOptions.scales.y.ticks.font.family).toContain('Segoe UI')
    // 无 capture 数据：cost 系列与右轴都不出现，图表与之前一致。
    expect(hasDataset(charts.trendChartData.value, '消耗积分')).toBe(false)
  })
  it('keeps empty buckets at double zero instead of 100% failure', () => {
    const statistics = makeStatistics({
      trend: [],
      quality_trend: [],
    })
    const charts = useUsageStatisticsCharts({
      adminStatistics: ref(statistics),
      timeRange: ref(GetAdminStatisticsRange['7d']),
    })
    const success = readDataset(charts.qualityTrendChartData.value, 'Success')
    const errors = readDataset(charts.qualityTrendChartData.value, 'Errors')
    expect(success.data).toEqual(Array(7).fill(0))
    expect(errors.data).toEqual(Array(7).fill(0))
  })

  it('shows double zero for cancelled-only days (attempted denominator)', () => {
    const statistics = makeStatistics({
      quality_trend: [
        {
          date: '2026-07-06',
          label: '07/06',
          total: 5,
          success: 0,
          fail: 0,
          success_rate: 0,
        },
      ],
    })
    const charts = useUsageStatisticsCharts({
      adminStatistics: ref(statistics),
      timeRange: ref(GetAdminStatisticsRange['7d']),
    })
    const success = readDataset(charts.qualityTrendChartData.value, 'Success')
    const errors = readDataset(charts.qualityTrendChartData.value, 'Errors')
    expect(success.data).toEqual(Array(7).fill(0))
    expect(errors.data).toEqual(Array(7).fill(0))
  })

  it('represents empty segments as no series without a phantom dataset', () => {
    const statistics = makeStatistics({
      trend: [
        {
          date: '2026-07-06',
          label: '07/06',
          total: 0,
          segments: [],
        },
      ],
    })
    const charts = useUsageStatisticsCharts({
      adminStatistics: ref(statistics),
      timeRange: ref(GetAdminStatisticsRange['7d']),
    })

    expect(charts.trendChartData.value.datasets).toEqual([])
    expect(hasDataset(charts.trendChartData.value, '无数据')).toBe(false)
  })

  it('warns once when segments sum diverges from total without changing output', () => {
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    try {
      const statistics = makeStatistics({
        trend: [
          {
            date: '2026-07-04',
            label: '07/04',
            total: 10,
            segments: [{ label: 'gpt-image', value: 3 }],
          },
        ],
      })
      const charts = useUsageStatisticsCharts({
        adminStatistics: ref(statistics),
        timeRange: ref(GetAdminStatisticsRange['7d']),
      })

      // 渲染输出不变：系列取 segments 值，tooltip 总数取 total。
      expect(readDataset(charts.trendChartData.value, 'gpt-image').data).toEqual([
        0, 0, 0, 0, 3, 0, 0,
      ])
      expect(trendTooltipAfterBody(charts.trendChartOptions.value, 4)).toEqual(['总调用: 10 次'])
      expect(warnSpy).toHaveBeenCalledTimes(1)
      expect(warnSpy.mock.calls[0]?.[0]).toContain('2026-07-04')

      // 跨 re-render 去重：同 key 的第二次求值不再刷屏。
      const rerendered = useUsageStatisticsCharts({
        adminStatistics: ref(statistics),
        timeRange: ref(GetAdminStatisticsRange['7d']),
      })
      expect(rerendered.trendChartData.value.datasets).toHaveLength(1)
      expect(warnSpy).toHaveBeenCalledTimes(1)
    } finally {
      warnSpy.mockRestore()
    }
  })

  it('adds a cost series on its own axis when trend carries captured credits', () => {
    const statistics = makeStatistics({
      trend: [
        {
          date: '2026-07-05',
          label: '07/05',
          total: 4,
          captured_credits: 150,
          segments: [{ label: 'gpt-image', value: 4 }],
        },
        {
          date: '2026-07-06',
          label: '07/06',
          total: 7,
          captured_credits: 0,
          segments: [{ label: 'gpt-image', value: 7 }],
        },
      ],
    })
    const charts = useUsageStatisticsCharts({
      adminStatistics: ref(statistics),
      timeRange: ref(GetAdminStatisticsRange['7d']),
    })

    const cost = readDataset(charts.trendChartData.value, '消耗积分')
    expect(cost.yAxisID).toBe('cost')
    // 不进调用量堆叠：无 stack 键。
    expect('stack' in (cost as Record<string, unknown>)).toBe(false)
    expect(cost.data).toEqual([0, 0, 0, 0, 0, 150, 0])
    const options = charts.trendChartOptions.value as {
      scales: Record<string, { position?: string } | undefined>
    }
    expect(options.scales.cost?.position).toBe('right')
    const label = trendTooltipLabel(charts.trendChartOptions.value, 5, '消耗积分')
    expect(label).toBe('消耗积分: 150 积分')
  })

  it('reads quality trend from the single admin statistics stream', () => {
    const adminStatistics = makeStatistics({
      quality_trend: [
        {
          date: '2026-07-06',
          label: '07/06',
          total: 10,
          success: 6,
          fail: 4,
          // 后端 0-100 量级：失败率只能 100 - success_rate，不能用 fail/total 重算。
          success_rate: 60,
        },
      ],
    })

    const charts = useUsageStatisticsCharts({
      adminStatistics: ref(adminStatistics),
      timeRange: ref(GetAdminStatisticsRange['7d']),
    })
    expect(readDataset(charts.qualityTrendChartData.value, 'Success').data.at(-1)).toBe(60)
    expect(readDataset(charts.qualityTrendChartData.value, 'Errors').data.at(-1)).toBe(40)
    expect(qualityTooltipAfterBody(charts.qualityTrendChartOptions.value, 6)).toEqual([
      '总调用: 10',
      '成功: 6',
      '失败: 4',
    ])
  })

  it('normalizes bottleneck values before charting and tooltip output', () => {
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
  return dataset as { data: unknown[]; yAxisID?: string; stack?: string }
}

function hasDataset(chartData: { datasets: unknown[] }, label: string) {
  return chartData.datasets.some((item) => (item as { label?: string }).label === label)
}

function trendTooltipLabel(options: unknown, dataIndex: number, label: string) {
  return tooltipCallbacks(options).label?.({ dataIndex, dataset: { label } }) as string
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
