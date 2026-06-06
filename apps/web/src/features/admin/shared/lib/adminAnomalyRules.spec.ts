import { describe, expect, it } from 'vitest'
import { buildAdminAnomalyItems } from './adminAnomalyRules'
import type {
  AdminStatistics,
  MonitorData,
  MonitorStats,
} from '../../../../shared/api/generated-query/model'

type AdminAnomalyInput = Parameters<typeof buildAdminAnomalyItems>[0]

const TASK_MONITOR_ROUTE = '/admin/task-monitor'
const STATISTICS_ROUTE = '/admin/statistics'

describe('admin anomaly rules', () => {
  it('reports queue backlog as a task monitor anomaly', () => {
    const anomalies = buildAdminAnomalyItems(
      createInput({
        monitorData: createMonitorData({
          global_queue_len: 42,
        }),
      }),
    )

    expect(anomalies).toContainEqual(
      expect.objectContaining({
        id: 'runtime.queue-backlog',
        route: TASK_MONITOR_ROUTE,
        metric: '42 个等待任务',
      }),
    )
  })

  it('reports high model failure rate as a statistics anomaly', () => {
    const anomalies = buildAdminAnomalyItems(
      createInput({
        statistics: createStatistics({
          model_health: [
            {
              name: 'GPT Image',
              total: 30,
              completed: 24,
              failed: 6,
              failure_rate: 20,
              avg_seconds: 12,
            },
          ],
        }),
      }),
    )

    expect(anomalies).toContainEqual(
      expect.objectContaining({
        id: 'provider.model-failure-gpt-image',
        severity: 'critical',
        route: STATISTICS_ROUTE,
      }),
    )
  })

  it('reports concentrated bottleneck and failure categories', () => {
    const anomalies = buildAdminAnomalyItems(
      createInput({
        statistics: createStatistics({
          bottleneck_sources: [
            {
              source: 'provider',
              label: '模型服务',
              total: 70,
              failed: 12,
              avg_latency_ms: 18_000,
            },
            {
              source: 'storage',
              label: '对象存储',
              total: 30,
              failed: 2,
              avg_latency_ms: 3_000,
            },
          ],
          failure_categories: [
            {
              category: 'provider_error',
              label: '模型异常',
              total: 55,
              avg_latency_ms: 14_000,
            },
            {
              category: 'timeout',
              label: '超时',
              total: 45,
              avg_latency_ms: 31_000,
            },
          ],
        }),
      }),
    )

    expect(anomalies.map((item) => item.id)).toEqual(
      expect.arrayContaining([
        'statistics.bottleneck-provider',
        'statistics.failure-category-provider_error',
      ]),
    )
  })

  it('does not report anomalies for healthy inputs', () => {
    const anomalies = buildAdminAnomalyItems(
      createInput({
        monitorData: createMonitorData(),
        monitorStats: createMonitorStats(),
        statistics: createStatistics(),
      }),
    )

    expect(anomalies).toEqual([])
  })
})

function createInput(overrides: Partial<AdminAnomalyInput> = {}): AdminAnomalyInput {
  return {
    monitorData: null,
    monitorStats: null,
    statistics: null,
    isMonitorDegraded: false,
    isMonitorUnavailable: false,
    isStatisticsUnavailable: false,
    now: 1_700_000_000_000,
    ...overrides,
  }
}

function createMonitorData(overrides: Partial<MonitorData> = {}): MonitorData {
  return {
    global_queue_len: 0,
    wait_queue_len: 0,
    running_tasks: 0,
    account_loads: {},
    ...overrides,
  }
}

function createMonitorStats(overrides: Partial<MonitorStats> = {}): MonitorStats {
  return {
    pending: 0,
    running: 0,
    completed: 10,
    failed: 0,
    ...overrides,
  }
}

function createStatistics(overrides: Partial<AdminStatistics> = {}): AdminStatistics {
  return {
    summary: {
      total_calls: 100,
      today_calls: 20,
      today_success: 19,
      today_failed: 1,
      yesterday_calls: 18,
      yesterday_success: 17,
      yesterday_failed: 1,
      this_month_calls: 100,
      pending: 0,
      running: 0,
      completed: 95,
      failed: 5,
    },
    trend: [],
    model_health: [],
    quality_trend: [
      {
        date: '2026-06-06',
        label: '00:00',
        total: 20,
        success: 19,
        fail: 1,
        success_rate: 95,
      },
    ],
    slow_models: [],
    bottleneck_sources: [],
    failure_categories: [],
    backfill_status: {
      range_start: '2026-06-06',
      range_end: '2026-06-06',
      missing_days: 0,
      backfilled_days: 0,
    },
    ...overrides,
  }
}
