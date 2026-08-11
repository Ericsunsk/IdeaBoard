import { describe, expect, it, vi } from 'vitest'
import { createAdminCreditFindingQueueResource } from './adminCreditFindingQueueResource'

describe('admin credit finding queue resource', () => {
  it('keeps the global keyset request behind the feature resource adapter', async () => {
    const listReconciliationFindingQueue = vi.fn().mockResolvedValue({ findings: [] })
    const resource = createAdminCreditFindingQueueResource({
      listReconciliationFindingQueue,
    } as never)

    await resource.list({ status: 'open', beforeId: 41, signal: new AbortController().signal })

    expect(listReconciliationFindingQueue).toHaveBeenCalledWith(
      { status: 'open', before_id: 41, limit: 25 },
      { signal: expect.any(AbortSignal) },
    )
  })
})
