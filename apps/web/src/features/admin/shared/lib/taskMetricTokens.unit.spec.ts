import { describe, expect, it } from 'vitest'
import {
  getTaskMetricByStatus,
  taskMetricDefinitions,
  monitorChartSpecs,
  TaskStatus,
  type TaskMetricStatsSnapshot,
} from './taskMetricTokens'
import { adminThemeTokenFallbacks } from './adminThemeTokens'

describe('taskMetricTokens', () => {
  it('defines the 6 core observability metrics in canonical lifecycle order', () => {
    const keys = taskMetricDefinitions.map((item) => item.key)
    expect(keys).toEqual(['all', 'pending', 'running', 'completed', 'failed', 'cancelled'])
  })

  it('correctly maps status parameters for each metric', () => {
    const statusMap = Object.fromEntries(
      taskMetricDefinitions.map((item) => [item.key, item.status]),
    )
    expect(statusMap).toEqual({
      all: null,
      pending: TaskStatus.pending,
      running: TaskStatus.running,
      completed: TaskStatus.completed,
      failed: TaskStatus.failed,
      cancelled: TaskStatus.cancelled,
    })
  })

  it('defines chart lines only for granular status metrics and excludes all', () => {
    const chartLineMetrics = taskMetricDefinitions.filter((item) => item.hasChartLine)
    expect(chartLineMetrics.map((item) => item.key)).toEqual([
      'pending',
      'running',
      'completed',
      'failed',
      'cancelled',
    ])

    const allMetric = taskMetricDefinitions.find((item) => item.key === 'all')
    expect(allMetric?.hasChartLine).toBe(false)
  })

  it('correctly extracts count values from stats snapshot context', () => {
    const snapshot: TaskMetricStatsSnapshot = {
      total: 100,
      pending: 10,
      running: 20,
      completed: 60,
      failed: 5,
      cancelled: 5,
    }

    const counts = Object.fromEntries(
      taskMetricDefinitions.map((item) => [item.key, item.getCount(snapshot)]),
    )

    expect(counts).toEqual({
      all: 100,
      pending: 10,
      running: 20,
      completed: 60,
      failed: 5,
      cancelled: 5,
    })
  })

  it('provides sensible chart visual specs', () => {
    expect(monitorChartSpecs.heightPx).toBe(180)
    expect(monitorChartSpecs.lineWidth).toBe(1)
    expect(monitorChartSpecs.pointRadius).toBe(0)
    expect(monitorChartSpecs.pointHoverRadius).toBe(2)
    expect(monitorChartSpecs.fillOpacity).toBe(0.12)
  })

  it('correctly retrieves metric definition by status code', () => {
    expect(getTaskMetricByStatus('completed')?.key).toBe('completed')
    expect(getTaskMetricByStatus('failed')?.key).toBe('failed')
    expect(getTaskMetricByStatus('running')?.key).toBe('running')
    expect(getTaskMetricByStatus('pending')?.key).toBe('pending')
    expect(getTaskMetricByStatus('unknown_status')).toBeUndefined()
    expect(getTaskMetricByStatus(null)).toBeUndefined()
  })

  it('assigns unique and consistent chart colors matching status pill definitions', () => {
    // Chart colors resolve from the Admin theme token fallbacks, so this asserts
    // the shared source of truth rather than restating hex values.
    const { danger, info, success, warning } = adminThemeTokenFallbacks
    expect(getTaskMetricByStatus('pending')?.chartColor).toBe(info)
    expect(getTaskMetricByStatus('running')?.chartColor).toBe(warning)
    expect(getTaskMetricByStatus('completed')?.chartColor).toBe(success)
    expect(getTaskMetricByStatus('failed')?.chartColor).toBe(danger)
  })
})
