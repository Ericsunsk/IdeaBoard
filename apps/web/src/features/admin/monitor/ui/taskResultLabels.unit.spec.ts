import { describe, expect, it } from 'vitest'
import {
  formatAttemptLatency,
  getAttemptFailureLabel,
  getAttemptProviderLabel,
  getAttemptRetryLayerLabel,
  getAttemptStageLabel,
  getAttemptStatusLabel,
  getAttemptStatusTone,
  getBillingSettlementLabel,
  getDetailExecutionModelLabel,
  getDetailExecutionProviderLabel,
  getDetailFinalModelLabel,
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

  it('maps attempt status, stage, and variant with passthrough fallback', () => {
    // 任务五态走 core/status 单源（英文，与表格同文）；requeued 等专有态保留中文。
    expect(getAttemptStatusLabel('failed')).toBe('Failed')
    expect(getAttemptStatusLabel('requeued')).toBe('已重试')
    expect(getAttemptStatusLabel('mystery')).toBe('mystery')
    expect(getAttemptStatusTone('failed')).toBe('danger')
    expect(getAttemptStatusTone('requeued')).toBe('warning')
    expect(getAttemptStatusTone('mystery')).toBe('neutral')
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

  it('labels retry layers distinctly with passthrough fallback', () => {
    expect(getAttemptRetryLayerLabel(undefined)).toBe('-')
    expect(getAttemptRetryLayerLabel('')).toBe('-')
    expect(getAttemptRetryLayerLabel('gateway')).toBe('网关请求')
    expect(getAttemptRetryLayerLabel('model_fallback')).toBe('模型兜底')
    expect(getAttemptRetryLayerLabel('river_task')).toBe('任务重投')
    expect(getAttemptRetryLayerLabel('mystery')).toBe('mystery')
  })

  it('prefers the final resolved model over the selected model', () => {
    expect(getDetailFinalModelLabel({} as never)).toBe('-')
    expect(
      getDetailFinalModelLabel({
        final_resolved_model: 'model-b',
        latest_execution: { resolved_model: 'model-a' },
      } as never),
    ).toBe('model-b')
    expect(
      getDetailFinalModelLabel({ latest_execution: { resolved_model: 'model-a' } } as never),
    ).toBe('model-a')
  })
})
