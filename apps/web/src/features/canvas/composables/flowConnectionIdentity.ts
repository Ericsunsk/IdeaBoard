import type { Edge, EdgeUpdateEvent } from '@vue-flow/core'
import type {
  CanvasConnection,
  CanvasConnectionEndpoint,
} from '../../canvas-board/domain'
import {
  toCanvasGraphEdgeId,
  type CanvasGraphEdgeId,
  type CanvasGraphStore,
} from '../graph-runtime'
import { flowConnectionToCanvasConnectionEndpoint } from '../utils/flowConnectionAdapter'
import {
  createFlowConnectionClaimIndex,
} from '../utils/flowConnectionIdentityResolver'

export type FlowEdgeIdentity = Pick<Edge, 'id' | 'data'> &
  Partial<Pick<Edge, 'source' | 'target' | 'sourceHandle' | 'targetHandle'>>

export type FlowEdgeIdentitySource = FlowEdgeIdentity | Pick<EdgeUpdateEvent, 'edge'>

type FlowEdgeRemoveChangeIdentity = Pick<Edge, 'id'> &
  Partial<Pick<Edge, 'data' | 'source' | 'target' | 'sourceHandle' | 'targetHandle'>>

function resolveFlowEdgeFromIdentitySource(input: FlowEdgeIdentitySource): FlowEdgeIdentity {
  return 'edge' in input ? input.edge : input
}

function flowEdgeIdentityToCanvasConnectionEndpoint(
  edge: FlowEdgeIdentity,
): CanvasConnectionEndpoint | null {
  if (!edge.source || !edge.target) return null
  return flowConnectionToCanvasConnectionEndpoint({
    source: edge.source,
    target: edge.target,
    sourceHandle: edge.sourceHandle ?? null,
    targetHandle: edge.targetHandle ?? null,
  })
}

function collectStoreConnections(store: CanvasGraphStore) {
  const connections: CanvasConnection[] = []
  for (const edgeId of store.getEdgeIds()) {
    const connection = store.getEdge(edgeId)?.connection
    if (connection) connections.push(connection)
  }
  return connections
}

export function createFlowEdgeIdentityFromRemoveChange(
  change: FlowEdgeRemoveChangeIdentity,
): FlowEdgeIdentity {
  return {
    id: change.id,
    data: change.data,
    source: change.source,
    target: change.target,
    sourceHandle: change.sourceHandle,
    targetHandle: change.targetHandle,
  }
}

export type FlowEdgeProjectionIndex = {
  resolveCanvasEdgeId: (input: FlowEdgeIdentitySource) => CanvasGraphEdgeId | null
  resolveFlowEdgeId: (canvasEdgeId: CanvasGraphEdgeId) => string
  getFlowEdge: (flowEdgeId: string) => Edge | null
}

export function createFlowEdgeProjectionIndex(
  store: CanvasGraphStore,
  existingEdges: readonly Edge[],
): FlowEdgeProjectionIndex {
  const connectionIdByCanvasEdgeId = new Map<CanvasGraphEdgeId, number>()
  for (const canvasEdgeId of store.getEdgeIds()) {
    const connectionId = store.getEdge(canvasEdgeId)?.connection.id
    if (connectionId !== undefined) {
      connectionIdByCanvasEdgeId.set(canvasEdgeId, connectionId)
    }
  }
  const claimIndex = createFlowConnectionClaimIndex({
    candidates: existingEdges.map((edge) => ({
      edge,
      endpoint: flowEdgeIdentityToCanvasConnectionEndpoint(edge),
    })),
    baseConnections: collectStoreConnections(store),
    allocateGeneratedIds: false,
  })

  return {
    resolveCanvasEdgeId(input) {
      const edge = resolveFlowEdgeFromIdentitySource(input) as Edge
      const connectionId = claimIndex.resolveConnectionId({
        edge,
        endpoint: flowEdgeIdentityToCanvasConnectionEndpoint(edge),
      })
      if (connectionId === null) return null
      return toCanvasGraphEdgeId(connectionId)
    },
    resolveFlowEdgeId(canvasEdgeId) {
      const connectionId = connectionIdByCanvasEdgeId.get(canvasEdgeId)
      return connectionId === undefined
        ? canvasEdgeId
        : (claimIndex.resolveFlowEdgeId(connectionId) ?? canvasEdgeId)
    },
    getFlowEdge(flowEdgeId) {
      return claimIndex.getFlowEdge(flowEdgeId)
    },
  }
}

export function isRemovedProjectionEdge(
  edge: Edge,
  removedEdgeIds: ReadonlySet<CanvasGraphEdgeId>,
  projectionIndex: FlowEdgeProjectionIndex,
) {
  const projectionEdgeId = projectionIndex.resolveCanvasEdgeId(edge)
  return projectionEdgeId ? removedEdgeIds.has(projectionEdgeId) : false
}
