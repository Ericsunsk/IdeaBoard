import type { Ref } from 'vue'
import type { Connection, Edge, EdgeUpdateEvent, Node } from '@vue-flow/core'
import type { CanvasVueFlowGraphAdapter } from './useCanvasVueFlowGraphAdapter'
import { createFlowGraphCommandOptions } from './flowGraphAdapterCommandSideEffects'
import { flowConnectionToCanvasConnection } from '../utils/flowConnectionAdapter'
import {
  canvasConnectionToFlowEdge,
  flowToCanvasBoardSnapshot,
  resolveFlowEdgeCanvasConnectionId,
  resolveNextCanvasConnectionId,
} from '../utils/flowSnapshotAdapter'
import {
  commitCanvasGraphConnectIntent,
  commitCanvasGraphEdgeUpdateIntent,
  createCanvasGraphStore,
  resolveCanvasGraphEdgeId,
  shouldReplaceCanvasConnection,
  type CanvasGraphEdgeId,
  type CanvasGraphStore,
  type CanvasGraphTransactionSummary,
} from '../graph-runtime'
import type { CanvasBoardSnapshot, CanvasConnection } from '../../canvas-board/domain'

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

  function createTransactionCommandOptions(summary: CanvasGraphTransactionSummary) {
    return createFlowGraphCommandOptions({
      recordHistory: summary.shouldRecordHistory,
      markDirty: summary.shouldMarkDirty,
    })
  }

  function findTransactionEdgeId(store: CanvasGraphStore, edge: Edge): CanvasGraphEdgeId | null {
    const restoredConnectionId = resolveFlowEdgeCanvasConnectionId(edge)
    const canvasConnection = flowConnectionToCanvasConnection(1, edge)
    return resolveCanvasGraphEdgeId(store, {
      connectionId: restoredConnectionId,
      endpointConnection: canvasConnection,
    })
  }

  function createNextEdgeFromConnection(resolvedConnection: Connection, existingEdges: Edge[]) {
    const nextConnectionId = resolveNextCanvasConnectionId(
      options.boardBaseSnapshot?.value,
      existingEdges as Parameters<typeof resolveNextCanvasConnectionId>[1],
    )
    const nextConnection = flowConnectionToCanvasConnection(nextConnectionId, resolvedConnection)
    if (!nextConnection) return null
    return {
      connection: nextConnection,
      edge: canvasConnectionToFlowEdge(nextConnection) as Edge,
    }
  }

  function filterConnectEdges(
    existingEdges: Edge[],
    nextConnection: CanvasConnection,
    allowsMultipleTargetConnections: boolean,
  ) {
    return existingEdges.filter((edge) => {
      const currentConnection = flowConnectionToCanvasConnection(nextConnection.id, edge)
      return currentConnection
        ? !shouldReplaceCanvasConnection(currentConnection, nextConnection, {
            allowsMultipleTargetConnections,
          })
        : true
    })
  }

  function commitConnectProjection(
    existingEdges: Edge[],
    filteredEdges: Edge[],
    nextEdge: Edge,
    commandOptions = createFlowGraphCommandOptions(),
  ) {
    if (filteredEdges.length === existingEdges.length) {
      options.graphAdapter.addEdges(nextEdge, commandOptions)
    } else {
      options.graphAdapter.replaceEdges([...filteredEdges, nextEdge], commandOptions)
    }
  }

  function onConnectTransaction(resolvedConnection: Connection) {
    const existingEdges = options.edges.value as unknown as Edge[]
    const nextEdge = createNextEdgeFromConnection(resolvedConnection, existingEdges)
    if (!nextEdge) {
      options.onWarning('连线未能写入画布，请刷新后重试')
      return
    }

    const allowsMultipleTargetConnections =
      options.allowsMultipleTargetConnections?.(resolvedConnection) ?? false
    const filteredEdges = filterConnectEdges(
      existingEdges,
      nextEdge.connection,
      allowsMultipleTargetConnections,
    )
    const store = createCurrentGraphStore()
    let summary: CanvasGraphTransactionSummary
    try {
      summary = commitCanvasGraphConnectIntent(store, {
        connection: nextEdge.connection,
        allowsMultipleTargetConnections,
      })
    } catch (error) {
      console.error('[Canvas] Failed to commit connection transaction', error)
      options.onWarning('连线未能写入画布，请刷新后重试')
      return
    }
    if (!summary.didChange) return

    commitConnectProjection(
      existingEdges,
      filteredEdges,
      nextEdge.edge,
      createTransactionCommandOptions(summary),
    )
    options.markFlowExecutionNodeDirty(resolvedConnection.target, { includeSelf: true })
  }

  function onConnect(connection: Connection) {
    const existingEdges = options.edges.value as unknown as Edge[]
    const resolvedConnection = resolveValidatedConnection(connection, existingEdges)
    if (!resolvedConnection) return
    onConnectTransaction(resolvedConnection)
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

  function onEdgeUpdateTransaction(event: EdgeUpdateEvent, resolvedConnection: Connection) {
    const { edge } = event
    const store = createCurrentGraphStore()
    const transactionEdgeId = findTransactionEdgeId(store, edge)
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
    const retainedEdges = (options.edges.value as unknown as Edge[]).filter((item) => {
      if (item.id === edge.id) return true
      const currentConnection = flowConnectionToCanvasConnection(nextConnection.id, item)
      return currentConnection
        ? !shouldReplaceCanvasConnection(currentConnection, nextConnection, {
            allowsMultipleTargetConnections,
          })
        : true
    })

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

    const nextEdge = canvasConnectionToFlowEdge(nextConnection) as Edge
    const commandOptions = createTransactionCommandOptions(summary)
    if (retainedEdges.length === options.edges.value.length) {
      options.graphAdapter.updateEdge(edge.id, createEdgeUpdatePatch(nextEdge), commandOptions)
    } else {
      options.graphAdapter.replaceEdges(
        retainedEdges.map((item) =>
          item.id === edge.id ? ({ ...item, ...nextEdge } as Edge) : item,
        ),
        commandOptions,
      )
    }
    options.markFlowExecutionNodeDirty(resolvedConnection.target, { includeSelf: true })
  }

  function onEdgeUpdate(event: EdgeUpdateEvent) {
    const { edge, connection } = event
    const existingEdges = (options.edges.value as unknown as Edge[]).filter(
      (item) => item.id !== edge.id,
    )
    const resolvedConnection = resolveValidatedConnection(connection, existingEdges)
    if (!resolvedConnection) return
    onEdgeUpdateTransaction(event, resolvedConnection)
  }

  return {
    onConnect,
    onEdgeUpdate,
  }
}
