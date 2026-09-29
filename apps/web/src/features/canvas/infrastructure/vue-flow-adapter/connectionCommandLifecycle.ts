import type { Connection, Edge, Node } from '@vue-flow/core'
import { i18n } from '../../../../shared/i18n'
import type { CanvasVueFlowGraphAdapter } from './useCanvasVueFlowGraphAdapter'
import { createFlowGraphCommandOptions } from './flowGraphAdapterCommandSideEffects'
import { flowConnectionToCanvasConnection } from './flowConnectionAdapter.ts'
import { resolveNextCanvasConnectionId } from './flowSnapshotAdapter.ts'
import {
  commitCanvasGraphConnectIntent,
  shouldReplaceCanvasConnection,
  type CanvasGraphEdgeId,
  type CanvasGraphStore,
  type CanvasGraphTransactionSummary,
} from '../../domain/graph'
import type { BoardSnapshot } from '../../../board/ports/canvas'
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

type CanvasConnectionValidationFailure =
  { kind: 'semantics' } | { kind: 'endpoints' } | { kind: 'mutation'; message: string }

type CanvasConnectionValidationResult =
  { connection: Connection } | { failure: CanvasConnectionValidationFailure }

export function createCanvasConnectionCommandLifecycle(
  options: CanvasConnectionCommandLifecycleOptions,
) {
  // Preview and commit share this single normalization + validation run
  // (including the target input-limit policy), so the highlight shown during
  // drag always agrees with the drop outcome.
  function validateConnection(
    connection: Connection,
    existingEdges: readonly Edge[],
  ): CanvasConnectionValidationResult {
    const resolvedConnection = options.normalizeConnection
      ? options.normalizeConnection(connection, existingEdges)
      : connection
    if (!resolvedConnection || !options.isValidConnection(resolvedConnection)) {
      return { failure: { kind: 'semantics' } }
    }
    if (!resolvedConnection.source || !resolvedConnection.target) {
      return { failure: { kind: 'endpoints' } }
    }
    const validationMessage =
      options.validateConnectionMutation?.(resolvedConnection, existingEdges) ?? null
    if (validationMessage) return { failure: { kind: 'mutation', message: validationMessage } }
    return { connection: resolvedConnection }
  }

  function resolveValidatedConnection(
    connection: Connection,
    existingEdges: readonly Edge[],
  ): Connection | null {
    const result = validateConnection(connection, existingEdges)
    if ('connection' in result) return result.connection
    if (result.failure.kind === 'semantics') {
      options.onWarning(i18n.global.t('canvas.edges.connectionSemanticsWarning'))
    } else if (result.failure.kind === 'mutation') {
      options.onWarning(result.failure.message)
    }
    return null
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

  function canConnectPreview(connection: Connection): boolean {
    return 'connection' in validateConnection(connection, options.readEdges())
  }

  return createCanvasVueFlowConnectionAdapter({
    canConnect: canConnectPreview,
    connect,
  })
}
