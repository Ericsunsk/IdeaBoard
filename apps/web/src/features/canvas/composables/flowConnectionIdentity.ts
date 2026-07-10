import type { Edge, EdgeUpdateEvent } from '@vue-flow/core'
import type { CanvasConnection } from '../../canvas-board/domain'
import {
  resolveCanvasGraphEdgeId,
  type CanvasGraphEdgeId,
  type CanvasGraphStore,
} from '../graph-runtime'
import { flowConnectionToCanvasConnection } from '../utils/flowConnectionAdapter'
import {
  canvasConnectionToFlowEdge,
  resolveFlowEdgeCanvasConnectionId,
} from '../utils/flowSnapshotAdapter'

export type FlowEdgeIdentity = Pick<Edge, 'id' | 'data'> &
  Partial<Pick<Edge, 'source' | 'target' | 'sourceHandle' | 'targetHandle'>>

type FlowEdgeIdentitySource = FlowEdgeIdentity | Pick<EdgeUpdateEvent, 'edge'>

type FlowEdgeRemoveChangeIdentity = Pick<Edge, 'id'> &
  Partial<Pick<Edge, 'data' | 'source' | 'target' | 'sourceHandle' | 'targetHandle'>>

function resolveFlowEdgeFromIdentitySource(input: FlowEdgeIdentitySource): FlowEdgeIdentity {
  return 'edge' in input ? input.edge : input
}

function flowEdgeIdentityToCanvasConnection(
  connectionId: number,
  edge: FlowEdgeIdentity,
): CanvasConnection | null {
  if (!edge.source || !edge.target) return null
  return flowConnectionToCanvasConnection(connectionId, {
    source: edge.source,
    target: edge.target,
    sourceHandle: edge.sourceHandle ?? null,
    targetHandle: edge.targetHandle ?? null,
  })
}

function resolveBaseConnectionByEndpoint(
  baseStore: CanvasGraphStore | null,
  edge: FlowEdgeIdentity,
): CanvasConnection | null {
  if (!baseStore || resolveFlowEdgeCanvasConnectionId(edge) !== null) return null
  const endpointConnection = flowEdgeIdentityToCanvasConnection(1, edge)
  const edgeId = endpointConnection
    ? resolveCanvasGraphEdgeId(baseStore, { endpointConnection })
    : null
  return edgeId ? (baseStore.getEdge(edgeId)?.connection ?? null) : null
}

export function canonicalizeFlowEdgeIdentity(edge: Edge, baseStore: CanvasGraphStore | null): Edge {
  const baseConnection = resolveBaseConnectionByEndpoint(baseStore, edge)
  if (!baseConnection) return edge
  const canonicalEdge = canvasConnectionToFlowEdge(baseConnection)
  return {
    ...edge,
    id: canonicalEdge.id,
    data: canonicalEdge.data,
  } as Edge
}

export function resolveFlowEdgeCanvasGraphEdgeId(
  store: CanvasGraphStore,
  input: FlowEdgeIdentitySource,
): CanvasGraphEdgeId | null {
  const edge = resolveFlowEdgeFromIdentitySource(input)
  const restoredConnectionId = resolveFlowEdgeCanvasConnectionId(edge)
  const endpointConnection = flowEdgeIdentityToCanvasConnection(restoredConnectionId ?? 1, edge)
  return resolveCanvasGraphEdgeId(store, {
    connectionId: restoredConnectionId,
    endpointConnection,
  })
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

export function createProjectionEdgeIdLookup(
  store: CanvasGraphStore,
  existingEdges: readonly Edge[],
) {
  const edgeIds = new Map<Edge, CanvasGraphEdgeId | null>()
  existingEdges.forEach((edge) => {
    edgeIds.set(edge, resolveFlowEdgeCanvasGraphEdgeId(store, edge))
  })
  return edgeIds
}

export function isRemovedProjectionEdge(
  edge: Edge,
  removedEdgeIds: ReadonlySet<CanvasGraphEdgeId>,
  projectionEdgeIds: ReadonlyMap<Edge, CanvasGraphEdgeId | null>,
) {
  const projectionEdgeId = projectionEdgeIds.get(edge)
  return projectionEdgeId ? removedEdgeIds.has(projectionEdgeId) : false
}

export function resolveProjectionFlowEdgeId(
  projectionEdgeIds: ReadonlyMap<Edge, CanvasGraphEdgeId | null>,
  canvasEdgeId: CanvasGraphEdgeId,
) {
  for (const [edge, edgeId] of projectionEdgeIds) {
    if (edgeId === canvasEdgeId) return edge.id
  }
  return canvasEdgeId
}
