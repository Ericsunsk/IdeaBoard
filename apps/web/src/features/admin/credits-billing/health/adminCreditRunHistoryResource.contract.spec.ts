import { describe, expect, it, vi } from 'vitest'
import { createAdminCreditRunHistoryResource } from './adminCreditRunHistoryResource'

describe('Admin Credit Run history resource', () => {
  it('keeps run list and detail requests behind the feature resource adapter', async () => {
    const listReconciliationRuns = vi.fn().mockResolvedValue({ runs: [makeRun()] })
    const getReconciliationRun = vi.fn().mockResolvedValue(makeRun({ reason: '人工复核' }))
    const resource = createAdminCreditRunHistoryResource({
      listReconciliationRuns,
      getReconciliationRun,
    } as never)

    await resource.list({ beforeId: 21, signal: new AbortController().signal })
    await resource.get(21, new AbortController().signal)

    expect(listReconciliationRuns).toHaveBeenCalledWith(
      { before_id: 21, limit: 20 },
      { signal: expect.any(AbortSignal) },
    )
    expect(getReconciliationRun).toHaveBeenCalledWith(21, { signal: expect.any(AbortSignal) })
  })
})

function makeRun(overrides: Record<string, unknown> = {}) {
  return {
    id: 21,
    status: 'completed',
    trigger: 'scheduled',
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
