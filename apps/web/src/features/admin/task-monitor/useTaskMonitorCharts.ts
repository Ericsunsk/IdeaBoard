import { computed, type Ref } from 'vue'
import {
  ListAdminTasksStatus,
  type ListAdminTasksStatus as ListAdminTasksStatusValue,
  type MonitorData,
  type MonitorStats,
} from '../../../shared/api/generated-query/model'
import {
  defineChartData,
  type ChartData,
  type ChartOptions,
  withAlpha,
} from '../../../shared/lib/chartPrimitives'
import {
  adminChartLegend,
  adminChartStrokeWidth,
  adminChartTheme,
  defineAdminChartBubbleDatasetStyle,
  defineAdminChartPercentScale,
  defineAdminChartPointClipStyle,
  defineAdminChartRadialPercentScale,
  defineAdminChartOptions,
  getAdminChartBubbleRadius,
} from '../shared/lib/chart'
import { getTaskStatusDefinition, taskStatusOrder } from './status'
import {
  normalizeMonitorExecutionPressurePoints,
  type TaskExecutionPressurePoint,
  type TaskExecutionSignalKey,
} from './executionPressure'

export type TaskStatusSnapshot = Record<ListAdminTasksStatusValue, number>

export type TaskDistributionRow = {
  key: ListAdminTasksStatusValue
  label: string
  count: number
  sharePercent: number
}

export type AccountLoadRow = {
  id: string
  current: number
  max: number
  available: number
  percent: number
  color: string
}

type ExecutionPressureChartPoint = TaskExecutionPressurePoint & {
  x: number
  y: number
  r: number
}

const adminMonitorTooltipBase = {
  backgroundColor: adminChartTheme.tooltipBg,
  titleColor: adminChartTheme.tooltipText,
  bodyColor: adminChartTheme.tooltipText,
  borderColor: adminChartTheme.tooltipBorder,
  borderWidth: 1,
  padding: 10,
} as const

const executionPressureColorByKey: Record<TaskExecutionSignalKey, string> = {
  fast_completed: adminChartTheme.success,
  slow_completed: adminChartTheme.olive,
  normal_pending: adminChartTheme.mist,
  backlog_pending: adminChartTheme.warning,
  normal_running: adminChartTheme.info,
  stuck_running: adminChartTheme.mauve,
  fast_failed: adminChartTheme.danger,
  slow_failed: adminChartTheme.brand,
  cancelled: adminChartTheme.emptyText,
  retrying: adminChartTheme.cyan,
  other: adminChartTheme.emptyText,
}

const taskDistributionStatuses = taskStatusOrder.map((key) => ({
  key,
  label: getTaskStatusDefinition(key).text,
}))

export function useTaskMonitorCharts(options: {
  monitorData: Ref<MonitorData | null>
  monitorStats: Ref<MonitorStats | null>
}) {
  const taskStatusSnapshot = computed(() => buildTaskStatusSnapshot(options.monitorStats.value))
  const statusTotalCount = computed(() => getTaskStatusTotalCount(taskStatusSnapshot.value))
  const accountLoadRows = computed(() =>
    buildAccountLoadRows(options.monitorData.value?.account_loads),
  )
  const peakAccountLoadPercent = computed(() => accountLoadRows.value[0]?.percent || 0)
  const accountLoadCurrent = computed(() =>
    accountLoadRows.value.reduce((sum, load) => sum + load.current, 0),
  )
  const accountLoadCapacity = computed(() =>
    accountLoadRows.value.reduce((sum, load) => sum + load.max, 0),
  )
  const taskDistributionRows = computed(() => buildTaskDistributionRows(taskStatusSnapshot.value))
  const executionPressurePoints = computed(() =>
    normalizeMonitorExecutionPressurePoints(options.monitorData.value?.execution_pressure ?? []),
  )
  const activeExecutionPressurePoints = computed(() =>
    executionPressurePoints.value.filter((point) => point.count > 0),
  )

  const accountConcurrencyChartData = computed(() =>
    buildAccountConcurrencyChartData(accountLoadRows.value),
  )
  const accountConcurrencyChartOptions = computed(() =>
    buildAccountConcurrencyChartOptions(accountLoadRows.value),
  )
  const taskDistributionChartData = computed(() =>
    buildTaskDistributionChartData(taskDistributionRows.value),
  )
  const taskDistributionChartOptions = computed(() =>
    buildTaskDistributionChartOptions(taskDistributionRows.value),
  )
  const executionPressureChartData = computed(() =>
    buildExecutionPressureChartData(executionPressurePoints.value),
  )
  const executionPressureChartOptions = computed(() => buildExecutionPressureChartOptions())

  return {
    taskStatusSnapshot,
    statusTotalCount,
    accountLoadRows,
    accountLoadCurrent,
    accountLoadCapacity,
    peakAccountLoadPercent,
    taskDistributionRows,
    activeExecutionPressurePoints,
    accountConcurrencyChartData,
    accountConcurrencyChartOptions,
    taskDistributionChartData,
    taskDistributionChartOptions,
    executionPressureChartData,
    executionPressureChartOptions,
  }
}

