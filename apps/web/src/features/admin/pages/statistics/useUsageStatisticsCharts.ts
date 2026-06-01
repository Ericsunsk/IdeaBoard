import type { EChartsOption } from 'echarts'
import type { Ref } from 'vue'

type StatisticsLike = {
  summary?: {
    total_calls?: number
    today_calls?: number
    today_success?: number
    today_failed?: number
    yesterday_calls?: number
    yesterday_success?: number
    yesterday_failed?: number
  }
  trend?: Array<{
    date: string
    label: string
    total: number
    segments: Array<{ label: string; value: number }>
  }>
  model_health?: Array<{
    name: string
    total: number
    completed: number
    failed: number
    failure_rate: number
    avg_seconds: number
  }>
  quality_trend?: Array<{
    date: string
    label: string
    total: number
    success: number
    fail: number
    success_rate: number
  }>
  slow_models?: Array<{
    name: string
    avg_seconds: number
    p95_seconds: number
    samples: number
  }>
  user_risk?: Array<{
    name: string
    total: number
    completed: number
    failed: number
    failure_rate: number
  }>
}
type MonitorLike = {
  global_queue_len: number
  wait_queue_len: number
  running_tasks: number
  account_loads: Record<number, { current_concurrent: number; max_concurrent: number }>
}

const adminChartTheme = {
  tooltipBg: '#f8fafc',
  tooltipBorder: '#d9e2ef',
  tooltipText: '#0f172a',
  tooltipMuted: '#64748b',
  axisLine: '#d5deea',
  axisText: '#6e7487',
  splitLine: '#edf2f7',
  legendText: '#475569',
  legendInactive: '#cbd5e1',
  emptyText: '#6e7487',
  emptyStroke: '#c8d0e4',
  cardBg: '#f8fafc',
  mainText: '#293042',
  shadow: 'rgba(41,48,66,0.18)',
}

