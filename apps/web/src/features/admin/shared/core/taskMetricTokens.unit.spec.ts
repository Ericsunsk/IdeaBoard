import { describe, expect, it } from 'vitest'
import {
  getTaskMetricByStatus,
  taskMetricDefinitions,
  monitorChartSpecs,
  TaskStatus,
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
    expect(getTaskMetricByStatus('pending')?.chartColorRole).toBe('info')
    expect(getTaskMetricByStatus('running')?.chartColorRole).toBe('warning')
    expect(getTaskMetricByStatus('completed')?.chartColorRole).toBe('success')
    expect(getTaskMetricByStatus('failed')?.chartColorRole).toBe('danger')
  })

  it('keeps filter pills neutral while chart lines retain semantic colors', () => {
    // 过滤 pills 是中性分类控件：pending/running/completed/failed 的圆点与激活态
    // 一律中性（bg-muted 载体），不蹭状态色；图表线条仍保留语义色。
    const neutralActive = 'bg-muted text-foreground font-semibold ring-1 ring-border'
    for (const key of ['pending', 'running', 'completed', 'failed'] as const) {
      const metric = taskMetricDefinitions.find((item) => item.key === key)!
      expect(metric.dotClass).toBe('bg-muted-foreground')
      expect(metric.activePillClass).toBe(neutralActive)
      expect(metric.chartColorRole).toBeTruthy()
    }
  })
})
