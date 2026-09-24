import { describe, expect, it } from 'vitest'
import {
  getTaskMetricByStatus,
  taskMetricDefinitions,
  monitorChartSpecs,
  ListAdminTasksStatus,
  type TaskMetricStatsSnapshot,
} from './taskMetricTokens'

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
      pending: ListAdminTasksStatus.pending,
      running: ListAdminTasksStatus.running,
      completed: ListAdminTasksStatus.completed,
      failed: ListAdminTasksStatus.failed,
      cancelled: ListAdminTasksStatus.cancelled,
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
    expect(getTaskMetricByStatus('pending')?.chartColor).toBe('#4775a8')
    expect(getTaskMetricByStatus('running')?.chartColor).toBe('#d48830')
    expect(getTaskMetricByStatus('completed')?.chartColor).toBe('#2f9e73')
    expect(getTaskMetricByStatus('failed')?.chartColor).toBe('#d14d5a')
  })
})
