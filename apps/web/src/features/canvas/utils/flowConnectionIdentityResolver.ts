import type { CanvasConnection, CanvasConnectionEndpoint } from '../../canvas-board/domain'
import { toCanvasConnectionEndpointKey } from '../graph-runtime'

const CANVAS_CONNECTION_EDGE_ID_PATTERN = /^connection-(\d+)$/

export type FlowEdgeIdentityRecord = {
  id: string
  data?: unknown
}

export type FlowConnectionClaimCandidate<TFlowEdge extends FlowEdgeIdentityRecord> = {
  edge: TFlowEdge
  endpoint: CanvasConnectionEndpoint | null
}

export type FlowConnectionClaimIndex<TFlowEdge extends FlowEdgeIdentityRecord> = {
  resolveConnectionId: (candidate: FlowConnectionClaimCandidate<TFlowEdge>) => number | null
  getFlowEdge: (flowEdgeId: string) => TFlowEdge | null
}

export function resolveFlowEdgeCanvasConnectionId(edge: FlowEdgeIdentityRecord): number | null {
  const data = edge.data
  if (data && typeof data === 'object') {
    const restoredId = (data as { connectionSnapshot?: { id?: unknown } }).connectionSnapshot?.id
    if (typeof restoredId === 'number' && Number.isFinite(restoredId)) return restoredId
  }

  const edgeIdMatch = CANVAS_CONNECTION_EDGE_ID_PATTERN.exec(edge.id)
  if (!edgeIdMatch) return null
  const parsedId = Number(edgeIdMatch[1])
  return Number.isInteger(parsedId) && parsedId >= 1 ? parsedId : null
}

export function resolveNextFlowConnectionId(
  baseConnections: readonly CanvasConnection[] = [],
  edges: readonly FlowEdgeIdentityRecord[] = [],
) {
  let maxConnectionId = 0
  for (const connection of baseConnections) {
    if (Number.isFinite(connection.id) && connection.id > maxConnectionId) {
      maxConnectionId = connection.id
    }
  }
  for (const edge of edges) {
    const restoredId = resolveFlowEdgeCanvasConnectionId(edge)
    if (restoredId !== null && restoredId > maxConnectionId) {
      maxConnectionId = restoredId
    }
  }
  return maxConnectionId + 1
}

export function createFlowConnectionClaimIndex<TFlowEdge extends FlowEdgeIdentityRecord>(options: {
  candidates: readonly FlowConnectionClaimCandidate<TFlowEdge>[]
  baseConnections?: readonly CanvasConnection[]
  allocateGeneratedIds: boolean
}): FlowConnectionClaimIndex<TFlowEdge> {
  const baseConnections = options.baseConnections ?? []
  const baseConnectionIds = new Set<number>()
  const endpointByBaseConnectionId = new Map<number, string>()
  const availableBaseConnectionIdsByEndpoint = new Map<string, Set<number>>()
  const firstBaseConnectionIdByEndpoint = new Map<string, number>()

  for (const connection of baseConnections) {
    const endpointKey = toCanvasConnectionEndpointKey(connection)
    baseConnectionIds.add(connection.id)
    endpointByBaseConnectionId.set(connection.id, endpointKey)
    const availableIds = availableBaseConnectionIdsByEndpoint.get(endpointKey)
    if (availableIds) {
      availableIds.add(connection.id)
    } else {
      availableBaseConnectionIdsByEndpoint.set(endpointKey, new Set([connection.id]))
    }
    if (!firstBaseConnectionIdByEndpoint.has(endpointKey)) {
      firstBaseConnectionIdByEndpoint.set(endpointKey, connection.id)
    }
  }

  const explicitOwnerByConnectionId = new Map<number, TFlowEdge>()
  for (const { edge } of options.candidates) {
    const explicitId = resolveFlowEdgeCanvasConnectionId(edge)
    if (explicitId !== null && !explicitOwnerByConnectionId.has(explicitId)) {
      explicitOwnerByConnectionId.set(explicitId, edge)
    }
  }
  const reservedExplicitIds = new Set(explicitOwnerByConnectionId.keys())
  for (const connectionId of reservedExplicitIds) {
    const endpointKey = endpointByBaseConnectionId.get(connectionId)
    if (endpointKey) {
      availableBaseConnectionIdsByEndpoint.get(endpointKey)?.delete(connectionId)
    }
  }

  const connectionIdByFlowEdge = new Map<TFlowEdge, number | null>()
  const connectionIdByFlowEdgeId = new Map<string, number | null>()
  const flowEdgeById = new Map<string, TFlowEdge>()
  const claimedConnectionIds = new Set<number>()
  let nextGeneratedConnectionId = resolveNextFlowConnectionId(
    baseConnections,
    options.candidates.map(({ edge }) => edge),
  )

  function claimConnectionId(edge: TFlowEdge, connectionId: number) {
    claimedConnectionIds.add(connectionId)
    const endpointKey = endpointByBaseConnectionId.get(connectionId)
    if (endpointKey) {
      availableBaseConnectionIdsByEndpoint.get(endpointKey)?.delete(connectionId)
    }
  }

  function allocateGeneratedConnectionId() {
    while (
      claimedConnectionIds.has(nextGeneratedConnectionId) ||
      reservedExplicitIds.has(nextGeneratedConnectionId) ||
      baseConnectionIds.has(nextGeneratedConnectionId)
    ) {
      nextGeneratedConnectionId += 1
    }
    const connectionId = nextGeneratedConnectionId
    nextGeneratedConnectionId += 1
    return connectionId
  }

  for (const candidate of options.candidates) {
    const { edge, endpoint } = candidate
    const explicitId = resolveFlowEdgeCanvasConnectionId(edge)
    let connectionId: number | null = null

    if (
      explicitId !== null &&
      explicitOwnerByConnectionId.get(explicitId) === edge &&
      !claimedConnectionIds.has(explicitId) &&
      (options.allocateGeneratedIds || baseConnectionIds.has(explicitId))
    ) {
      connectionId = explicitId
    }

    if (connectionId === null && endpoint) {
      const availableIds = availableBaseConnectionIdsByEndpoint.get(
        toCanvasConnectionEndpointKey(endpoint),
      )
      connectionId = availableIds?.values().next().value ?? null
    }

    if (connectionId === null && options.allocateGeneratedIds) {
      connectionId = allocateGeneratedConnectionId()
    }

    if (connectionId !== null) claimConnectionId(edge, connectionId)
    connectionIdByFlowEdge.set(edge, connectionId)
    if (!connectionIdByFlowEdgeId.has(edge.id)) {
      connectionIdByFlowEdgeId.set(edge.id, connectionId)
    }
    if (!flowEdgeById.has(edge.id)) flowEdgeById.set(edge.id, edge)
  }

  return {
    resolveConnectionId(candidate) {
      if (connectionIdByFlowEdge.has(candidate.edge)) {
        return connectionIdByFlowEdge.get(candidate.edge) ?? null
      }
      if (connectionIdByFlowEdgeId.has(candidate.edge.id)) {
        return connectionIdByFlowEdgeId.get(candidate.edge.id) ?? null
      }

      const explicitId = resolveFlowEdgeCanvasConnectionId(candidate.edge)
      if (explicitId !== null && baseConnectionIds.has(explicitId)) return explicitId
      return candidate.endpoint
        ? (firstBaseConnectionIdByEndpoint.get(toCanvasConnectionEndpointKey(candidate.endpoint)) ??
            null)
        : null
    },
    getFlowEdge(flowEdgeId) {
      return flowEdgeById.get(flowEdgeId) ?? null
    },
  }
}
