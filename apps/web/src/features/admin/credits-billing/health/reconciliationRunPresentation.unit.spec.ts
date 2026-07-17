import { describe, expect, it } from 'vitest'
import type { AdminCreditRun } from './adminCreditRunHistoryResource'
import {
  formatReconciliationRunDuration,
  formatReconciliationRunResult,
  getReconciliationTriggerLabel,
} from './reconciliationRunPresentation'

describe('Reconciliation Run presentation', () => {
  it('formats completed, active, and invalid durations safely', () => {
    expect(formatReconciliationRunDuration(makeRun())).toBe('3 秒')
    expect(formatReconciliationRunDuration(makeRun({ completed_at: undefined }))).toBe('进行中')
    expect(formatReconciliationRunDuration(makeRun({ started_at: undefined }))).toBe('—')
    expect(
      formatReconciliationRunDuration(
        makeRun({ started_at: 'invalid', completed_at: 'also-invalid' }),
      ),
    ).toBe('—')
  })

  it('keeps result and Trigger labels compact and deterministic', () => {
    const run = makeRun({ finding_count: 3, critical_count: 1, isolated_wallet_count: 2 })
    expect(formatReconciliationRunResult(run)).toBe('3 异常 · 1 严重 · 2 Wallet')
    expect(formatReconciliationRunResult(makeRun({ status: 'queued' }))).toBe('等待执行')
    expect(formatReconciliationRunResult(makeRun({ status: 'running' }))).toBe('检查中')
    expect(getReconciliationTriggerLabel(run)).toBe('自动调度')
    expect(getReconciliationTriggerLabel(makeRun({ trigger: 'manual' }))).toBe('人工触发')
  })
})

function makeRun(overrides: Partial<AdminCreditRun> = {}): AdminCreditRun {
  return {
    id: 11,
    status: 'completed',
    trigger: 'scheduled',
    stale_hold_after_seconds: 86400,
    finding_count: 0,
    critical_count: 0,
    isolated_wallet_count: 0,
    attempt_count: 1,
    requested_at: '2026-07-17T02:00:00Z',
    started_at: '2026-07-17T02:00:01Z',
    completed_at: '2026-07-17T02:00:04Z',
    ...overrides,
  }
}
