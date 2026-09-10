import type { Edge } from '@vue-flow/core'
import type { CanvasConnection, CanvasConnectionEndpoint } from '../../board/domain'
import {
  shouldReplaceCanvasConnection,
  toCanvasGraphEdgeId,
  type CanvasGraphConnectionTargetPolicy,
  type CanvasGraphEdgeId,
  type CanvasGraphStore,
} from '../graph-runtime'
import { flowConnectionToCanvasConnectionEndpoint } from '../utils/flowConnectionAdapter'
import { createFlowConnectionClaimIndex } from '../utils/flowConnectionIdentityResolver'

export type FlowEdgeIdentity = Pick<Edge, 'id' | 'data'> & {
  source?: Edge['source'] | undefined
  target?: Edge['target'] | undefined
  sourceHandle?: Edge['sourceHandle'] | undefined
  targetHandle?: Edge['targetHandle'] | undefined
}

type FlowEdgeRemoveChangeIdentity = Pick<Edge, 'id'> &
  Partial<Pick<Edge, 'data' | 'source' | 'target' | 'sourceHandle' | 'targetHandle'>>

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
  resolveCanvasEdgeId: (edge: FlowEdgeIdentity) => CanvasGraphEdgeId | null
  getFlowEdge: (flowEdgeId: string) => Edge | null
  classifyReplacement: (
    edge: FlowEdgeIdentity,
    nextConnection: CanvasConnectionEndpoint,
    policy: CanvasGraphConnectionTargetPolicy,
  ) => 'replace' | 'retain' | 'unresolved'
}

export function createFlowEdgeProjectionIndex(
  store: CanvasGraphStore,
  existingEdges: readonly Edge[],
): FlowEdgeProjectionIndex {
  const claimIndex = createFlowConnectionClaimIndex({
    candidates: existingEdges.map((edge) => ({
      edge,
      endpoint: flowEdgeIdentityToCanvasConnectionEndpoint(edge),
    })),
    baseConnections: collectStoreConnections(store),
    allocateGeneratedIds: false,
  })

  return {
    resolveCanvasEdgeId(edge) {
      const connectionId = claimIndex.resolveConnectionId({
        edge: edge as Edge,
        endpoint: flowEdgeIdentityToCanvasConnectionEndpoint(edge),
      })
      if (connectionId === null) return null
      return toCanvasGraphEdgeId(connectionId)
    },
    getFlowEdge(flowEdgeId) {
      return claimIndex.getFlowEdge(flowEdgeId)
    },
    classifyReplacement(edge, nextConnection, policy) {
      const endpoint = flowEdgeIdentityToCanvasConnectionEndpoint(edge)
      if (!endpoint) return 'unresolved'
      return shouldReplaceCanvasConnection(endpoint, nextConnection, policy) ? 'replace' : 'retain'
    },
  }
}

export function resolveFlowEdgeProjectionReplacement(options: {
  edges: readonly Edge[]
  projectionIndex: FlowEdgeProjectionIndex
  removedEdgeIds: ReadonlySet<CanvasGraphEdgeId>
  nextConnection: CanvasConnectionEndpoint
  policy: CanvasGraphConnectionTargetPolicy
}): { retainedEdges: Edge[]; removedFlowEdgeIds: string[]; didReplace: boolean } | null {
  const retainedEdges: Edge[] = []
  const removedFlowEdgeIds: string[] = []
  let didReplace = false

  for (const edge of options.edges) {
    const projectionEdgeId = options.projectionIndex.resolveCanvasEdgeId(edge)
    if (projectionEdgeId !== null && options.removedEdgeIds.has(projectionEdgeId)) {
      didReplace = true
      removedFlowEdgeIds.push(edge.id)
      continue
    }

    const replacement = options.projectionIndex.classifyReplacement(
      edge,
      options.nextConnection,
      options.policy,
    )
    if (replacement === 'unresolved') return null
    if (replacement === 'replace') {
      didReplace = true
      removedFlowEdgeIds.push(edge.id)
    } else {
      retainedEdges.push(edge)
    }
  }

  return { retainedEdges, removedFlowEdgeIds, didReplace }
}
