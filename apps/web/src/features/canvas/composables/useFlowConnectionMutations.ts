import type { Ref } from 'vue'
import type { Connection, Edge, EdgeUpdateEvent, Node } from '@vue-flow/core'
import type { CanvasVueFlowGraphAdapter } from './useCanvasVueFlowGraphAdapter'
import {
  applyCanvasGraphCommitPolicy,
  type CanvasGraphCommitPolicy,
} from './canvasGraphCommitPolicy'
import { flowConnectionToCanvasConnection } from '../utils/flowConnectionAdapter'
import {
  canvasConnectionToFlowEdge,
  flowToCanvasBoardSnapshot,
  resolveNextCanvasConnectionId,
} from '../utils/flowSnapshotAdapter'
import {
  commitCanvasGraphConnectIntent,
  commitCanvasGraphEdgeUpdateIntent,
  createCanvasGraphStore,
  type CanvasGraphEdgeId,
  type CanvasGraphStore,
  type CanvasGraphTransactionSummary,
} from '../graph-runtime'
import type { CanvasBoardSnapshot, CanvasConnection } from '../../canvas-board/domain'
import {
  createFlowEdgeProjectionIndex,
  resolveFlowEdgeProjectionReplacement,
  type FlowEdgeProjectionIndex,
} from './flowConnectionIdentity'
import {
  createVueFlowConnectionBindings,
  createVueFlowConnectionFromIntent,
  type CanvasFlowPaneConnectIntent,
} from '../flow-pane/vueFlowConnectionBindings'

