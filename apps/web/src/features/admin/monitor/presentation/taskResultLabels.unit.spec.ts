import { describe, expect, it } from 'vitest'
import {
  formatAttemptLatency,
  getAttemptFailureLabel,
  getAttemptProviderLabel,
  getAttemptStageLabel,
  getAttemptStatusLabel,
  getAttemptStatusSeverity,
  getBillingSettlementLabel,
  getDetailExecutionModelLabel,
  getDetailExecutionProviderLabel,
} from './taskResultLabels'

describe('taskResultLabels', () => {
  it('labels execution provider and model with fallbacks', () => {
    expect(getDetailExecutionProviderLabel({} as never)).toBe('-')
    expect(
      getDetailExecutionProviderLabel({ latest_execution: { provider: 'aigateway' } } as never),
    ).toBe('aigateway')
    expect(getDetailExecutionModelLabel({} as never)).toBe('-')
    expect(
      getDetailExecutionModelLabel({ latest_execution: { resolved_model: 'm' } } as never),
    ).toBe('m')
    expect(getAttemptProviderLabel({ provider: '', resolved_model: '' } as never)).toBe('模型 未知')
  })

  it('maps attempt status, stage, and severity with passthrough fallback', () => {
    expect(getAttemptStatusLabel('failed')).toBe('失败')
    expect(getAttemptStatusLabel('mystery')).toBe('mystery')
    expect(getAttemptStatusSeverity('failed')).toBe('danger')
    expect(getAttemptStatusSeverity('mystery')).toBe('secondary')
    expect(getAttemptStageLabel('queued')).toBe('排队中')
    expect(getAttemptStageLabel('mystery')).toBe('mystery')
  })

  it('maps failure categories and billing states', () => {
    expect(getAttemptFailureLabel('')).toBe('-')
    expect(getAttemptFailureLabel('quota')).toBe('配额')
    expect(getAttemptFailureLabel('mystery')).toBe('mystery')
    expect(getBillingSettlementLabel('captured')).toBe('已扣费')
    expect(getBillingSettlementLabel('mystery')).toBe('mystery')
  })

  it('formats attempt latency across millisecond and second ranges', () => {
    expect(formatAttemptLatency(0)).toBe('-')
    expect(formatAttemptLatency(250)).toBe('250 ms')
    expect(formatAttemptLatency(1500)).toBe('1.5 s')
    expect(formatAttemptLatency(12000)).toBe('12 s')
  })
})
