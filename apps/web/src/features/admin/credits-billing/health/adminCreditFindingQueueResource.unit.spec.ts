import { ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const useQueryMock = vi.hoisted(() => vi.fn())

vi.mock('@tanstack/vue-query', async (importOriginal) => {
  const original = await importOriginal<typeof import('@tanstack/vue-query')>()
  return { ...original, useQuery: useQueryMock }
})

import { useAdminCreditFindingQueueResource } from './adminCreditFindingQueueResource'

describe('admin credit finding queue resource', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useQueryMock.mockReturnValue({
      data: ref(undefined),
      isLoading: ref(false),
      isFetching: ref(false),
      isPlaceholderData: ref(false),
      dataUpdatedAt: ref(0),
    })
  })

  it('uses the global keyset query and refreshes the active view every five seconds', async () => {
    const listReconciliationFindingQueue = vi.fn().mockResolvedValue({ findings: [] })
    const status = ref<'open' | 'resolved'>('open')
    const beforeId = ref<number | undefined>(41)

    useAdminCreditFindingQueueResource(
      { status, beforeId },
      { api: { listReconciliationFindingQueue } as never },
    )

    const options = useQueryMock.mock.calls[0]?.[0]
    const queryKey = options.queryKey.value
    expect(options.refetchInterval({ queryKey })).toBe(5_000)
    await options.queryFn({ queryKey, signal: new AbortController().signal })

    expect(listReconciliationFindingQueue).toHaveBeenCalledWith(
      { status: 'open', before_id: 41, limit: 25 },
      { signal: expect.any(AbortSignal) },
    )

    status.value = 'resolved'
    expect(options.refetchInterval({ queryKey: options.queryKey.value })).toBe(30_000)
  })
})
