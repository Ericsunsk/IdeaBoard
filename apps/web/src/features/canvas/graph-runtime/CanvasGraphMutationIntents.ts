import type { CanvasConnection, CanvasPoint } from '../../canvas-board/domain'
import {
  toCanvasGraphEdgeId,
  type CanvasGraphEdgeId,
  type CanvasGraphNodeId,
  type CanvasGraphStore,
  type CanvasGraphTransaction,
  type CanvasGraphTransactionReason,
  type CanvasGraphTransactionSummary,
} from './CanvasGraphStore'

export type CanvasGraphConnectionTargetPolicy = {
  allowsMultipleTargetConnections: boolean
}

export type CanvasGraphNodeDragIntent = {
  nodeId: string
  position?: CanvasPoint | null
  selected?: boolean
}

export function isCanvasGraphNodeId(nodeId: string): nodeId is CanvasGraphNodeId {
  return /^(?:image|prompt|fusion)-\d+$/.test(nodeId)
}

export function isSameCanvasConnectionEndpoint(left: CanvasConnection, right: CanvasConnection) {
  return (
    left.fromNodeType === right.fromNodeType &&
    left.fromNodeId === right.fromNodeId &&
    left.fromPort === right.fromPort &&
    left.toNodeType === right.toNodeType &&
    left.toNodeId === right.toNodeId &&
    left.toPort === right.toPort
  )
}

export function targetsSameCanvasInput(left: CanvasConnection, right: CanvasConnection) {
  return (
    left.toNodeType === right.toNodeType &&
    left.toNodeId === right.toNodeId &&
    left.toPort === right.toPort
  )
}

export function shouldReplaceCanvasConnection(
  current: CanvasConnection,
  nextConnection: CanvasConnection,
  policy: CanvasGraphConnectionTargetPolicy,
) {
  if (!policy.allowsMultipleTargetConnections && targetsSameCanvasInput(current, nextConnection)) {
    return true
  }
  return isSameCanvasConnectionEndpoint(current, nextConnection)
}

export function findCanvasGraphEdgeIdByEndpoint(
  store: CanvasGraphStore,
  connection: CanvasConnection,
): CanvasGraphEdgeId | null {
  return (
    store.getEdgeIds().find((edgeId) => {
      const record = store.getEdge(edgeId)
      return record ? isSameCanvasConnectionEndpoint(record.connection, connection) : false
    }) ?? null
  )
}

export function resolveCanvasGraphEdgeId(
  store: CanvasGraphStore,
  input: {
    edgeId?: CanvasGraphEdgeId | null
    connectionId?: number | null
    endpointConnection?: CanvasConnection | null
  },
): CanvasGraphEdgeId | null {
  if (input.edgeId && store.getEdge(input.edgeId)) return input.edgeId
  if (input.connectionId !== undefined && input.connectionId !== null) {
    const edgeId = toCanvasGraphEdgeId(input.connectionId)
    if (store.getEdge(edgeId)) return edgeId
  }
  return input.endpointConnection
    ? findCanvasGraphEdgeIdByEndpoint(store, input.endpointConnection)
    : null
}

export function stageCanvasGraphNodeDragIntent(
  transaction: CanvasGraphTransaction | null | undefined,
  intent: CanvasGraphNodeDragIntent,
): boolean {
  if (!transaction || !isCanvasGraphNodeId(intent.nodeId)) return false
  if (intent.position) {
    transaction.updateNode(intent.nodeId, {
      x: intent.position.x,
      y: intent.position.y,
    })
  }
  if (intent.selected !== undefined) {
    transaction.selectNode(intent.nodeId, intent.selected)
  }
  return true
}

function removeTransactionEdgesMatching(
  store: CanvasGraphStore,
  transaction: CanvasGraphTransaction,
  predicate: (connection: CanvasConnection, edgeId: CanvasGraphEdgeId) => boolean,
) {
  store.getEdgeIds().forEach((edgeId) => {
    const record = store.getEdge(edgeId)
    if (record && predicate(record.connection, edgeId)) transaction.removeEdge(edgeId)
  })
}

export function commitCanvasGraphConnectIntent(
  store: CanvasGraphStore,
  intent: {
    connection: CanvasConnection
    allowsMultipleTargetConnections: boolean
  },
): CanvasGraphTransactionSummary {
  const transaction = store.beginTransaction('connect')
  try {
    removeTransactionEdgesMatching(store, transaction, (current) =>
      shouldReplaceCanvasConnection(current, intent.connection, {
        allowsMultipleTargetConnections: intent.allowsMultipleTargetConnections,
      }),
    )
    transaction.addEdge(intent.connection)
    return transaction.commit()
  } catch (error) {
    transaction.cancel()
    throw error
  }
}

export function commitCanvasGraphEdgeUpdateIntent(
  store: CanvasGraphStore,
  intent: {
    edgeId: CanvasGraphEdgeId
    connection: CanvasConnection
    allowsMultipleTargetConnections: boolean
  },
): CanvasGraphTransactionSummary {
  const transaction = store.beginTransaction('connect')
  try {
    removeTransactionEdgesMatching(store, transaction, (current, edgeId) => {
      if (edgeId === intent.edgeId) return false
      return shouldReplaceCanvasConnection(current, intent.connection, {
        allowsMultipleTargetConnections: intent.allowsMultipleTargetConnections,
      })
    })
    transaction.updateEdge(intent.edgeId, intent.connection)
    return transaction.commit()
  } catch (error) {
    transaction.cancel()
    throw error
  }
}

export function resolveCanvasGraphDeleteReason(
  nodeIds: readonly CanvasGraphNodeId[],
  extraEdgeIds: readonly CanvasGraphEdgeId[],
): CanvasGraphTransactionReason {
  if (nodeIds.length === 0) return extraEdgeIds.length <= 1 ? 'disconnect' : 'batch-delete'
  if (nodeIds.length === 1 && extraEdgeIds.length === 0) return 'delete'
  return 'batch-delete'
}

export function commitCanvasGraphDeleteIntent(
  store: CanvasGraphStore,
  intent: {
    nodeIds?: readonly CanvasGraphNodeId[]
    edgeIds?: readonly CanvasGraphEdgeId[]
    removeConnectedEdges?: boolean
  },
): CanvasGraphTransactionSummary {
  const nodeIds = [...(intent.nodeIds ?? [])]
  const edgeIds = [...(intent.edgeIds ?? [])]
  const transaction = store.beginTransaction(resolveCanvasGraphDeleteReason(nodeIds, edgeIds))
  try {
    transaction.batchDelete({
      nodeIds,
      edgeIds,
      removeConnectedEdges: intent.removeConnectedEdges ?? true,
    })
    return transaction.commit()
  } catch (error) {
    transaction.cancel()
    throw error
  }
}
