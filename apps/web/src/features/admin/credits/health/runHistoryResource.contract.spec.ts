import { describe, expect, it, vi } from 'vitest'
import { createAdminCreditRunHistoryResource } from './runHistoryResource'
import { makeReconciliationRun } from './testing'

describe('Admin Credit Run history resource', () => {
  it('keeps run list and detail requests behind the feature resource adapter', async () => {
    const listReconciliationRuns = vi
      .fn()
      .mockResolvedValue({ runs: [makeReconciliationRun({ id: 21 })] })
    const getReconciliationRun = vi
      .fn()
      .mockResolvedValue(makeReconciliationRun({ id: 21, reason: '人工复核' }))
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
