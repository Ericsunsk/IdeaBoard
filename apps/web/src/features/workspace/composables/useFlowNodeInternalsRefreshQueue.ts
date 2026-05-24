type UpdateNodeInternals = (nodeIds: string[]) => void

type UseFlowNodeInternalsRefreshQueueOptions = {
  updateNodeInternals: UpdateNodeInternals
}

function runNextFrame(callback: () => void) {
  if (typeof requestAnimationFrame === 'function') {
    requestAnimationFrame(callback)
    return
  }
  setTimeout(callback, 0)
}

export function useFlowNodeInternalsRefreshQueue(options: UseFlowNodeInternalsRefreshQueueOptions) {
  let pendingNodeInternalsRefresh = false
  let queuedNodeInternalsIds: Set<string> | null = null

  function readQueuedNodeInternalsRefreshIds(): string[] {
    const ids = queuedNodeInternalsIds ? Array.from(queuedNodeInternalsIds) : []
    queuedNodeInternalsIds = null
    return ids
  }

  function scheduleFlowNodeInternalsRefresh(nodeIds: string[]) {
    const nextIds = nodeIds.filter((id) => typeof id === 'string' && id.length > 0)
    if (nextIds.length === 0) return
    if (!queuedNodeInternalsIds) queuedNodeInternalsIds = new Set()
    nextIds.forEach((id) => queuedNodeInternalsIds?.add(id))
    if (pendingNodeInternalsRefresh) return
    pendingNodeInternalsRefresh = true
    void nextTick(() => {
      runNextFrame(() => {
        const ids = readQueuedNodeInternalsRefreshIds()
        if (ids.length > 0) {
          options.updateNodeInternals(ids)
        }
        const pendingIds = readQueuedNodeInternalsRefreshIds()
        pendingNodeInternalsRefresh = false
        if (pendingIds.length > 0) {
          scheduleFlowNodeInternalsRefresh(pendingIds)
        }
      })
    })
  }

  return {
    scheduleFlowNodeInternalsRefresh,
  }
}
