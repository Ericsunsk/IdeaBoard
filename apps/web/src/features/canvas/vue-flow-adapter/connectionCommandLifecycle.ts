import type { Connection, Edge, Node } from '@vue-flow/core'
import { i18n } from '../../../shared/i18n'
import type { CanvasVueFlowGraphAdapter } from './useCanvasVueFlowGraphAdapter'
import { createFlowGraphCommandOptions } from './flowGraphAdapterCommandSideEffects'
import { flowConnectionToCanvasConnection } from '../utils/flowConnectionAdapter'
import { resolveNextCanvasConnectionId } from '../utils/flowSnapshotAdapter'
import {
  commitCanvasGraphConnectIntent,
  shouldReplaceCanvasConnection,
  type CanvasGraphEdgeId,
  type CanvasGraphStore,
  type CanvasGraphTransactionSummary,
} from '../graph-runtime'
import type { BoardSnapshot } from '../../board/domain'
import {
  createFlowEdgeProjectionIndex,
  resolveFlowEdgeProjectionReplacement,
} from './flowConnectionIdentity'
import { createCanvasVueFlowConnectionAdapter } from './vueFlowConnectionBindings'

export type CanvasConnectionCommandLifecycleOptions = {
  readNodes: () => Node[]
  readEdges: () => Edge[]
  readBoardBaseSnapshot: () => BoardSnapshot | undefined
  readGraphStore: () => CanvasGraphStore
  isValidConnection: (connection: Connection) => boolean
  normalizeConnection?: (
    connection: Connection,
    existingEdges: readonly Edge[],
  ) => Connection | null
  allowsMultipleTargetConnections?: (connection: Connection) => boolean
  validateConnectionMutation?: (
    connection: Connection,
    existingEdges: readonly Edge[],
  ) => string | null
  graphAdapter: CanvasVueFlowGraphAdapter
  onWarning: (message: string) => void
  markFlowExecutionNodeDirty: (nodeId: string, options?: { includeSelf?: boolean }) => void
}

export function createCanvasConnectionCommandLifecycle(
  options: CanvasConnectionCommandLifecycleOptions,
) {
  function resolveValidatedConnection(
    connection: Connection,
    existingEdges: readonly Edge[],
  ): Connection | null {
    const resolvedConnection = options.normalizeConnection
      ? options.normalizeConnection(connection, existingEdges)
      : connection
    if (!resolvedConnection || !options.isValidConnection(resolvedConnection)) {
      options.onWarning(i18n.global.t('canvas.edges.connectionSemanticsWarning'))
      return null
    }
    if (!resolvedConnection.source || !resolvedConnection.target) return null
    const validationMessage =
      options.validateConnectionMutation?.(resolvedConnection, existingEdges) ?? null
    if (validationMessage) {
      options.onWarning(validationMessage)
      return null
    }
    return resolvedConnection
  }

  function createNextCanvasConnection(resolvedConnection: Connection, existingEdges: Edge[]) {
    const nextConnectionId = resolveNextCanvasConnectionId(
      options.readBoardBaseSnapshot(),
      existingEdges as Parameters<typeof resolveNextCanvasConnectionId>[1],
    )
    return flowConnectionToCanvasConnection(nextConnectionId, resolvedConnection)
  }

  function commitConnectProjection(
    replacement: NonNullable<ReturnType<typeof resolveFlowEdgeProjectionReplacement>>,
    store: CanvasGraphStore,
  ) {
    // The canonical commit subscriber projects the added/removed graph edges.
    // Only Flow IDs that never belonged to the canonical store need an
    // explicit cleanup (for example a stale Vue Flow-generated edge ID).
    const canonicalEdgeIds = new Set(store.getEdgeIds())
    const staleFlowEdgeIds = replacement.removedFlowEdgeIds.filter(
      (edgeId) => !canonicalEdgeIds.has(edgeId as CanvasGraphEdgeId),
    )
    if (staleFlowEdgeIds.length > 0) {
      options.graphAdapter.removeEdges(
        staleFlowEdgeIds,
        createFlowGraphCommandOptions({ recordHistory: false, markDirty: false }),
      )
    }
    return true
  }

  function commitConnection(resolvedConnection: Connection): boolean {
    const existingEdges = options.readEdges()
    const nextConnection = createNextCanvasConnection(resolvedConnection, existingEdges)
    if (!nextConnection) {
      options.onWarning(i18n.global.t('canvas.edges.connectionWriteFailedWarning'))
      return false
    }

    const allowsMultipleTargetConnections =
      options.allowsMultipleTargetConnections?.(resolvedConnection) ?? false
    const store = options.readGraphStore()
    const projectionIndex = createFlowEdgeProjectionIndex(store, existingEdges)
    const removedEdgeIds = new Set<CanvasGraphEdgeId>()
    for (const edgeId of store.getEdgeIds()) {
      const existing = store.getEdge(edgeId)?.connection
      if (
        existing &&
        shouldReplaceCanvasConnection(existing, nextConnection, { allowsMultipleTargetConnections })
      ) {
        removedEdgeIds.add(edgeId)
      }
    }
    const replacement = resolveFlowEdgeProjectionReplacement({
      edges: existingEdges,
      projectionIndex,
      removedEdgeIds,
      nextConnection,
      policy: { allowsMultipleTargetConnections },
    })
    if (!replacement) {
      options.onWarning(i18n.global.t('canvas.edges.connectionWriteFailedWarning'))
      return false
    }
    let summary: CanvasGraphTransactionSummary
    try {
      summary = commitCanvasGraphConnectIntent(store, {
        connection: nextConnection,
        allowsMultipleTargetConnections,
      })
    } catch (error) {
      console.error('[Canvas] Failed to commit connection transaction', error)
      options.onWarning(i18n.global.t('canvas.edges.connectionWriteFailedWarning'))
      return false
    }
    if (!summary.didChange) return false

    const didCommitProjection = commitConnectProjection(replacement, store)
    if (!didCommitProjection) return false
    options.markFlowExecutionNodeDirty(resolvedConnection.target, { includeSelf: true })
    return true
  }

  function connect(connection: Connection): boolean {
    const existingEdges = options.readEdges()
    const resolvedConnection = resolveValidatedConnection(connection, existingEdges)
    if (!resolvedConnection) return false
    return commitConnection(resolvedConnection)
  }

  return createCanvasVueFlowConnectionAdapter({
    canConnect: options.isValidConnection,
    connect,
  })
}
