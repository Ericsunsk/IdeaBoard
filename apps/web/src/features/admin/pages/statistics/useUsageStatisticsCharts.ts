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
  user_rank?: Array<{ name: string; value: number }>
  latency?: Array<{ name: string; avg_seconds: number }>
  health_cells?: Array<{ status: number; success: number; fail: number; total: number }>
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
  const HEALTH_COLS = 70
  const HEALTH_ROWS = 6

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
      grid: { left: 10, right: 12, top: 36, bottom: 30, containLabel: true },
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
      grid: { left: 8, right: 10, top: 54, bottom: 28, containLabel: true },
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
          return `${escapeHTML(params.name || '未知模型')}<br/>失败率：${failureRate.toFixed(1)}%<br/>平均 TTFT：${avgSeconds.toFixed(2)}s<br/>调用量：${total.toLocaleString()}<br/>失败：${failed.toLocaleString()}`
        },
      },
      visualMap: {
        show: hasModelRiskData,
        type: 'piecewise',
        dimension: 1,
        top: 0,
        right: 0,
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
        name: 'TTFT',
        min: 0,
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
              top: 'middle',
              style: {
                text: '暂无模型风险数据',
                fill: adminChartTheme.emptyText,
                fontSize: 12,
              },
            },
          ],
    } as EChartsOption
  })

  const healthGridFlat = computed(() => {
    const totalCells = HEALTH_COLS * HEALTH_ROWS
    const rows = options.adminStatistics.value?.health_cells || []
    if (rows.length >= totalCells) return rows.slice(0, totalCells)
    return rows.concat(
      Array.from({ length: totalCells - rows.length }, () => ({
        status: 0,
        success: 0,
        fail: 0,
        total: 0,
      })),
    )
  })
  const healthSampleTotal = computed(() =>
    healthGridFlat.value.reduce((sum, cell) => sum + (cell.total || 0), 0),
  )
  const healthSuccessTotal = computed(() =>
    healthGridFlat.value.reduce((sum, cell) => sum + (cell.success || 0), 0),
  )
  const healthRangeLabel = computed(() => getRangeLabel(options.timeRange.value))

  const healthHeatmapOption = computed<EChartsOption>(() => {
    const hasHealthData = healthGridFlat.value.some((cell) => cell.total > 0)
    const heatmapData = healthGridFlat.value.map((cell, index) => ({
      value: [index % HEALTH_COLS, Math.floor(index / HEALTH_COLS), cell.status],
      success: cell.success,
      fail: cell.fail,
      total: cell.total,
      index,
    }))

    return {
      animationDuration: 200,
      grid: { left: 0, right: 0, top: 6, bottom: 36, containLabel: false },
      tooltip: {
        trigger: 'item',
        backgroundColor: adminChartTheme.tooltipBg,
        borderColor: adminChartTheme.tooltipBorder,
        borderWidth: 1,
        textStyle: { color: adminChartTheme.tooltipText, fontSize: 12 },
        formatter: (rawParams: unknown) => {
          const params = rawParams as {
            data?: { success?: number; fail?: number; total?: number; index?: number }
          }
          const data = params.data
          if (!data) return ''
          const success = Number(data.success) || 0
          const fail = Number(data.fail) || 0
          const total = Number(data.total) || 0
          const passRate = total === 0 ? '0.0' : ((success / total) * 100).toFixed(1)
          return `${formatHealthRangeLabel(options.timeRange.value, Number(data.index) || 0)}<br/>成功 ${success} / 失败 ${fail}<br/>成功率 ${passRate}%`
        },
      },
      xAxis: {
        type: 'category',
        show: false,
        data: Array.from({ length: HEALTH_COLS }, (_, i) => i),
      },
      yAxis: {
        type: 'category',
        show: false,
        inverse: true,
        data: Array.from({ length: HEALTH_ROWS }, (_, i) => i),
      },
      visualMap: {
        show: true,
        type: 'piecewise',
        dimension: 2,
        min: 0,
        max: 3,
        left: 0,
        bottom: 0,
        orient: 'horizontal',
        itemWidth: 8,
        itemHeight: 8,
        itemGap: 14,
        textStyle: { color: adminChartTheme.axisText, fontSize: 11 },
        pieces: [
          { value: 3, label: '正常', color: '#10b981' },
          { value: 2, label: '波动', color: '#f59e0b' },
          { value: 1, label: '故障', color: '#ef4444' },
          { value: 0, label: '无数据', color: '#e2e8f0' },
        ],
      },
      series: [
        {
          type: 'heatmap' as const,
          data: heatmapData,
          progressive: 0,
          itemStyle: {
            borderRadius: 1.5,
            borderWidth: 1,
            borderColor: 'rgba(148, 163, 184, 0.12)',
          },
          emphasis: {
            itemStyle: {
              shadowBlur: 8,
              shadowColor: 'rgba(15, 23, 42, 0.24)',
            },
          },
        },
      ],
      graphic: hasHealthData
        ? []
        : [
            {
              type: 'text',
              left: 'center',
              top: 'middle',
              style: {
                text: '暂无健康监测数据',
                fill: adminChartTheme.emptyText,
                fontSize: 12,
              },
            },
          ],
    } as EChartsOption
  })

  const latencyRows = computed(() => {
    const rows: Array<{ name: string; val: number; color: string }> = []
    const source = options.adminStatistics.value?.latency || []
    source.forEach((item, idx) => {
      const latency = Number(item.avg_seconds)
      if (!Number.isFinite(latency)) return
      const color = getModelColorByName(item.name)
      rows.push({
        name: item.name || '未知模型',
        val: Math.max(0, latency),
        color:
          color || defaultModelColors[idx % defaultModelColors.length] || 'var(--admin-warning)',
      })
    })
    return rows.sort((a, b) => b.val - a.val).slice(0, 5)
  })
  const latencyChartOption = computed<EChartsOption>(() => {
    const hasLatencyData = latencyRows.value.length > 0
    return {
      animationDuration: 260,
      grid: { left: 10, right: 12, top: 6, bottom: 4, containLabel: true },
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
            value?: number
          }
          const value = Number(params?.value) || 0
          return `${escapeHTML(params?.axisValueLabel || '未知模型')}<br/>平均 TTFT：${value.toFixed(2)}s`
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
          width: 90,
          overflow: 'truncate',
        },
        data: latencyRows.value.map((row) => row.name),
      },
      series: [
        {
          type: 'bar' as const,
          showBackground: true,
          backgroundStyle: { color: 'rgba(148, 163, 184, 0.1)', borderRadius: [0, 4, 4, 0] },
          data: latencyRows.value.map((row) => ({
            value: row.val,
            itemStyle: { color: row.color, borderRadius: [0, 4, 4, 0] },
          })),
          barWidth: 8,
          label: {
            show: true,
            position: 'right',
            color: adminChartTheme.axisText,
            fontSize: 10,
            formatter: ({ value }: { value: number }) => `${Number(value).toFixed(2)}s`,
          },
        },
      ],
      graphic: hasLatencyData
        ? []
        : [
            {
              type: 'text',
              left: 'center',
              top: 'middle',
              style: {
                text: '暂无延迟数据',
                fill: adminChartTheme.emptyText,
                fontSize: 12,
              },
            },
          ],
    } as EChartsOption
  })

  const healthRate = computed(() =>
    healthSampleTotal.value === 0
      ? '0.0%'
      : `${((healthSuccessTotal.value / healthSampleTotal.value) * 100).toFixed(1)}%`,
  )

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
  const accountLoadChartOption = computed<EChartsOption>(() => {
    const hasAccountData = accountLoadRows.value.length > 0
    return {
      animationDuration: 260,
      grid: { left: 4, right: 8, top: 8, bottom: 4, containLabel: true },
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
        splitNumber: 3,
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
          barWidth: 8,
          showBackground: true,
          backgroundStyle: { color: 'rgba(148, 163, 184, 0.1)', borderRadius: [0, 4, 4, 0] },
          data: accountLoadRows.value.map((item) => ({
            value: item.percent,
            current: item.current,
            max: item.max,
            itemStyle: { color: item.color, borderRadius: [0, 4, 4, 0] },
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

  const userRankRows = computed(() =>
    (options.adminStatistics.value?.user_rank || [])
      .slice(0, 6)
      .map((item, idx) => ({ ...item, rank: idx + 1 })),
  )
  const userRankChartOption = computed<EChartsOption>(() => {
    const hasRankData = userRankRows.value.length > 0
    return {
      animationDuration: 260,
      grid: { left: 10, right: 10, top: 8, bottom: 4, containLabel: true },
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
            value?: number
            dataIndex?: number
          }
          const index = Number(params.dataIndex) || 0
          return `#${index + 1} ${escapeHTML(params.axisValueLabel || '未知用户')}<br/>调用量：${Number(params.value || 0).toLocaleString()}`
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
        data: userRankRows.value.map((item) => item.name),
      },
      series: [
        {
          type: 'bar' as const,
          barWidth: 8,
          showBackground: true,
          backgroundStyle: { color: 'rgba(148, 163, 184, 0.1)', borderRadius: [0, 4, 4, 0] },
          data: userRankRows.value.map((item) => item.value),
          itemStyle: { color: '#4f46e5', borderRadius: [0, 4, 4, 0] },
          label: {
            show: true,
            position: 'right',
            color: adminChartTheme.axisText,
            fontSize: 10,
            formatter: ({ value }: { value: number }) => Number(value).toLocaleString(),
          },
        },
      ],
      graphic: hasRankData
        ? []
        : [
            {
              type: 'text',
              left: 'center',
              top: 'middle',
              style: {
                text: '暂无用户排行数据',
                fill: adminChartTheme.emptyText,
                fontSize: 12,
              },
            },
          ],
    } as EChartsOption
  })

  const saturatedAccountCount = computed(
    () => accountLoadRows.value.filter((item) => item.percent >= 90).length,
  )

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
    healthHeatmapOption,
    latencyChartOption,
    accountLoadChartOption,
    userRankChartOption,
    healthRangeLabel,
    healthRate,
    saturatedAccountCount,
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

function formatHealthRangeLabel(timeRange: string, index: number) {
  return `${getRangeLabel(timeRange)} 样本区间 #${index + 1}`
}

function getRangeLabel(timeRange: string) {
  if (timeRange === '7d') return '最近7天'
  if (timeRange === '30d') return '最近30天'
  return '最近1年'
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
