import type { Connection } from '@vue-flow/core'
import {
  getCanvasNodePortInputLimit,
  type BoardSnapshot,
  type CanvasConnection,
  type CanvasConnectionEndpoint,
  type CanvasConnectionNodeKind,
  type CanvasConnectionPort,
} from '../../../../board/ports/canvas'
import type { FlowNodeData } from '../../../core/nodes/flowTypes.ts'
import { normalizeFlowConnection } from '../connections/flowConnectionAdapter'
import {
  createFlowConnectionClaimIndex,
  resolveNextFlowConnectionId,
} from '../connections/flowConnectionIdentityResolver'

export type FlowSnapshotConnectionNodeRef = {
  nodeType: CanvasConnectionNodeKind
  id: number
}

export type FlowSnapshotConnectionEdge = Connection & {
  id: string
  source: string
  target: string
  data?: unknown
}

type NormalizedFlowEdgeForResolution = Pick<FlowSnapshotConnectionEdge, 'id' | 'target'> & {
  targetHandle?: string | null
}

export type FlowSnapshotConnectionMaterializerOptions = {
  edges: readonly FlowSnapshotConnectionEdge[]
  baseConnections?: readonly CanvasConnection[] | undefined
  persistedFlowNodeRefs: ReadonlyMap<string, FlowSnapshotConnectionNodeRef>
  lookupData: (nodeId: string) => FlowNodeData | undefined
}

function parsePortFromHandle(
  handle: string | null | undefined,
  direction: 'in' | 'out',
): CanvasConnectionPort | null {
  if (!handle) return null
  const [handleDirection, port] = handle.split(':')
  if (handleDirection !== direction || !port) return null
  return port as CanvasConnectionPort
}

export function resolveNextCanvasConnectionId(
  base: BoardSnapshot | undefined,
  edges: readonly FlowSnapshotConnectionEdge[],
) {
  return resolveNextFlowConnectionId(base?.connections, edges)
}

function getTargetPortKey(
  nodeType: CanvasConnectionNodeKind,
  nodeId: number,
  port: CanvasConnectionPort,
) {
  return `${nodeType}:${nodeId}:${port}`
}

function getInputLimitKey(nodeType: CanvasConnectionNodeKind, port: CanvasConnectionPort) {
  return `${nodeType}:${port}`
}

function createInputLimitResolver() {
  const inputLimitCache = new Map<string, number>()
  return (nodeType: CanvasConnectionNodeKind, port: CanvasConnectionPort) => {
    const key = getInputLimitKey(nodeType, port)
    const cachedLimit = inputLimitCache.get(key)
    if (cachedLimit !== undefined) return cachedLimit > 1
    const limit = getCanvasNodePortInputLimit(nodeType, port)
    inputLimitCache.set(key, limit)
    return limit > 1
  }
}

function rememberNormalizedTargetEdge(
  normalizedEdgesByTarget: Map<string, NormalizedFlowEdgeForResolution[]>,
  edge: FlowSnapshotConnectionEdge,
  toPort: CanvasConnectionPort,
) {
  const targetEdges = normalizedEdgesByTarget.get(edge.target)
  const edgeForResolution: NormalizedFlowEdgeForResolution = {
    id: edge.id,
    target: edge.target,
    targetHandle: `in:${toPort}`,
  }
  if (targetEdges) {
    targetEdges.push(edgeForResolution)
  } else {
    normalizedEdgesByTarget.set(edge.target, [edgeForResolution])
  }
}

type FlowSnapshotClaimCandidate = {
  edge: FlowSnapshotConnectionEdge
  endpoint: CanvasConnectionEndpoint
  toRef: FlowSnapshotConnectionNodeRef
  toPort: CanvasConnectionPort
}

function collectClaimCandidates(
  edges: readonly FlowSnapshotConnectionEdge[],
  persistedFlowNodeRefs: ReadonlyMap<string, FlowSnapshotConnectionNodeRef>,
  lookupData: (nodeId: string) => FlowNodeData | undefined,
  normalizedEdgesByTarget: Map<string, NormalizedFlowEdgeForResolution[]>,
): FlowSnapshotClaimCandidate[] {
  const emptyNormalizedEdges: readonly NormalizedFlowEdgeForResolution[] = []
  const candidates: FlowSnapshotClaimCandidate[] = []

  for (const edge of edges) {
    const fromRef = persistedFlowNodeRefs.get(edge.source)
    const toRef = persistedFlowNodeRefs.get(edge.target)
    if (!fromRef || !toRef) continue

    const normalizedEdge = normalizeFlowConnection(
      edge,
      lookupData,
      normalizedEdgesByTarget.get(edge.target) ?? emptyNormalizedEdges,
    )
    const sourceHandle = normalizedEdge?.sourceHandle ?? edge.sourceHandle
    const targetHandle = normalizedEdge?.targetHandle ?? edge.targetHandle
    const fromPort = parsePortFromHandle(sourceHandle ?? null, 'out')
    const toPort = parsePortFromHandle(targetHandle ?? null, 'in')
    if (!fromPort || !toPort) continue

    rememberNormalizedTargetEdge(normalizedEdgesByTarget, edge, toPort)

    candidates.push({
      edge,
      endpoint: {
        fromNodeType: fromRef.nodeType,
        fromNodeId: fromRef.id,
        fromPort,
        toNodeType: toRef.nodeType,
        toNodeId: toRef.id,
        toPort,
      },
      toRef,
      toPort,
    })
  }

  return candidates
}

function resolveClaimedConnections(
  candidates: readonly FlowSnapshotClaimCandidate[],
  claimIndex: {
    resolveConnectionId: (candidate: FlowSnapshotClaimCandidate) => number | null
  },
  allowsMultipleInputs: (nodeType: CanvasConnectionNodeKind, port: CanvasConnectionPort) => boolean,
): CanvasConnection[] {
  const singleInputConnectionIndex = new Map<string, number>()
  const connections: Array<CanvasConnection | null> = []

  for (const candidate of candidates) {
    const { endpoint, toRef, toPort } = candidate
    const connectionId = claimIndex.resolveConnectionId(candidate)
    if (connectionId === null) continue
    if (!allowsMultipleInputs(toRef.nodeType, toPort)) {
      const targetKey = getTargetPortKey(toRef.nodeType, toRef.id, toPort)
      const duplicateIndex = singleInputConnectionIndex.get(targetKey)
      if (duplicateIndex !== undefined) {
        connections[duplicateIndex] = null
      }
      singleInputConnectionIndex.set(targetKey, connections.length)
    }
    connections.push({
      ...endpoint,
      id: connectionId,
    })
  }

  return connections.filter((connection): connection is CanvasConnection => connection !== null)
}

export function materializeFlowSnapshotConnections({
  edges,
  baseConnections,
  persistedFlowNodeRefs,
  lookupData,
}: FlowSnapshotConnectionMaterializerOptions): CanvasConnection[] {
  const normalizedEdgesByTarget = new Map<string, NormalizedFlowEdgeForResolution[]>()
  const candidates = collectClaimCandidates(
    edges,
    persistedFlowNodeRefs,
    lookupData,
    normalizedEdgesByTarget,
  )

  const claimIndex = createFlowConnectionClaimIndex({
    candidates,
    baseConnections,
    shouldAllocateGeneratedIds: true,
  })
  const allowsMultipleInputs = createInputLimitResolver()
  return resolveClaimedConnections(candidates, claimIndex, allowsMultipleInputs)
}
