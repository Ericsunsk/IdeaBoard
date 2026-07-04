import { describe, expect, it } from 'vitest'
import {
  buildAccountLoadRows,
  buildExecutionPressureChartData,
  buildTaskDistributionChartData,
  buildTaskDistributionRows,
  buildTaskStatusSnapshot,
} from './useTaskMonitorCharts'
import {
  normalizeMonitorExecutionPressurePoints,
  type TaskExecutionSignalKey,
} from './taskMonitorExecutionPressure'
import { adminChartBubbleClipPadding, adminChartPointClipPadding } from '../shared/lib/adminChart'

describe('useTaskMonitorCharts builders', () => {
  it('keeps stable empty execution pressure buckets when monitor aggregation is empty', () => {
    const points = normalizeMonitorExecutionPressurePoints([])

    expect(getExecutionPressureBucket(points, 'fast_completed')?.count).toBe(0)
    expect(points.every((point) => point.count === 0)).toBe(true)
  })

  it('maps monitor aggregation into the fixed execution pressure bucket order', () => {
    const points = normalizeMonitorExecutionPressurePoints([
      {
        key: 'normal_running',
        label: '正常执行',
        count: 3,
        share_percent: 30,
        duration_pressure: 40,
        anomaly_pressure: 32,
        avg_duration_seconds: 18,
        max_duration_seconds: 42,
        failed_count: 0,
        retry_count: 1,
      },
    ])

    expect(getExecutionPressureBucket(points, 'normal_running')).toMatchObject({
      count: 3,
      sharePercent: 30,
      retryCount: 1,
    })
    expect(getExecutionPressureBucket(points, 'fast_completed')?.count).toBe(0)
  })

  it('builds stable task distribution rows from monitor stats', () => {
    const snapshot = buildTaskStatusSnapshot({
      pending: 2,
      running: 1,
      completed: 5,
      failed: 2,
      cancelled: 0,
    })

    const rows = buildTaskDistributionRows(snapshot)

    expect(rows.map((row) => row.label)).toEqual(['等待中', '执行中', '已完成', '失败', '已取消'])
    expect(rows.find((row) => row.key === 'completed')).toMatchObject({
      count: 5,
      sharePercent: 50,
    })
  })

  it('normalizes account loads into descending concurrency rows', () => {
    const rows = buildAccountLoadRows({
      low: { current_concurrent: 1, max_concurrent: 4 },
      saturated: { current_concurrent: 12, max_concurrent: 10 },
      empty: { current_concurrent: -1, max_concurrent: 0 },
    })

    expect(rows.map((row) => row.id)).toEqual(['saturated', 'low', 'empty'])
    expect(rows[0]).toMatchObject({
      current: 12,
      max: 10,
      available: 0,
      percent: 100,
    })
    expect(rows[2]).toMatchObject({
      current: 0,
      max: 1,
      available: 1,
      percent: 0,
    })
  })

  it('encodes execution pressure points as visible bubble datasets only when count is positive', () => {
    const chartData = buildExecutionPressureChartData([
      {
        key: 'normal_running',
        label: '正常执行',
        count: 4,
        sharePercent: 40,
        durationPressure: 30,
        anomalyPressure: 20,
        avgDurationSeconds: 12,
        maxDurationSeconds: 28,
        failedCount: 0,
        retryCount: 1,
      },
      {
        key: 'fast_completed',
        label: '快速完成',
        count: 0,
        sharePercent: 0,
        durationPressure: 0,
        anomalyPressure: 0,
        avgDurationSeconds: 0,
        maxDurationSeconds: 0,
        failedCount: 0,
        retryCount: 0,
      },
    ])

    expect(chartData.datasets).toHaveLength(2)
    expect(chartData.datasets[0]).toMatchObject({ clip: adminChartBubbleClipPadding })
    expect(chartData.datasets[0]?.data).toEqual([
      expect.objectContaining({ x: 30, y: 20, count: 4 }),
    ])
    expect(chartData.datasets[1]?.data).toEqual([])
  })

  it('allows radar points to render past chart boundaries without changing scale', () => {
    const rows = buildTaskDistributionRows(
      buildTaskStatusSnapshot({
        pending: 0,
        running: 0,
        completed: 10,
        failed: 0,
        cancelled: 0,
      }),
    )

    const chartData = buildTaskDistributionChartData(rows)

    expect(chartData.datasets[0]).toMatchObject({ clip: adminChartPointClipPadding })
  })
})

function getExecutionPressureBucket(
  points: ReturnType<typeof normalizeMonitorExecutionPressurePoints>,
  key: TaskExecutionSignalKey,
) {
  return points.find((point) => point.key === key)
}
