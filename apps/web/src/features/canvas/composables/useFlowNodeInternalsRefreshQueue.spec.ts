import { effectScope, nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useFlowNodeInternalsRefreshQueue } from './useFlowNodeInternalsRefreshQueue'

describe('useFlowNodeInternalsRefreshQueue', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.stubGlobal('requestAnimationFrame', undefined)
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('dedupes queued node ids and refreshes once per frame', async () => {
    const updateNodeInternals = vi.fn()
    const { scheduleFlowNodeInternalsRefresh } = useFlowNodeInternalsRefreshQueue({
      updateNodeInternals,
    })

    scheduleFlowNodeInternalsRefresh(['node-1', 'node-2'])
    scheduleFlowNodeInternalsRefresh(['node-2', '', 'node-3'])
    await nextTick()
    vi.runOnlyPendingTimers()

    expect(updateNodeInternals).toHaveBeenCalledTimes(1)
    expect(updateNodeInternals).toHaveBeenCalledWith(['node-1', 'node-2', 'node-3'])
  })

  it('does not refresh after the owning scope is disposed', async () => {
    const updateNodeInternals = vi.fn()
    const scope = effectScope()

    scope.run(() => {
      const { scheduleFlowNodeInternalsRefresh } = useFlowNodeInternalsRefreshQueue({
        updateNodeInternals,
      })
      scheduleFlowNodeInternalsRefresh(['node-1'])
    })

    await nextTick()
    scope.stop()
    vi.runOnlyPendingTimers()

    expect(updateNodeInternals).not.toHaveBeenCalled()
  })
})
