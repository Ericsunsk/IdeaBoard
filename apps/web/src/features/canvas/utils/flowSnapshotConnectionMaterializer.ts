import type { Connection } from '@vue-flow/core'
import {
  getCanvasNodePortInputLimit,
  type CanvasBoardSnapshot,
  type CanvasConnection,
  type CanvasConnectionNodeType,
  type CanvasConnectionPort,
} from '../../canvas-board/domain'
import type { FlowNodeData } from '../types/flowTypes'
import { normalizeFlowConnection } from './flowConnectionAdapter'

const CANVAS_CONNECTION_EDGE_ID_PATTERN = /^connection-(\d+)$/

export type FlowSnapshotConnectionNodeRef = {
  nodeType: CanvasConnectionNodeType
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
  baseConnections?: readonly CanvasConnection[]
  persistedFlowNodeRefs: ReadonlyMap<string, FlowSnapshotConnectionNodeRef>
  lookupData: (nodeId: string) => FlowNodeData | undefined
}

export function parsePortFromHandle(
  handle: string | null | undefined,
  direction: 'in' | 'out',
): CanvasConnectionPort | null {
  if (!handle) return null
  const [handleDirection, port] = handle.split(':')
  if (handleDirection !== direction || !port) return null
  return port as CanvasConnectionPort
}

function getRestoredConnectionId(edge: Pick<FlowSnapshotConnectionEdge, 'id' | 'data'>) {
  return resolveFlowEdgeCanvasConnectionId(edge)
}

export function resolveFlowEdgeCanvasConnectionId(
  edge: Pick<FlowSnapshotConnectionEdge, 'id' | 'data'>,
): number | null {
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

function getInitialConnectionId(
  baseConnections: readonly CanvasConnection[] | undefined,
  edges: readonly FlowSnapshotConnectionEdge[],
) {
  let maxConnectionId = 0
  if (baseConnections) {
    for (const connection of baseConnections) {
      if (Number.isFinite(connection.id) && connection.id > maxConnectionId) {
        maxConnectionId = connection.id
      }
    }
  }
  for (const edge of edges) {
    const restoredId = getRestoredConnectionId(edge)
    if (restoredId !== null && restoredId > maxConnectionId) {
      maxConnectionId = restoredId
    }
  }
  return maxConnectionId + 1
}

export function resolveNextCanvasConnectionId(
  base: CanvasBoardSnapshot | undefined,
  edges: readonly FlowSnapshotConnectionEdge[],
) {
  return getInitialConnectionId(base?.connections, edges)
}

function createConnectionIdAllocator(
  baseConnections: readonly CanvasConnection[] | undefined,
  edges: readonly FlowSnapshotConnectionEdge[],
) {
  const usedConnectionIds = new Set<number>()
  let nextGeneratedConnectionId = getInitialConnectionId(baseConnections, edges)

  return (edge: FlowSnapshotConnectionEdge) => {
    const restoredId = getRestoredConnectionId(edge)
    if (restoredId !== null && !usedConnectionIds.has(restoredId)) {
      usedConnectionIds.add(restoredId)
      return restoredId
    }

    while (usedConnectionIds.has(nextGeneratedConnectionId)) {
      nextGeneratedConnectionId += 1
    }
    const connectionId = nextGeneratedConnectionId
    usedConnectionIds.add(connectionId)
    nextGeneratedConnectionId += 1
    return connectionId
  }
}

function getTargetPortKey(
  nodeType: CanvasConnectionNodeType,
  nodeId: number,
  port: CanvasConnectionPort,
) {
  return `${nodeType}:${nodeId}:${port}`
}

function getInputLimitKey(nodeType: CanvasConnectionNodeType, port: CanvasConnectionPort) {
  return `${nodeType}:${port}`
}

function createInputLimitResolver() {
  const inputLimitCache = new Map<string, number>()
  return (nodeType: CanvasConnectionNodeType, port: CanvasConnectionPort) => {
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

export function materializeFlowSnapshotConnections({
  edges,
  baseConnections,
  persistedFlowNodeRefs,
  lookupData,
}: FlowSnapshotConnectionMaterializerOptions): CanvasConnection[] {
  const normalizedEdgesByTarget = new Map<string, NormalizedFlowEdgeForResolution[]>()
  const emptyNormalizedEdges: readonly NormalizedFlowEdgeForResolution[] = []
  const singleInputConnectionIndex = new Map<string, number>()
  const allowsMultipleInputs = createInputLimitResolver()
  const nextConnectionId = createConnectionIdAllocator(baseConnections, edges)
  const connections: Array<CanvasConnection | null> = []

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

    const connectionId = nextConnectionId(edge)
    if (!allowsMultipleInputs(toRef.nodeType, toPort)) {
      const targetKey = getTargetPortKey(toRef.nodeType, toRef.id, toPort)
      const duplicateIndex = singleInputConnectionIndex.get(targetKey)
      if (duplicateIndex !== undefined) {
        connections[duplicateIndex] = null
      }
      singleInputConnectionIndex.set(targetKey, connections.length)
    }
    connections.push({
      id: connectionId,
      fromNodeType: fromRef.nodeType,
      fromNodeId: fromRef.id,
      fromPort,
      toNodeType: toRef.nodeType,
      toNodeId: toRef.id,
      toPort,
    })
  }

  return connections.filter((connection): connection is CanvasConnection => connection !== null)
}
