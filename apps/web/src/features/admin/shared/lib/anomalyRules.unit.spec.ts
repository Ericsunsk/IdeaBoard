import { describe, expect, it } from 'vitest'
import { buildAdminAnomalyItems } from './anomalyRules'
import type { AdminStatistics } from '../../statistics/resource'
import type { MonitorData, MonitorStats } from '../../monitor/infrastructure/resource'
import { ADMIN_STATISTICS_PATH, ADMIN_MONITOR_PATH } from '../../routePaths'

type AdminAnomalyInput = Parameters<typeof buildAdminAnomalyItems>[0]

const MONITOR_ROUTE = ADMIN_MONITOR_PATH
const STATISTICS_ROUTE = ADMIN_STATISTICS_PATH

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
        route: MONITOR_ROUTE,
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
    running_attempts: 0,
    execution_pressure: [],
    golden_signals: {
      window_seconds: 3600,
      sample_count: 0,
      provider_attempts: 0,
      provider_error_count: 0,
      provider_error_rate: 0,
      queue_delay_ms: { samples: 0, avg_ms: 0, p95_ms: 0, max_ms: 0 },
      provider_latency_ms: { samples: 0, avg_ms: 0, p95_ms: 0, max_ms: 0 },
      finalize_latency_ms: { samples: 0, avg_ms: 0, p95_ms: 0, max_ms: 0 },
    },
    ...overrides,
  }
}

function createMonitorStats(overrides: Partial<MonitorStats> = {}): MonitorStats {
  return {
    pending: 0,
    running: 0,
    completed: 10,
    failed: 0,
    cancelled: 0,
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
      today_cancelled: 0,
      yesterday_calls: 18,
      yesterday_success: 17,
      yesterday_failed: 1,
      yesterday_cancelled: 0,
      this_month_calls: 100,
      this_month_cancelled: 0,
      pending: 0,
      running: 0,
      completed: 95,
      failed: 5,
      cancelled: 0,
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
