import { describe, expect, it } from 'vitest'
import {
  normalizeMonitorExecutionPressurePoints,
  type TaskExecutionSignalKey,
} from './taskMonitorExecutionPressure'

describe('task monitor execution pressure source', () => {
  it('keeps stable empty buckets when monitor aggregation is empty', () => {
    const points = normalizeMonitorExecutionPressurePoints([])

    expect(getBucket(points, 'fast_completed')?.count).toBe(0)
    expect(points.every((point) => point.count === 0)).toBe(true)
  })

  it('maps monitor aggregation into the fixed chart bucket order', () => {
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

    expect(getBucket(points, 'normal_running')).toMatchObject({
      count: 3,
      sharePercent: 30,
      retryCount: 1,
    })
    expect(getBucket(points, 'fast_completed')?.count).toBe(0)
  })
})

function getBucket(
  points: ReturnType<typeof normalizeMonitorExecutionPressurePoints>,
  key: TaskExecutionSignalKey,
) {
  return points.find((point) => point.key === key)
}
