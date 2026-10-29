import { computed, ref, type Ref } from 'vue'
import type { ChartDataset } from 'chart.js'
import { adminChartPlugins, adminChartTheme, defineAdminChartTooltip } from '../../shared'
import { formatLocaleNumber } from '@/core/i18n/formatters'
import {
  getShanghaiMonthShortFormat,
  getShanghaiWallPartsFormat,
  toShanghaiParts,
} from '@/core/datetime/shanghai'
import { formatCompactNumber, resolveChartScaleMax, withAlpha } from '@/core/lib/chartPrimitives'
import { adminTaskStatuses } from '../core/status'
import { taskMetricDefinitions, monitorChartSpecs } from '../../shared'
import { timeRangePresets, type TimeRangePresetKey } from '../core/timeRange'
import type { AdminTaskRow } from './useAdminTaskList'
import type { TaskStatus } from '../../shared'

type MonitorTimelineOptions = {
  chartRows: Ref<readonly AdminTaskRow[]>
  filterStatus: Ref<TaskStatus | null>
}

// Use Shanghai wall time and fixed en-US month names regardless of the browser zone.

export function toGmt8Date(value: Date | number) {
  const date = value instanceof Date ? value : new Date(value)
  const entries = toShanghaiParts(date, getShanghaiWallPartsFormat())
  return new Date(
    Number(entries.year),
    Number(entries.month) - 1,
    Number(entries.day),
    Number(entries.hour),
    Number(entries.minute),
    Number(entries.second),
  )
}

export function formatXAxisTime(date: Date, durationMs: number) {
  const entries = toShanghaiParts(date, getShanghaiWallPartsFormat())
  const hh = entries.hour ?? '00'
  const mm = entries.minute ?? '00'
  if (durationMs <= 12 * 3600 * 1000) {
    return `${hh}:${mm}`
  }
  const mon = getShanghaiMonthShortFormat().format(date)
  const day = Number(entries.day)
  if (durationMs <= 24 * 3600 * 1000) {
    return `${mon} ${day}, ${hh}:${mm}`
  }
  return `${mon} ${day} ${hh}:00`
}

export function calculateTimelineYScale(maxCount: number): { max: number; stepSize: number } {
  // 取整算法单源（shared resolveChartScaleMax）；headroom 1.1 与步数≈5 沿用旧观感参数。
  const max = resolveChartScaleMax({ values: [maxCount], min: 10, headroom: 1.1 }) ?? 10
  return {
    max,
    stepSize: Math.max(1, max / 5),
  }
}