export function buildTaskStatusSnapshot(
  stats: MonitorStats | null | undefined,
): TaskStatusSnapshot {
  return {
    [ListAdminTasksStatus.pending]: stats?.pending ?? 0,
    [ListAdminTasksStatus.running]: stats?.running ?? 0,
    [ListAdminTasksStatus.completed]: stats?.completed ?? 0,
    [ListAdminTasksStatus.failed]: stats?.failed ?? 0,
    [ListAdminTasksStatus.cancelled]: stats?.cancelled ?? 0,
  }
}

function getTaskStatusTotalCount(snapshot: TaskStatusSnapshot) {
  return Object.values(snapshot).reduce((sum, count) => sum + count, 0)
}

export function buildAccountLoadRows(
  accountLoads: MonitorData['account_loads'] | null | undefined,
): AccountLoadRow[] {
  return Object.entries(accountLoads || {})
    .map(([id, load]) => {
      const current = Math.max(0, Number(load.current_concurrent) || 0)
      const max = Math.max(1, Number(load.max_concurrent) || 0)
      const percent = Math.min(100, Math.round((current / max) * 100))
      const available = Math.max(0, max - current)
      return {
        id,
        current,
        max,
        available,
        percent,
        color: getAccountConcurrencyColor(percent),
      }
    })
    .sort((a, b) => b.percent - a.percent)
}

export function buildTaskDistributionRows(snapshot: TaskStatusSnapshot): TaskDistributionRow[] {
  const totalCount = getTaskStatusTotalCount(snapshot)
  return taskDistributionStatuses.map((status) => {
    const count = Math.max(0, Number(snapshot[status.key]) || 0)
    return {
      key: status.key,
      label: status.label,
      count,
      sharePercent: totalCount > 0 ? (count / totalCount) * 100 : 0,
    }
  })
}

function buildAccountConcurrencyChartData(rows: AccountLoadRow[]): ChartData {
  if (rows.length <= 0) {
    return defineChartData({
      labels: ['无账号'],
      datasets: [
        {
          type: 'bar',
          label: '占用',
          data: [0],
          backgroundColor: [withAlpha(adminChartTheme.emptyText, 0.18)],
          borderWidth: 0,
        },
        {
          type: 'bar',
          label: '可用',
          data: [0],
          backgroundColor: [withAlpha(adminChartTheme.mist, 0.12)],
          borderWidth: 0,
        },
      ],
    })
  }

  return defineChartData({
    labels: rows.map((row) => `账号 ${row.id}`),
    datasets: [
      {
        type: 'bar',
        label: '占用',
        data: rows.map((row) => row.percent),
        backgroundColor: rows.map((row) => row.color),
        borderColor: rows.map((row) => row.color),
        borderWidth: adminChartStrokeWidth,
        borderRadius: 5,
        maxBarThickness: 14,
      },
      {
        type: 'bar',
        label: '可用',
        data: rows.map((row) => Math.max(0, 100 - row.percent)),
        backgroundColor: withAlpha(adminChartTheme.mist, 0.14),
        borderColor: withAlpha(adminChartTheme.mist, 0.28),
        borderWidth: adminChartStrokeWidth,
        borderRadius: 5,
        maxBarThickness: 14,
      },
    ],
  })
}

function buildAccountConcurrencyChartOptions(rows: AccountLoadRow[]): ChartOptions {
  return defineAdminChartOptions({
    responsive: true,
    maintainAspectRatio: false,
    indexAxis: 'y',
    scales: {
      x: {
        ...defineAdminChartPercentScale({
          stepSize: 20,
          borderColor: adminChartTheme.axisLine,
        }),
        stacked: true,
      },
      y: {
        stacked: true,
        ticks: {
          color: adminChartTheme.axisText,
        },
        grid: { display: false },
        border: { display: false },
      },
    },
    plugins: {
      legend: adminChartLegend('bottom'),
      tooltip: {
        ...adminMonitorTooltipBase,
        callbacks: {
          label: (context) => {
            const row = rows[context.dataIndex]
            if (!row) return '暂无账号并发'
            if (context.dataset.label === '可用') return `可用: ${row.available}/${row.max}`
            return `占用: ${row.current}/${row.max}（${row.percent}%）`
          },
        },
      },
    },
  })
}