type UseFlowConnectionMutationsOptions = {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- vue-flow node data is an open Record; narrowing breaks vue-flow's internal type expectations
  nodes: Ref<Array<Record<string, any>>>
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- vue-flow edge data is an open Record; narrowing breaks vue-flow's internal type expectations
  edges: Ref<Array<Record<string, any>>>
  boardBaseSnapshot?: Ref<CanvasBoardSnapshot>
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

export function useFlowConnectionMutations(options: UseFlowConnectionMutationsOptions) {
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
          nodes: options.nodes.value as unknown as Node[],
          edges: options.edges.value as Parameters<typeof flowToCanvasBoardSnapshot>[0]['edges'],
        },
        options.boardBaseSnapshot?.value,
      ),
    )
  }

  function createNextCanvasConnection(resolvedConnection: Connection, existingEdges: Edge[]) {
    const nextConnectionId = resolveNextCanvasConnectionId(
      options.boardBaseSnapshot?.value,
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

  function onConnectTransaction(resolvedConnection: Connection) {
    const existingEdges = options.edges.value as unknown as Edge[]
    const nextConnection = createNextCanvasConnection(resolvedConnection, existingEdges)
    if (!nextConnection) {
      options.onWarning('连线未能写入画布，请刷新后重试')
      return
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
      return
    }
    if (!summary.didChange) return

    const didCommitProjection = commitConnectProjection(
      existingEdges,
      projectionIndex,
      nextConnection,
      allowsMultipleTargetConnections,
      { summary },
    )
    if (!didCommitProjection) return
    options.markFlowExecutionNodeDirty(resolvedConnection.target, { includeSelf: true })
  }

  function connect(connection: Connection) {
    const existingEdges = options.edges.value as unknown as Edge[]
    const resolvedConnection = resolveValidatedConnection(connection, existingEdges)
    if (!resolvedConnection) return
    onConnectTransaction(resolvedConnection)
  }

  function connectNodes(intent: CanvasFlowPaneConnectIntent) {
    connect(createVueFlowConnectionFromIntent(intent))
  }

  function createEdgeUpdatePatch(nextEdge: Edge): Partial<Edge> {
    return {
      id: nextEdge.id,
      source: nextEdge.source,
      target: nextEdge.target,
      sourceHandle: nextEdge.sourceHandle ?? null,
      targetHandle: nextEdge.targetHandle ?? null,
      animated: nextEdge.animated,
      data: nextEdge.data,
    }
  }

  function resolveUpdatedEdge(
    summary: CanvasGraphTransactionSummary,
    nextConnection: CanvasConnection,
  ) {
    const updatedConnection = summary.projectionPatch.edges.updated[0]?.connection ?? nextConnection
    return canvasConnectionToFlowEdge(updatedConnection) as Edge
  }

  function commitEdgeUpdateProjection(
    existingEdges: Edge[],
    projectionIndex: FlowEdgeProjectionIndex,
    updatedEdgeId: CanvasGraphEdgeId,
    updatedFlowEdgeId: string,
    nextConnection: CanvasConnection,
    allowsMultipleTargetConnections: boolean,
    commitPolicy: CanvasGraphCommitPolicy,
  ) {
    const { summary } = commitPolicy
    const nextEdge = resolveUpdatedEdge(summary, nextConnection)
    const removedEdgeIds = new Set(summary.projectionPatch.edges.removedIds)
    const replacement = resolveFlowEdgeProjectionReplacement({
      edges: existingEdges,
      projectionIndex,
      removedEdgeIds,
      nextConnection,
      policy: { allowsMultipleTargetConnections },
      preserveFlowEdgeId: updatedFlowEdgeId,
    })
    if (!replacement) {
      options.onWarning('连线更新未能写入画布，请刷新后重试')
      return false
    }

    applyCanvasGraphCommitPolicy(commitPolicy, (commandOptions) => {
      if (!replacement.didReplace) {
        options.graphAdapter.updateEdge(
          projectionIndex.resolveFlowEdgeId(updatedEdgeId),
          createEdgeUpdatePatch(nextEdge),
          commandOptions,
        )
        return
      }

      options.graphAdapter.replaceEdges(
        replacement.retainedEdges.map((edge) =>
          projectionIndex.resolveCanvasEdgeId(edge) === updatedEdgeId
            ? ({ ...edge, ...nextEdge } as Edge)
            : edge,
        ),
        commandOptions,
      )
    })
    return true
  }

  function onEdgeUpdateTransaction(event: EdgeUpdateEvent, resolvedConnection: Connection) {
    const store = createCurrentGraphStore()
    const existingEdges = options.edges.value as unknown as Edge[]
    const projectionIndex = createFlowEdgeProjectionIndex(store, existingEdges)
    const transactionEdgeId = projectionIndex.resolveCanvasEdgeId(event)
    if (!transactionEdgeId) {
      options.onWarning('连线更新未能写入画布，请刷新后重试')
      return
    }

    const currentConnection = store.getEdge(transactionEdgeId)?.connection
    if (!currentConnection) {
      options.onWarning('连线更新未能写入画布，请刷新后重试')
      return
    }
    const nextConnection = flowConnectionToCanvasConnection(
      currentConnection.id,
      resolvedConnection,
    )
    if (!nextConnection) {
      options.onWarning('连线更新未能写入画布，请刷新后重试')
      return
    }

    const allowsMultipleTargetConnections =
      options.allowsMultipleTargetConnections?.(resolvedConnection) ?? false

    let summary: CanvasGraphTransactionSummary
    try {
      summary = commitCanvasGraphEdgeUpdateIntent(store, {
        edgeId: transactionEdgeId,
        connection: nextConnection,
        allowsMultipleTargetConnections,
      })
    } catch (error) {
      console.error('[Canvas] Failed to commit edge update transaction', error)
      options.onWarning('连线更新未能写入画布，请刷新后重试')
      return
    }
    if (!summary.didChange) return

    const didCommitProjection = commitEdgeUpdateProjection(
      existingEdges,
      projectionIndex,
      transactionEdgeId,
      event.edge.id,
      nextConnection,
      allowsMultipleTargetConnections,
      { summary },
    )
    if (!didCommitProjection) return
    options.markFlowExecutionNodeDirty(resolvedConnection.target, { includeSelf: true })
  }

  function updateEdge(event: EdgeUpdateEvent) {
    const { edge, connection } = event
    const existingEdges = (options.edges.value as unknown as Edge[]).filter(
      (item) => item.id !== edge.id,
    )
    const resolvedConnection = resolveValidatedConnection(connection, existingEdges)
    if (!resolvedConnection) return
    onEdgeUpdateTransaction(event, resolvedConnection)
  }

  const vueFlowConnectionBindings = createVueFlowConnectionBindings({
    canConnect: options.isValidConnection,
    connect,
    updateEdge,
  })

  return {
    connect,
    connectNodes,
    vueFlowConnectionBindings,
  }
}