export function useUsageStatisticsCharts(options: {
  adminStatistics: Ref<StatisticsLike | null>
  timeRange: Ref<string>
  monitorData: Ref<MonitorLike | null>
}) {
  const defaultModelColors = ['#818cf8', '#c084fc', '#22d3ee', '#14b8a6', '#f59e0b', '#ef4444']

  function getModelColorByName(name: string) {
    const key = normalizeModelName(name)
    if (!key) return null
    return colorForModel(key, defaultModelColors)
  }

  const trendData = computed(() => {
    const days = options.timeRange.value === '7d' ? 7 : options.timeRange.value === '30d' ? 30 : 365
    const now = new Date()
    const trend = options.adminStatistics.value?.trend || []
    const trendMap = new Map(trend.map((day) => [day.date, day]))
    const fallback = Array.from({ length: days }, (_, idx) => {
      const d = new Date(now)
      d.setDate(now.getDate() - (days - 1 - idx))
      const date = `${d.getFullYear()}-${(d.getMonth() + 1).toString().padStart(2, '0')}-${d.getDate().toString().padStart(2, '0')}`
      const label = `${(d.getMonth() + 1).toString().padStart(2, '0')}/${d.getDate().toString().padStart(2, '0')}`
      return trendMap.get(date) || { date, label, total: 0, segments: [] }
    })
    return fallback.map((day) => {
      const segments = day.segments.map((seg, idx) => {
        const color = getModelColorByName(seg.label)
        return {
          label: seg.label,
          value: seg.value,
          color: color || defaultModelColors[idx % defaultModelColors.length],
        }
      })
      if (segments.length === 0)
        segments.push({ label: '无数据', value: 0, color: adminChartTheme.emptyStroke })
      return { date: day.label || day.date, total: day.total, segments }
    })
  })

  const totalCallsInRange = computed(() => trendData.value.reduce((sum, d) => sum + d.total, 0))

  const trendSeriesModelOrder = computed(() => {
    const byModel = new Map<string, { color: string; total: number }>()
    trendData.value.forEach((day) => {
      day.segments.forEach((segment, idx) => {
        const existing = byModel.get(segment.label)
        if (existing) {
          existing.total += segment.value
          return
        }
        byModel.set(segment.label, {
          color: segment.color || defaultModelColors[idx % defaultModelColors.length],
          total: segment.value,
        })
      })
    })
    return Array.from(byModel.entries())
      .filter(([label, meta]) => label !== '无数据' || meta.total > 0)
      .map(([label, meta]) => ({ label, color: meta.color }))
  })

  function shouldShowTrendLabel(index: number) {
    const count = trendData.value.length
    if (count <= 7) return true
    if (options.timeRange.value === '30d')
      return index === 0 || index === count - 1 || index % 6 === 0
    if (options.timeRange.value === '1y') {
      if (index === 0 || index === count - 1) return true
      const day = trendData.value[index]
      if (!day) return false
      return isMonthStartLabel(day.date)
    }
    return true
  }

  const trendChartOption = computed<EChartsOption>(() => {
    const categories = trendData.value.map((day) => day.date)
    const hasAnyCalls = trendData.value.some((day) => day.total > 0)

    const series = trendSeriesModelOrder.value.map((model) => ({
      name: model.label,
      type: 'line' as const,
      smooth: true,
      stack: 'usage',
      symbol: trendData.value.length <= 1 ? 'circle' : 'none',
      symbolSize: 6,
      showSymbol: trendData.value.length <= 1,
      lineStyle: { width: 2, color: model.color },
      areaStyle: { opacity: 0.16, color: model.color },
      emphasis: { focus: 'series' as const },
      data: trendData.value.map((day) => {
        const segment = day.segments.find((seg) => seg.label === model.label)
        return segment?.value || 0
      }),
    }))

    return {
      animationDuration: 300,
      color: trendSeriesModelOrder.value.map((m) => m.color),
      legend: {
        type: 'scroll',
        top: 0,
        left: 0,
        right: 0,
        icon: 'roundRect',
        itemWidth: 10,
        itemHeight: 6,
        itemGap: 14,
        pageIconColor: adminChartTheme.legendText,
        pageIconInactiveColor: adminChartTheme.legendInactive,
        pageTextStyle: { color: adminChartTheme.axisText },
        textStyle: { color: adminChartTheme.legendText, fontSize: 11 },
      },
      grid: { left: 18, right: 24, top: 42, bottom: 34, containLabel: true },
      tooltip: {
        trigger: 'axis',
        backgroundColor: adminChartTheme.tooltipBg,
        borderColor: adminChartTheme.tooltipBorder,
        borderWidth: 1,
        textStyle: { color: adminChartTheme.tooltipText, fontSize: 12 },
        axisPointer: {
          type: 'line',
          lineStyle: { color: adminChartTheme.tooltipMuted, type: 'dashed' },
        },
        valueFormatter: (value: number | string) => Number(value || 0).toLocaleString(),
      },
      xAxis: {
        type: 'category',
        boundaryGap: false,
        data: categories,
        axisLine: { lineStyle: { color: adminChartTheme.axisLine } },
        axisTick: { show: false },
        axisLabel: {
          color: adminChartTheme.axisText,
          fontSize: 11,
          hideOverlap: true,
          interval: (index: number) => shouldShowTrendLabel(index),
        },
      },
      yAxis: {
        type: 'value',
        minInterval: 1,
        axisLabel: {
          color: adminChartTheme.axisText,
          fontSize: 11,
          formatter: (value: number) => Math.round(value).toString(),
        },
        splitLine: { lineStyle: { color: adminChartTheme.splitLine } },
      },
      series,
      graphic: hasAnyCalls
        ? []
        : [
            {
              type: 'text',
              left: 'center',
              top: 'middle',
              style: {
                text: '暂无调用数据',
                fill: adminChartTheme.emptyText,
                fontSize: 12,
              },
            },
          ],
    } as EChartsOption
  })

  const todayCalls = computed(() => options.adminStatistics.value?.summary?.today_calls || 0)
  const todaySuccess = computed(() => options.adminStatistics.value?.summary?.today_success || 0)
  const todayFailed = computed(() => options.adminStatistics.value?.summary?.today_failed || 0)
  const yesterdayCalls = computed(
    () => options.adminStatistics.value?.summary?.yesterday_calls || 0,
  )
  const yesterdaySuccess = computed(
    () => options.adminStatistics.value?.summary?.yesterday_success || 0,
  )
  const yesterdayFailed = computed(
    () => options.adminStatistics.value?.summary?.yesterday_failed || 0,
  )

  const todayCallsFoot = computed(() =>
    buildDeltaFoot(todayCalls.value, yesterdayCalls.value, '昨日'),
  )
  const todayFailedFoot = computed(() =>
    buildDeltaFoot(todayFailed.value, yesterdayFailed.value, '昨日'),
  )
  const todayAttemptedCalls = computed(() => todaySuccess.value + todayFailed.value)
  const todaySuccessRate = computed(() =>
    todayAttemptedCalls.value === 0
      ? '0.0%'
      : `${((todaySuccess.value / todayAttemptedCalls.value) * 100).toFixed(1)}%`,
  )
  const yesterdayAttemptedCalls = computed(() => yesterdaySuccess.value + yesterdayFailed.value)
  const todaySuccessRateFoot = computed(() => {
    if (yesterdayAttemptedCalls.value === 0) return '昨日无可比数据'
    const yesterdayRate = yesterdaySuccess.value / yesterdayAttemptedCalls.value
    const currentRate =
      todayAttemptedCalls.value === 0 ? 0 : todaySuccess.value / todayAttemptedCalls.value
    return `${formatSignedPercentPoint((currentRate - yesterdayRate) * 100)} 较昨日`
  })

  const modelRiskRows = computed(() =>
    (options.adminStatistics.value?.model_health || [])
      .filter((item) => item.total > 0)
      .slice(0, 12)
      .map((item) => ({
        name: item.name === 'Unknown' ? '未识别模型' : item.name,
        total: item.total,
        failed: item.failed,
        failureRate: Math.max(0, Math.min(100, item.failure_rate || 0)),
        avgSeconds: Math.max(0, item.avg_seconds || 0),
      })),
  )

  const modelRiskChartOption = computed<EChartsOption>(() => {
    const hasModelRiskData = modelRiskRows.value.length > 0
    const maxTotal = Math.max(1, ...modelRiskRows.value.map((item) => item.total))
    return {
      animationDuration: 300,
      grid: {
        left: 46,
        right: 78,
        top: hasModelRiskData ? 58 : 44,
        bottom: 52,
        containLabel: true,
      },
      tooltip: {
        trigger: 'item',
        backgroundColor: adminChartTheme.tooltipBg,
        borderColor: adminChartTheme.tooltipBorder,
        borderWidth: 1,
        textStyle: { color: adminChartTheme.tooltipText, fontSize: 12 },
        formatter: (rawParams: unknown) => {
          const params = rawParams as {
            name?: string
            value?: [number, number, number, number]
          }
          const [avgSeconds = 0, failureRate = 0, total = 0, failed = 0] = params.value || []
          return `${escapeHTML(params.name || '未知模型')}<br/>失败率：${failureRate.toFixed(1)}%<br/>平均耗时：${avgSeconds.toFixed(2)}s<br/>调用量：${total.toLocaleString()}<br/>失败：${failed.toLocaleString()}`
        },
      },
      visualMap: {
        show: hasModelRiskData,
        type: 'piecewise',
        dimension: 1,
        top: 0,
        right: 8,
        orient: 'horizontal',
        itemWidth: 8,
        itemHeight: 8,
        itemGap: 12,
        textStyle: { color: adminChartTheme.axisText, fontSize: 11 },
        pieces: [
          { gt: 20, label: '高风险', color: '#ef4444' },
          { gt: 5, lte: 20, label: '关注', color: '#f59e0b' },
          { lte: 5, label: '稳定', color: '#10b981' },
        ],
      },
      xAxis: {
        type: 'value',
        name: '平均耗时',
        nameLocation: 'middle',
        min: 0,
        nameGap: 32,
        splitNumber: 4,
        axisLabel: {
          color: adminChartTheme.axisText,
          fontSize: 10,
          formatter: (value: number) => `${value.toFixed(1)}s`,
        },
        nameTextStyle: { color: adminChartTheme.axisText, fontSize: 10 },
        splitLine: { lineStyle: { color: adminChartTheme.splitLine } },
      },
      yAxis: {
        type: 'value',
        name: '失败率',
        min: 0,
        max: 100,
        nameGap: 24,
        axisLabel: {
          color: adminChartTheme.axisText,
          fontSize: 10,
          formatter: '{value}%',
        },
        nameTextStyle: { color: adminChartTheme.axisText, fontSize: 10 },
        splitLine: { lineStyle: { color: adminChartTheme.splitLine } },
      },
      series: [
        {
          type: 'scatter' as const,
          data: modelRiskRows.value.map((item) => ({
            name: item.name,
            value: [item.avgSeconds, item.failureRate, item.total, item.failed],
          })),
          symbolSize: (rawValue: unknown) => {
            const value = Array.isArray(rawValue) ? Number(rawValue[2] || 0) : 0
            return 10 + Math.sqrt(value / maxTotal) * 30
          },
          emphasis: {
            focus: 'self' as const,
            scale: true,
            label: {
              show: true,
              formatter: ({ name }: { name?: string }) => name || '',
              color: adminChartTheme.mainText,
              fontSize: 11,
            },
          },
          itemStyle: {
            opacity: 0.88,
            borderColor: adminChartTheme.cardBg,
            borderWidth: 1,
            shadowBlur: 8,
            shadowColor: adminChartTheme.shadow,
          },
        },
      ],
      graphic: hasModelRiskData
        ? []
        : [
            {
              type: 'text',
              left: 'center',
              top: '48%',
              style: {
                text: '暂无模型风险数据',
                fill: adminChartTheme.emptyText,
                fontSize: 12,
              },
            },
          ],
    } as EChartsOption
  })

  const qualityTrendRows = computed(() =>
    (options.adminStatistics.value?.quality_trend || []).map((item) => ({
      label: item.label || item.date,
      total: Math.max(0, Number(item.total) || 0),
      success: Math.max(0, Number(item.success) || 0),
      fail: Math.max(0, Number(item.fail) || 0),
      successRate: normalizePercent(item.success_rate),
    })),
  )
  const qualityTrendChartOption = computed<EChartsOption>(() => {
    const hasQualityData = qualityTrendRows.value.some((item) => item.total > 0)
    const categories = qualityTrendRows.value.map((item) => item.label)
    return {
      animationDuration: 300,
      color: ['#ef4444', '#6366f1'],
      legend: {
        top: 0,
        right: 0,
        itemWidth: 10,
        itemHeight: 6,
        icon: 'roundRect',
        textStyle: { color: adminChartTheme.legendText, fontSize: 11 },
      },
      grid: { left: 44, right: 54, top: 42, bottom: 36, containLabel: true },
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'cross' },
        backgroundColor: adminChartTheme.tooltipBg,
        borderColor: adminChartTheme.tooltipBorder,
        borderWidth: 1,
        textStyle: { color: adminChartTheme.tooltipText, fontSize: 12 },
        formatter: (rawParams: unknown) => {
          const params = Array.isArray(rawParams) ? rawParams : [rawParams]
          const first = params[0] as { axisValueLabel?: string; dataIndex?: number } | undefined
          const row = qualityTrendRows.value[first?.dataIndex ?? -1]
          const axisValue = escapeHTML(String(first?.axisValueLabel || ''))
          if (!row) return axisValue
          return `${axisValue}<br/>总调用：${row.total.toLocaleString()}<br/>成功：${row.success.toLocaleString()}<br/>失败：${row.fail.toLocaleString()}<br/>成功率：${row.successRate.toFixed(1)}%`
        },
      },
      xAxis: {
        type: 'category',
        data: categories,
        axisLine: { lineStyle: { color: adminChartTheme.axisLine } },
        axisTick: { show: false },
        axisLabel: {
          color: adminChartTheme.axisText,
          fontSize: 10,
          hideOverlap: true,
          interval: (index: number) =>
            shouldShowAxisLabel(index, qualityTrendRows.value.length, options.timeRange.value),
        },
      },
      yAxis: [
        {
          type: 'value',
          min: 0,
          minInterval: 1,
          axisLabel: { color: adminChartTheme.axisText, fontSize: 10 },
          splitLine: { lineStyle: { color: adminChartTheme.splitLine } },
        },
        {
          type: 'value',
          min: 0,
          max: 100,
          axisLabel: {
            color: adminChartTheme.axisText,
            fontSize: 10,
            formatter: '{value}%',
          },
          splitLine: { show: false },
        },
      ],
      series: [
        {
          name: '失败',
          type: 'bar' as const,
          yAxisIndex: 0,
          barMaxWidth: 18,
          data: qualityTrendRows.value.map((item) => item.fail),
          itemStyle: { color: '#ef4444', borderRadius: [3, 3, 0, 0] },
        },
        {
          name: '成功率',
          type: 'line' as const,
          yAxisIndex: 1,
          smooth: true,
          symbolSize: 5,
          lineStyle: { width: 2, color: '#6366f1' },
          itemStyle: { color: '#6366f1' },
          data: qualityTrendRows.value.map((item) => item.successRate),
        },
      ],
      graphic: hasQualityData
        ? []
        : [
            {
              type: 'text',
              left: 'center',
              top: 'middle',
              style: {
                text: '暂无服务质量趋势数据',
                fill: adminChartTheme.emptyText,
                fontSize: 12,
              },
            },
          ],
    } as EChartsOption
  })

  const slowModelRows = computed(() =>
    (options.adminStatistics.value?.slow_models || [])
      .map((item, idx) => {
        const modelName = item.name || '未知模型'
        const p95Seconds = Math.max(0, Number(item.p95_seconds) || 0)
        const avgSeconds = Math.max(0, Number(item.avg_seconds) || 0)
        const samples = Math.max(0, Math.round(Number(item.samples) || 0))
        return {
          name: modelName,
          p95Seconds,
          avgSeconds,
          samples,
          color:
            getModelColorByName(modelName) ||
            defaultModelColors[idx % defaultModelColors.length] ||
            '#6366f1',
        }
      })
      .sort((a, b) => b.p95Seconds - a.p95Seconds)
      .slice(0, 6),
  )
  const slowModelsChartOption = computed<EChartsOption>(() => {
    const hasSlowModelData = slowModelRows.value.length > 0
    return {
      animationDuration: 260,
      grid: { left: 14, right: 76, top: 12, bottom: 12, containLabel: true },
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'shadow' },
        backgroundColor: adminChartTheme.tooltipBg,
        borderColor: adminChartTheme.tooltipBorder,
        borderWidth: 1,
        textStyle: { color: adminChartTheme.tooltipText, fontSize: 12 },
        formatter: (rawParams: unknown) => {
          const params = (Array.isArray(rawParams) ? rawParams[0] : rawParams) as {
            axisValueLabel?: string
            data?: { p95Seconds?: number; avgSeconds?: number; samples?: number }
          }
          const data = params.data || {}
          const p95 = Number(data.p95Seconds || 0)
          const avg = Number(data.avgSeconds || 0)
          const samples = Number(data.samples || 0)
          return `${escapeHTML(params.axisValueLabel || '未知模型')}<br/>P95：${p95.toFixed(2)}s<br/>平均：${avg.toFixed(2)}s<br/>样本：${samples.toLocaleString()}`
        },
      },
      xAxis: {
        type: 'value',
        min: 0,
        splitNumber: 3,
        axisLabel: {
          color: adminChartTheme.axisText,
          fontSize: 10,
          formatter: (value: number) => `${value.toFixed(1)}s`,
        },
        splitLine: { lineStyle: { color: adminChartTheme.splitLine } },
      },
      yAxis: {
        type: 'category',
        inverse: true,
        axisTick: { show: false },
        axisLine: { show: false },
        axisLabel: {
          color: adminChartTheme.axisText,
          fontSize: 11,
          width: 110,
          overflow: 'truncate',
        },
        data: slowModelRows.value.map((row) => row.name),
      },
      series: [
        {
          type: 'bar' as const,
          barWidth: 8,
          showBackground: true,
          backgroundStyle: { color: 'rgba(148, 163, 184, 0.1)', borderRadius: [0, 4, 4, 0] },
          data: slowModelRows.value.map((row) => ({
            value: row.p95Seconds,
            p95Seconds: row.p95Seconds,
            avgSeconds: row.avgSeconds,
            samples: row.samples,
            itemStyle: { color: row.color, borderRadius: [0, 4, 4, 0] },
          })),
          label: {
            show: true,
            position: 'right',
            color: adminChartTheme.axisText,
            fontSize: 10,
            formatter: ({ value }: { value: number }) => `${Number(value || 0).toFixed(2)}s`,
          },
        },
      ],
      graphic: hasSlowModelData
        ? []
        : [
            {
              type: 'text',
              left: 'center',
              top: 'middle',
              style: {
                text: '暂无慢模型数据',
                fill: adminChartTheme.emptyText,
                fontSize: 12,
              },
            },
          ],
    } as EChartsOption
  })

  const monitorQueueTotal = computed(
    () =>
      (options.monitorData.value?.global_queue_len || 0) +
      (options.monitorData.value?.wait_queue_len || 0),
  )
  const monitorRunningTotal = computed(() => options.monitorData.value?.running_tasks || 0)
  const queuePressurePercent = computed(() => {
    const pending = monitorQueueTotal.value
    const running = monitorRunningTotal.value
    const total = pending + running
    if (total === 0) return 0
    return Math.round((pending / total) * 100)
  })
  const queuePressure = computed(() => `${queuePressurePercent.value}%`)
  const queuePressureFoot = computed(
    () => `排队 ${monitorQueueTotal.value} / 执行 ${monitorRunningTotal.value}`,
  )

  const accountLoadRows = computed(() => {
    const loads = options.monitorData.value?.account_loads || {}
    return Object.entries(loads)
      .map(([id, val]) => {
        const current = val.current_concurrent || 0
        const max = Math.max(1, val.max_concurrent || 0)
        const percent = Math.min(100, Math.round((current / max) * 100))
        const color = percent >= 90 ? '#ef4444' : percent >= 70 ? '#f59e0b' : '#10b981'
        return { id, current, max, percent, color }
      })
      .sort((a, b) => b.percent - a.percent)
      .slice(0, 5)
  })

  const peakAccountLoad = computed(() => accountLoadRows.value[0] || null)
  const peakAccountLoadPercent = computed(() => peakAccountLoad.value?.percent || 0)

  const accountLoadChartOption = computed<EChartsOption>(() => {
    const hasAccountData = accountLoadRows.value.length > 0
    return {
      animationDuration: 260,
      grid: { left: 10, right: 70, top: 8, bottom: 8, containLabel: true },
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'shadow' },
        backgroundColor: adminChartTheme.tooltipBg,
        borderColor: adminChartTheme.tooltipBorder,
        borderWidth: 1,
        textStyle: { color: adminChartTheme.tooltipText, fontSize: 12 },
        formatter: (rawParams: unknown) => {
          const params = (Array.isArray(rawParams) ? rawParams[0] : rawParams) as {
            axisValueLabel?: string
            data?: { current?: number; max?: number; value?: number }
          }
          const current = Number(params.data?.current || 0)
          const max = Number(params.data?.max || 0)
          const value = Number(params.data?.value || 0)
          return `${escapeHTML(params.axisValueLabel || '未知账号')}<br/>并发：${current}/${max}<br/>负载：${value}%`
        },
      },
      xAxis: {
        type: 'value',
        min: 0,
        max: 100,
        splitNumber: 4,
        axisTick: { show: false },
        axisLine: { show: false },
        axisLabel: { color: adminChartTheme.axisText, fontSize: 10, formatter: '{value}%' },
        splitLine: { lineStyle: { color: adminChartTheme.splitLine } },
      },
      yAxis: {
        type: 'category',
        inverse: true,
        axisTick: { show: false },
        axisLine: { show: false },
        axisLabel: { color: adminChartTheme.axisText, fontSize: 11 },
        data: accountLoadRows.value.map((item) => `#${item.id}`),
      },
      series: [
        {
          type: 'bar' as const,
          barWidth: 10,
          showBackground: true,
          backgroundStyle: {
            color: 'rgba(148, 163, 184, 0.12)',
            borderRadius: [0, 6, 6, 0],
          },
          data: accountLoadRows.value.map((item) => ({
            value: item.percent,
            current: item.current,
            max: item.max,
            itemStyle: { color: item.color, borderRadius: [0, 6, 6, 0] },
          })),
          label: {
            show: true,
            position: 'right',
            color: adminChartTheme.axisText,
            fontSize: 10,
            formatter: ({ data }: { data: { current: number; max: number } }) =>
              `${data.current}/${data.max}`,
          },
        },
      ],
      graphic: hasAccountData
        ? []
        : [
            {
              type: 'text',
              left: 'center',
              top: 'middle',
              style: {
                text: '暂无渠道并发数据',
                fill: adminChartTheme.emptyText,
                fontSize: 12,
              },
            },
          ],
    } as EChartsOption
  })

  const userRiskRows = computed(() =>
    (options.adminStatistics.value?.user_risk || [])
      .map((item) => ({
        name: item.name || '未知用户',
        total: Math.max(0, Number(item.total) || 0),
        completed: Math.max(0, Number(item.completed) || 0),
        failed: Math.max(0, Number(item.failed) || 0),
        failureRate: normalizePercent(item.failure_rate),
      }))
      .sort((a, b) => b.failed - a.failed)
      .slice(0, 6),
  )
  const userRiskChartOption = computed<EChartsOption>(() => {
    const hasRiskData = userRiskRows.value.length > 0
    return {
      animationDuration: 260,
      grid: { left: 14, right: 68, top: 12, bottom: 12, containLabel: true },
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'shadow' },
        backgroundColor: adminChartTheme.tooltipBg,
        borderColor: adminChartTheme.tooltipBorder,
        borderWidth: 1,
        textStyle: { color: adminChartTheme.tooltipText, fontSize: 12 },
        formatter: (rawParams: unknown) => {
          const params = (Array.isArray(rawParams) ? rawParams[0] : rawParams) as {
            axisValueLabel?: string
            data?: {
              failed?: number
              failureRate?: number
              completed?: number
              total?: number
            }
          }
          const data = params.data || {}
          const total = Number(data.total || 0)
          const completed = Number(data.completed || 0)
          const failed = Number(data.failed || 0)
          const failureRate = Number(data.failureRate || 0)
          return `${escapeHTML(params.axisValueLabel || '未知用户')}<br/>失败：${failed.toLocaleString()}<br/>成功：${completed.toLocaleString()}<br/>总调用：${total.toLocaleString()}<br/>失败率：${failureRate.toFixed(1)}%`
        },
      },
      xAxis: {
        type: 'value',
        min: 0,
        splitNumber: 3,
        axisLabel: { color: adminChartTheme.axisText, fontSize: 10 },
        splitLine: { lineStyle: { color: adminChartTheme.splitLine } },
      },
      yAxis: {
        type: 'category',
        inverse: true,
        axisTick: { show: false },
        axisLine: { show: false },
        axisLabel: {
          color: adminChartTheme.axisText,
          fontSize: 11,
          width: 100,
          overflow: 'truncate',
        },
        data: userRiskRows.value.map((item) => item.name),
      },
      series: [
        {
          type: 'bar' as const,
          barWidth: 8,
          showBackground: true,
          backgroundStyle: { color: 'rgba(148, 163, 184, 0.1)', borderRadius: [0, 4, 4, 0] },
          data: userRiskRows.value.map((item) => ({
            value: item.failed,
            failed: item.failed,
            completed: item.completed,
            total: item.total,
            failureRate: item.failureRate,
            itemStyle: {
              color:
                item.failureRate >= 20 ? '#ef4444' : item.failureRate >= 8 ? '#f59e0b' : '#6366f1',
              borderRadius: [0, 4, 4, 0],
            },
          })),
          label: {
            show: true,
            position: 'right',
            color: adminChartTheme.axisText,
            fontSize: 10,
            formatter: ({ data }: { data: { failureRate: number } }) =>
              `${data.failureRate.toFixed(1)}%`,
          },
        },
      ],
      graphic: hasRiskData
        ? []
        : [
            {
              type: 'text',
              left: 'center',
              top: 'middle',
              style: {
                text: '暂无异常用户数据',
                fill: adminChartTheme.emptyText,
                fontSize: 12,
              },
            },
          ],
    } as EChartsOption
  })

  return {
    trendData,
    totalCallsInRange,
    trendChartOption,
    todayCalls,
    todayFailed,
    todayCallsFoot,
    todayFailedFoot,
    todaySuccessRate,
    todaySuccessRateFoot,
    queuePressure,
    queuePressureFoot,
    modelRiskChartOption,
    qualityTrendChartOption,
    slowModelsChartOption,
    accountLoadChartOption,
    peakAccountLoadPercent,
    userRiskChartOption,
  }
}