export function buildTaskDistributionChartData(rows: TaskDistributionRow[]): ChartData {
  return defineChartData({
    labels: rows.map((point) => point.label),
    datasets: [
      {
        type: 'radar',
        label: '任务占比',
        data: rows.map((point) => point.sharePercent),
        backgroundColor: withAlpha(adminChartTheme.danger, 0.12),
        borderColor: withAlpha(adminChartTheme.danger, 0.72),
        borderWidth: adminChartStrokeWidth,
        ...defineAdminChartPointClipStyle(),
        pointBackgroundColor: withAlpha(adminChartTheme.danger, 0.72),
        pointBorderColor: withAlpha(adminChartTheme.danger, 0.72),
        pointHoverBackgroundColor: adminChartTheme.danger,
        pointRadius: 3,
        pointHoverRadius: 5,
      },
    ],
  })
}

function buildTaskDistributionChartOptions(rows: TaskDistributionRow[]): ChartOptions {
  return defineAdminChartOptions({
    responsive: true,
    maintainAspectRatio: false,
    scales: {
      r: {
        ...defineAdminChartRadialPercentScale({ stepSize: 20 }),
      },
    },
    plugins: {
      legend: { display: false },
      tooltip: {
        ...adminMonitorTooltipBase,
        callbacks: {
          title: (items) => rows[items[0]?.dataIndex ?? -1]?.label || '',
          label: (context) => {
            const point = rows[context.dataIndex]
            if (!point) return []
            return [`占比: ${formatPercentValue(point.sharePercent)}`, `任务量: ${point.count} 条`]
          },
        },
      },
    },
  })
}

export function buildExecutionPressureChartData(points: TaskExecutionPressurePoint[]): ChartData {
  const maxCount = Math.max(1, ...points.map((point) => point.count))

  return defineChartData({
    datasets: points.map((point) => ({
      type: 'bubble' as const,
      label: point.label,
      data:
        point.count > 0
          ? ([
              {
                ...point,
                x: point.durationPressure,
                y: point.anomalyPressure,
                r: getAdminChartBubbleRadius(point.count, maxCount),
              },
            ] satisfies ExecutionPressureChartPoint[])
          : [],
      ...defineAdminChartBubbleDatasetStyle(executionPressureColorByKey[point.key]),
    })),
  })
}

function buildExecutionPressureChartOptions(): ChartOptions {
  return defineAdminChartOptions({
    responsive: true,
    maintainAspectRatio: false,
    scales: {
      x: {
        ...defineAdminChartPercentScale({
          stepSize: 10,
          borderColor: adminChartTheme.axisLine,
        }),
      },
      y: {
        ...defineAdminChartPercentScale({
          stepSize: 10,
          borderColor: adminChartTheme.axisLine,
        }),
      },
    },
    plugins: {
      legend: adminChartLegend(),
      tooltip: {
        ...adminMonitorTooltipBase,
        callbacks: {
          title: (items) => {
            const raw = items[0]?.raw as ExecutionPressureChartPoint | undefined
            return raw?.label || ''
          },
          label: (context) => {
            const raw = context.raw as ExecutionPressureChartPoint | undefined
            if (!raw) return []
            return [
              `任务量: ${raw.count} 条`,
              `耗时压力: ${formatPercentValue(raw.durationPressure)}`,
              `异常压力: ${formatPercentValue(raw.anomalyPressure)}`,
              `影响占比: ${formatPercentValue(raw.sharePercent)}`,
              `平均耗时: ${formatSeconds(Math.round(raw.avgDurationSeconds))}`,
              `最长耗时: ${formatSeconds(Math.round(raw.maxDurationSeconds))}`,
              `失败数: ${raw.failedCount} 条`,
              `重试次数: ${raw.retryCount}`,
            ]
          },
        },
      },
    },
  })
}

function formatSeconds(seconds: number) {
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  const rest = seconds % 60
  if (minutes < 60) return rest === 0 ? `${minutes}m` : `${minutes}m ${rest}s`
  const hours = Math.floor(minutes / 60)
  const minuteRest = minutes % 60
  return minuteRest === 0 ? `${hours}h` : `${hours}h ${minuteRest}m`
}

function formatPercentValue(value: number) {
  return `${value.toFixed(1)}%`
}

function getAccountConcurrencyColor(percent: number) {
  if (percent >= 90) return adminChartTheme.danger
  if (percent >= 70) return adminChartTheme.warning
  return adminChartTheme.info
}
