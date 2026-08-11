import type { Connection, Edge, Node } from '@vue-flow/core'
import type { CanvasVueFlowGraphAdapter } from '../composables/useCanvasVueFlowGraphAdapter'
import {
  applyCanvasGraphCommitPolicy,
  type CanvasGraphCommitPolicy,
} from '../composables/canvasGraphCommitPolicy'
import { flowConnectionToCanvasConnection } from '../utils/flowConnectionAdapter'
import {
  canvasConnectionToFlowEdge,
  flowToCanvasBoardSnapshot,
  resolveNextCanvasConnectionId,
} from '../utils/flowSnapshotAdapter'
import {
  commitCanvasGraphConnectIntent,
  createCanvasGraphStore,
  type CanvasGraphStore,
  type CanvasGraphTransactionSummary,
} from '../graph-runtime'
import type { CanvasBoardSnapshot, CanvasConnection } from '../../canvas-board/domain'
import {
  createFlowEdgeProjectionIndex,
  resolveFlowEdgeProjectionReplacement,
  type FlowEdgeProjectionIndex,
} from './flowConnectionIdentity'
import { createCanvasFlowPaneConnectionAdapter } from './vueFlowConnectionBindings'

export type CanvasConnectionCommandLifecycleOptions = {
  readNodes: () => Node[]
  readEdges: () => Edge[]
  readBoardBaseSnapshot: () => CanvasBoardSnapshot | undefined
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
      options.onWarning('该连线不符合当前连接语义')
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

  function createCurrentGraphStore(): CanvasGraphStore {
    return createCanvasGraphStore(
      flowToCanvasBoardSnapshot(
        {
          nodes: options.readNodes(),
          edges: options.readEdges() as Parameters<typeof flowToCanvasBoardSnapshot>[0]['edges'],
        },
        options.readBoardBaseSnapshot(),
      ),
    )
  }

  function createNextCanvasConnection(resolvedConnection: Connection, existingEdges: Edge[]) {
    const nextConnectionId = resolveNextCanvasConnectionId(
      options.readBoardBaseSnapshot(),
      existingEdges as Parameters<typeof resolveNextCanvasConnectionId>[1],
    )
    return flowConnectionToCanvasConnection(nextConnectionId, resolvedConnection)
  }

  function resolveAddedConnectEdge(
    summary: CanvasGraphTransactionSummary,
    nextConnection: CanvasConnection,
  ) {
    const addedConnection = summary.projectionPatch.edges.added[0]?.connection ?? nextConnection
    return canvasConnectionToFlowEdge(addedConnection) as Edge
  }

  function commitConnectProjection(
    existingEdges: Edge[],
    projectionIndex: FlowEdgeProjectionIndex,
    nextConnection: CanvasConnection,
    allowsMultipleTargetConnections: boolean,
    commitPolicy: CanvasGraphCommitPolicy,
  ) {
    const { summary } = commitPolicy
    const nextEdge = resolveAddedConnectEdge(summary, nextConnection)
    const removedEdgeIds = new Set(summary.projectionPatch.edges.removedIds)
    const replacement = resolveFlowEdgeProjectionReplacement({
      edges: existingEdges,
      projectionIndex,
      removedEdgeIds,
      nextConnection,
      policy: { allowsMultipleTargetConnections },
    })
    if (!replacement) {
      options.onWarning('连线未能写入画布，请刷新后重试')
      return false
    }

    applyCanvasGraphCommitPolicy(commitPolicy, (commandOptions) => {
      if (!replacement.didReplace) {
        options.graphAdapter.addEdges(nextEdge, commandOptions)
        return
      }

      options.graphAdapter.replaceEdges([...replacement.retainedEdges, nextEdge], commandOptions)
    })
    return true
  }

  function commitConnection(resolvedConnection: Connection): boolean {
    const existingEdges = options.readEdges()
    const nextConnection = createNextCanvasConnection(resolvedConnection, existingEdges)
    if (!nextConnection) {
      options.onWarning('连线未能写入画布，请刷新后重试')
      return false
    }

    const allowsMultipleTargetConnections =
      options.allowsMultipleTargetConnections?.(resolvedConnection) ?? false
    const store = createCurrentGraphStore()
    const projectionIndex = createFlowEdgeProjectionIndex(store, existingEdges)
    let summary: CanvasGraphTransactionSummary
    try {
      summary = commitCanvasGraphConnectIntent(store, {
        connection: nextConnection,
        allowsMultipleTargetConnections,
      })
    } catch (error) {
      console.error('[Canvas] Failed to commit connection transaction', error)
      options.onWarning('连线未能写入画布，请刷新后重试')
      return false
    }
    if (!summary.didChange) return false

    const didCommitProjection = commitConnectProjection(
      existingEdges,
      projectionIndex,
      nextConnection,
      allowsMultipleTargetConnections,
      { summary },
    )
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

  return createCanvasFlowPaneConnectionAdapter({
    canConnect: options.isValidConnection,
    connect,
  })
}
