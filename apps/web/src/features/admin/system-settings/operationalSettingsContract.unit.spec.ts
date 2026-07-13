import { describe, expect, it } from 'vitest'
import {
  DEFAULT_OPERATIONAL_SETTINGS_FORM,
  normalizeOperationalSettingsForm,
  operationalSettingsFormToRequest,
  validateOperationalSettingsForm,
} from './operationalSettingsContract'

describe('operational settings form contract', () => {
  it('normalizes configured settings into an editable allow-hosts list', () => {
    expect(
      normalizeOperationalSettingsForm({
        task_runtime: {
          worker_concurrency: 8,
          task_timeout_seconds: 240,
          no_capacity_retry_seconds: 12,
          max_retry: 30,
        },
        outbound_policy: {
          allow_hosts: ['models.internal', '10.0.0.8:8443'],
        },
        event_retention: { nats_retention_days: 14 },
      }),
    ).toEqual({
      task_runtime: {
        worker_concurrency: 8,
        task_timeout_seconds: 240,
        no_capacity_retry_seconds: 12,
        max_retry: 30,
      },
      outbound_allow_hosts: 'models.internal\n10.0.0.8:8443',
      nats_retention_days: 14,
      reason: '',
    })
  })

  it('trims and deduplicates allow hosts before submission', () => {
    expect(
      operationalSettingsFormToRequest({
        ...DEFAULT_OPERATIONAL_SETTINGS_FORM,
        outbound_allow_hosts: ' Models.Internal \n\nmodels.internal\n10.0.0.8:8443 ',
        reason: '  扩容并更新模型出口  ',
      }),
    ).toEqual({
      task_runtime: {
        worker_concurrency: 5,
        task_timeout_seconds: 180,
        no_capacity_retry_seconds: 5,
        max_retry: 25,
      },
      outbound_policy: {
        allow_hosts: ['models.internal', '10.0.0.8:8443'],
      },
      event_retention: { nats_retention_days: 7 },
      reason: '扩容并更新模型出口',
    })
  })

  it('returns user-readable validation errors for runtime limits, unsafe hosts, and reason', () => {
    expect(
      validateOperationalSettingsForm({
        ...DEFAULT_OPERATIONAL_SETTINGS_FORM,
        task_runtime: {
          ...DEFAULT_OPERATIONAL_SETTINGS_FORM.task_runtime,
          worker_concurrency: 0,
        },
        reason: '例行调整',
      }),
    ).toBe('Worker 并发必须在 1 到 64 之间')

    expect(
      validateOperationalSettingsForm({
        ...DEFAULT_OPERATIONAL_SETTINGS_FORM,
        outbound_allow_hosts: 'https://models.internal',
        reason: '例行调整',
      }),
    ).toBe('出站白名单仅支持 hostname、IP 或 host:port，每行一个')

    expect(
      validateOperationalSettingsForm({
        ...DEFAULT_OPERATIONAL_SETTINGS_FORM,
        reason: '   ',
      }),
    ).toBe('请输入操作原因')
  })
})