export function useMonitorTimeline(options: MonitorTimelineOptions) {
  const { chartRows, filterStatus } = options

  const selectedTimeRangeKey = ref<TimeRangePresetKey>('1h')

  const selectedPreset = computed(() => {
    return (
      timeRangePresets.find((p) => p.key === selectedTimeRangeKey.value) ??
      timeRangePresets.find((p) => p.key === '1h') ??
      timeRangePresets[0]!
    )
  })

  function selectTimeRange(key: TimeRangePresetKey) {
    selectedTimeRangeKey.value = key
  }

  const timelineBuckets = computed(() => {
    const bucketCount = 10
    const durationMs = selectedPreset.value.durationMs
    const bucketDurationMs = durationMs / bucketCount
    const now = Date.now()
    const startTime = now - durationMs

    const labels: string[] = []
    const bucketMap: Record<string, number[]> = {
      [adminTaskStatuses.pending]: new Array(bucketCount).fill(0),
      [adminTaskStatuses.running]: new Array(bucketCount).fill(0),
      [adminTaskStatuses.completed]: new Array(bucketCount).fill(0),
      [adminTaskStatuses.failed]: new Array(bucketCount).fill(0),
      [adminTaskStatuses.cancelled]: new Array(bucketCount).fill(0),
    }

    for (let i = 0; i < bucketCount; i++) {
      const time = new Date(startTime + i * bucketDurationMs)
      labels.push(formatXAxisTime(time, durationMs))
    }

    // 图表走图表专用全量行（最近 100），按所选区间分桶。
    // 区间外的旧行直接丢弃（不钳制到首桶，避免切小区间时出现虚假尖峰）；
    // 非法日期同样丢弃。
    const dataSource = chartRows.value

    for (const t of dataSource) {
      const taskTime = t.created_at ? new Date(t.created_at).getTime() : Number.NaN
      if (!Number.isFinite(taskTime) || taskTime < startTime) continue
      let index = Math.floor((taskTime - startTime) / bucketDurationMs)
      if (index < 0) index = 0
      if (index >= bucketCount) index = bucketCount - 1
      const bucket = bucketMap[t.status]
      const count = bucket?.[index]
      if (bucket && count !== undefined) {
        bucket[index] = count + 1
      }
    }

    return {
      labels,
      bucketMap,
    }
  })

  const timelineChartData = computed(() => {
    const { labels, bucketMap } = timelineBuckets.value

    const datasets = taskMetricDefinitions
      .filter((metric) => {
        if (!metric.hasChartLine || !metric.status) return false
        return filterStatus.value === null || filterStatus.value === metric.status
      })
      .map(
        (metric) =>
          ({
            type: 'line',
            label: metric.label,
            data: bucketMap[metric.status!] ?? [],
            borderColor: adminChartTheme[metric.chartColorRole!],
            backgroundColor: withAlpha(
              adminChartTheme[metric.chartColorRole!],
              monitorChartSpecs.fillOpacity,
            ),
            fill: true,
            tension: monitorChartSpecs.lineTension,
            borderWidth: monitorChartSpecs.lineWidth,
            pointStyle: 'circle',
            pointRadius: monitorChartSpecs.pointRadius,
            pointHoverRadius: monitorChartSpecs.pointHoverRadius,
            pointHitRadius: monitorChartSpecs.pointHitRadius,
            pointBackgroundColor: adminChartTheme[metric.chartColorRole!],
            pointBorderColor: 'transparent',
            pointBorderWidth: 0,
            clip: false,
          }) satisfies ChartDataset<'line', number[]>,
      )

    return {
      labels,
      datasets,
    }
  })

  const timelineMaxCount = computed(() => {
    const { bucketMap } = timelineBuckets.value

    const singleSeries = filterStatus.value !== null ? bucketMap[filterStatus.value] : undefined
    const activeSeries: number[][] =
      filterStatus.value === null ? Object.values(bucketMap) : singleSeries ? [singleSeries] : []

    let max = 0
    for (const series of activeSeries) {
      for (const val of series) {
        if (val > max) max = val
      }
    }
    return max
  })

  const timelineChartOptions = computed(() => {
    const { max, stepSize } = calculateTimelineYScale(timelineMaxCount.value)

    return {
      responsive: true,
      maintainAspectRatio: false,
      layout: {
        padding: {
          top: 4,
          right: 4,
          bottom: 4,
          left: 4,
        },
      },
      interaction: {
        mode: 'index' as const,
        intersect: false,
      },
      plugins: {
        legend: {
          display: false,
        },
        tooltip: defineAdminChartTooltip({
          mode: 'index' as const,
          intersect: false,
          callbacks: {
            labelColor(context: { dataset: { borderColor?: unknown } }) {
              const color = (context.dataset.borderColor as string) || ''
              return {
                borderColor: color,
                backgroundColor: color,
                borderWidth: 0,
              }
            },
            label(context: { dataset: { label?: string }; raw: unknown }) {
              const label = context.dataset.label || ''
              const val = context.raw
              const formatted = typeof val === 'number' ? formatLocaleNumber(val) : String(val ?? 0)
              return ` ${label}: ${formatted}`
            },
          },
        }),
      },
      scales: {
        x: {
          grid: {
            display: false,
          },
          ticks: {
            font: {
              size: 10,
              family: 'ui-monospace, monospace, sans-serif',
            },
            maxRotation: 0,
            color: adminChartTheme.axisText,
          },
          border: {
            display: true,
            color: adminChartTheme.axisLine,
          },
        },
        y: {
          beginAtZero: true,
          min: 0,
          max,
          position: 'left' as const,
          grid: {
            color: withAlpha(adminChartTheme.splitLine, 0.6),
          },
          ticks: {
            precision: 0,
            stepSize,
            font: {
              size: 10,
              family: 'ui-monospace, monospace, sans-serif',
            },
            color: adminChartTheme.axisText,
            callback: (value: string | number) => formatCompactNumber(Number(value)),
          },
          border: {
            display: true,
            color: adminChartTheme.axisLine,
          },
        },
      },
    }
  })

  return {
    selectedTimeRangeKey,
    selectedPreset,
    selectTimeRange,
    timelineChartData,
    timelineChartOptions,
    adminChartPlugins,
  }
}