function escapeHTML(value: string) {
  return value.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;')
}

function normalizeModelName(name: string | null | undefined): string {
  return (name || '').trim().toLowerCase()
}

function colorForModel(name: string, palette: string[]) {
  let hash = 0
  for (let i = 0; i < name.length; i++) hash = (hash << 5) - hash + name.charCodeAt(i)
  return palette[Math.abs(hash) % palette.length]
}

function isMonthStartLabel(label: string): boolean {
  const value = (label || '').trim()
  if (!value) return false
  const mmdd = /^(\d{2})\/(\d{2})$/.exec(value)
  if (mmdd) return mmdd[2] === '01'
  const ymd = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value)
  if (ymd) return ymd[3] === '01'
  return false
}

function buildDeltaFoot(current: number, previous: number, label: string) {
  if (previous === 0) return current === 0 ? '无数据' : `${current}（${label}无可比数据）`
  const diff = current - previous
  const ratio = Math.abs((diff / previous) * 100).toFixed(1)
  if (diff === 0) return `0.0% 持平 ${label}`
  return `${diff > 0 ? '+' : '-'}${ratio}% ${diff > 0 ? '较' : '低于'}${label}`
}

function formatSignedPercentPoint(value: number) {
  if (value === 0) return '0.0pp'
  return `${value > 0 ? '+' : '-'}${Math.abs(value).toFixed(1)}pp`
}

function normalizePercent(value: number): number {
  const parsed = Number(value) || 0
  const percent = parsed <= 1 ? parsed * 100 : parsed
  return Math.max(0, Math.min(100, percent))
}

function shouldShowAxisLabel(index: number, count: number, timeRange: string) {
  if (count <= 7) return true
  if (timeRange === '30d') return index === 0 || index === count - 1 || index % 6 === 0
  if (timeRange === '1y') return index === 0 || index === count - 1 || index % 30 === 0
  return true
}
