import type { NodeChange } from '@vue-flow/core'
import {
  stageCanvasGraphNodeDragIntent,
  type CanvasGraphTransaction,
} from '../graph-runtime'

export type CanvasNodeDragChange =
  | Extract<NodeChange, { type: 'position' }>
  | Extract<NodeChange, { type: 'select' }>

export type CanvasNodeDragLifecycle = {
  begin: () => void
  handleChanges: (changes: NodeChange[]) => boolean
  commit: () => void
  cancel: () => void
  isActive: () => boolean
}

export type CanvasNodeDragLifecycleOptions<HistorySnapshot> = {
  createTransaction: () => CanvasGraphTransaction
  projectChanges: (changes: CanvasNodeDragChange[]) => void
  syncChanges: (changes: CanvasNodeDragChange[]) => void
  captureHistory: () => HistorySnapshot
  commitHistory: (before: HistorySnapshot) => boolean
  advanceLocalEditVersion: () => void
  markDirtyAndScheduleAutosave: () => void
  onBegin: () => void
  reconcileMembership: (changes: CanvasNodeDragChange[]) => void
  scheduleAnimationFrame?: (callback: FrameRequestCallback) => number
  cancelAnimationFrame?: (handle: number) => void
}

function isNodeDragChange(change: NodeChange): change is CanvasNodeDragChange {
  return change.type === 'position' || change.type === 'select'
}

function mergeNodeDragChanges(
  current: CanvasNodeDragChange[],
  incoming: CanvasNodeDragChange[],
): CanvasNodeDragChange[] {
  const changesByKey = new Map<string, CanvasNodeDragChange>()
  current.forEach((change) => changesByKey.set(`${change.type}:${change.id}`, change))
  incoming.forEach((change) => changesByKey.set(`${change.type}:${change.id}`, change))
  return [...changesByKey.values()]
}

export function createCanvasNodeDragLifecycle<HistorySnapshot>(
  options: CanvasNodeDragLifecycleOptions<HistorySnapshot>,
): CanvasNodeDragLifecycle {
  const scheduleAnimationFrame =
    options.scheduleAnimationFrame ?? ((callback) => window.requestAnimationFrame(callback))
  const cancelAnimationFrame =
    options.cancelAnimationFrame ?? ((handle) => window.cancelAnimationFrame(handle))

  let activeDrag: {
    transaction: CanvasGraphTransaction
    historyBefore: HistorySnapshot
  } | null = null
  let pendingProjectionChanges: CanvasNodeDragChange[] = []
  let latestChanges: CanvasNodeDragChange[] = []
  let pendingFrameId: number | null = null
  let hasAdvancedLocalEdit = false

  function isActive() {
    return activeDrag !== null
  }

  function clearPendingFrame() {
    if (pendingFrameId === null) return
    cancelAnimationFrame(pendingFrameId)
    pendingFrameId = null
  }

  function flushProjection() {
    clearPendingFrame()
    if (pendingProjectionChanges.length === 0) return
    const changes = pendingProjectionChanges
    pendingProjectionChanges = []
    options.projectChanges(changes)
  }

  function syncProjection() {
    if (latestChanges.length === 0) return
    const changes = latestChanges
    latestChanges = []
    options.syncChanges(changes)
  }

  function reset() {
    pendingProjectionChanges = []
    latestChanges = []
    hasAdvancedLocalEdit = false
    clearPendingFrame()
    activeDrag = null
  }

  function cancel() {
    const drag = activeDrag
    if (!drag) return
    try {
      drag.transaction.cancel()
    } finally {
      reset()
    }
  }

  function begin() {
    if (activeDrag) cancel()
    options.onBegin()
    const transaction = options.createTransaction()
    try {
      activeDrag = {
        transaction,
        historyBefore: options.captureHistory(),
      }
    } catch (error) {
      try {
        transaction.cancel()
      } finally {
        reset()
      }
      throw error
    }
  }

  function handleChanges(changes: NodeChange[]): boolean {
    const drag = activeDrag
    if (!drag) return false
    if (changes.length === 0) return false
    if (!changes.every(isNodeDragChange)) {
      flushProjection()
      syncProjection()
      return false
    }

    if (!hasAdvancedLocalEdit && changes.some((change) => change.type === 'position')) {
      options.advanceLocalEditVersion()
      hasAdvancedLocalEdit = true
    }
    changes.forEach((change) => {
      stageCanvasGraphNodeDragIntent(drag.transaction, {
        nodeId: change.id,
        position: change.type === 'position' ? change.position : undefined,
        selected: change.type === 'select' ? change.selected : undefined,
      })
    })
    pendingProjectionChanges = mergeNodeDragChanges(pendingProjectionChanges, changes)
    latestChanges = mergeNodeDragChanges(latestChanges, changes)
    if (pendingFrameId === null) {
      pendingFrameId = scheduleAnimationFrame(() => {
        pendingFrameId = null
        flushProjection()
      })
    }
    return true
  }

  function commit() {
    const drag = activeDrag
    if (!drag) return
    let didCommitTransaction = false
    try {
      const committedChanges = latestChanges
      flushProjection()
      syncProjection()
      drag.transaction.commit()
      didCommitTransaction = true
      options.reconcileMembership(committedChanges)
      if (options.commitHistory(drag.historyBefore)) {
        options.markDirtyAndScheduleAutosave()
      }
    } catch (error) {
      if (!didCommitTransaction) {
        drag.transaction.cancel()
      }
      throw error
    } finally {
      reset()
    }
  }

  return {
    begin,
    handleChanges,
    commit,
    cancel,
    isActive,
  }
}
