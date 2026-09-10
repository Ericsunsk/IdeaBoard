import type { NodeChange } from '@vue-flow/core'
import {
  CanvasGraphStaleTransactionError,
  stageCanvasGraphNodeDragIntent,
  type CanvasGraphStore,
  type CanvasGraphTransaction,
} from '../../domain/graph'

export type CanvasNodeDragChange = Extract<NodeChange, { type: 'position' }>

export type CanvasNodeDragChangeHandling = {
  consumed: CanvasNodeDragChange[]
  remaining: NodeChange[]
}

export type CanvasNodeDragLifecycle = {
  begin: (nodeIds: readonly string[]) => void
  handleChanges: (changes: NodeChange[]) => CanvasNodeDragChangeHandling | null
  commit: (
    stageFinalization?: (store: CanvasGraphStore, transaction: CanvasGraphTransaction) => void,
  ) => void
  cancel: () => void
  isActive: () => boolean
}

export type CanvasNodeDragLifecycleOptions = {
  createCommand: () => {
    store: CanvasGraphStore
    transaction: CanvasGraphTransaction
  }
  onBegin: () => void
  reconcileMembership: (
    store: CanvasGraphStore,
    transaction: CanvasGraphTransaction,
    changes: CanvasNodeDragChange[],
  ) => (() => void) | void
}

function isNodeDragChange(change: NodeChange): change is CanvasNodeDragChange {
  return change.type === 'position'
}

function mergeNodeDragChanges(
  current: CanvasNodeDragChange[],
  incoming: CanvasNodeDragChange[],
): CanvasNodeDragChange[] {
  const changesByKey = new Map<string, CanvasNodeDragChange>()
  current.forEach((change) => changesByKey.set(`${change.type}:${change.id}`, change))
  incoming.forEach((change) => {
    const key = `${change.type}:${change.id}`
    const previous = changesByKey.get(key)

    // Vue Flow's final position change only carries dragging: false. Keep the
    // last measured position while adopting that terminal drag state.
    changesByKey.set(key, {
      ...previous,
      ...change,
      ...(change.position === undefined && previous?.position !== undefined
        ? { position: previous.position }
        : {}),
      ...(change.from === undefined && previous?.from !== undefined ? { from: previous.from } : {}),
    })
  })
  return [...changesByKey.values()]
}

export function createCanvasNodeDragLifecycle(
  options: CanvasNodeDragLifecycleOptions,
): CanvasNodeDragLifecycle {
  let activeDrag: {
    store: CanvasGraphStore
    transaction: CanvasGraphTransaction
    nodeIds: ReadonlySet<string>
  } | null = null
  let latestChanges: CanvasNodeDragChange[] = []
  let latestPositionChanges: CanvasNodeDragChange[] = []

  function isActive() {
    return activeDrag !== null
  }

  function reset() {
    latestChanges = []
    latestPositionChanges = []
    activeDrag = null
  }

  function isStaleTransactionError(error: unknown): boolean {
    return error instanceof CanvasGraphStaleTransactionError
  }

  function stagePositionChanges(
    transaction: CanvasGraphTransaction,
    changes: readonly CanvasNodeDragChange[],
  ) {
    changes.forEach((change) => {
      if (!change.position) return
      stageCanvasGraphNodeDragIntent(transaction, {
        nodeId: change.id,
        position: change.position,
      })
    })
  }

  function rebaseDragTransaction(
    drag: NonNullable<typeof activeDrag>,
    positionChanges: readonly CanvasNodeDragChange[],
  ) {
    drag.transaction.cancel()
    const command = options.createCommand()
    drag.store = command.store
    drag.transaction = command.transaction
    stagePositionChanges(drag.transaction, positionChanges)
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

  function begin(nodeIds: readonly string[]) {
    if (activeDrag) cancel()
    const targetNodeIds = new Set(nodeIds)
    if (targetNodeIds.size === 0) return

    options.onBegin()
    const command = options.createCommand()
    try {
      activeDrag = { ...command, nodeIds: targetNodeIds }
    } catch (error) {
      try {
        command.transaction.cancel()
      } finally {
        reset()
      }
      throw error
    }
  }

  function handleChanges(changes: NodeChange[]): CanvasNodeDragChangeHandling | null {
    const drag = activeDrag
    if (!drag || changes.length === 0) return null

    const positionChanges = changes.filter(isNodeDragChange)
    if (positionChanges.length === 0) return null

    // A Vue Flow batch can include nodes outside the pointer drag cohort. All
    // position writes must share this transaction, otherwise committing either
    // path invalidates the other's revision. Membership remains limited to the
    // actual drag cohort below.
    const nextPositionChanges = mergeNodeDragChanges(latestPositionChanges, positionChanges)
    try {
      stagePositionChanges(drag.transaction, positionChanges)
    } catch (error) {
      if (!isStaleTransactionError(error)) throw error
      // Another graph commit may legitimately land while Vue Flow is still
      // emitting drag changes. Rebuild from the latest store revision and
      // replay the coalesced positions instead of dropping the user gesture.
      rebaseDragTransaction(drag, nextPositionChanges)
    }
    latestPositionChanges = nextPositionChanges
    const targetChanges = positionChanges.filter((change) => drag.nodeIds.has(change.id))
    latestChanges = mergeNodeDragChanges(latestChanges, targetChanges)
    const consumedChanges = new Set<NodeChange>(positionChanges)
    return {
      consumed: positionChanges,
      remaining: changes.filter((change) => !consumedChanges.has(change)),
    }
  }

  function commit(
    stageFinalization?: (store: CanvasGraphStore, transaction: CanvasGraphTransaction) => void,
  ) {
    const drag = activeDrag
    if (!drag) return
    let didCommitTransaction = false
    let rebased = false
    try {
      while (true) {
        try {
          const committedChanges = latestChanges
          const finalizeMembership = options.reconcileMembership(
            drag.store,
            drag.transaction,
            committedChanges,
          )
          stageFinalization?.(drag.store, drag.transaction)
          drag.transaction.commit()
          didCommitTransaction = true
          finalizeMembership?.()
          break
        } catch (error) {
          if (!isStaleTransactionError(error) || rebased) throw error
          rebased = true
          rebaseDragTransaction(drag, latestPositionChanges)
        }
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
