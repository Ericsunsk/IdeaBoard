import {
  CANVAS_NODE_REGISTRY_LIST,
  createEmptyCanvasBoardSnapshot,
  getCanvasNodePortInputLimit,
  isCanvasConnectionNodeType,
  resolveImageNodeStageHeight,
  type CanvasSnapshotNodeCollectionKey,
  type CanvasBoardSnapshot,
  type CanvasConnection,
  type CanvasConnectionNodeType,
  type CanvasConnectionPort,
  type CanvasNodeItem,
  type ImageNodeItem,
} from '../../canvas-board/domain'
import type { FlowNodeData } from '../types/flowTypes'
import { getDefaultCanvasNodeTitle } from '../config/canvasNodeRegistry'
import { normalizeFlowConnection } from './flowConnectionAdapter'
import { resolveImageNodeDisplayRatio } from './flowImageNodeSelectors'
import { resolveFlowNodeDragHandle } from './flowNodeDragHandle'

export interface FlowAdapterNode {
  id: string
  type?: 'canvas' | string
  dragHandle?: string
  position: { x: number; y: number }
  dimensions?: { width: number; height: number }
  data?: unknown
}

export interface FlowAdapterEdge {
  id: string
  source: string
  target: string
  sourceHandle?: string | null
  targetHandle?: string | null
  animated?: boolean
  class?: string
  data?: unknown
}

function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

function createSnapshotShell(base?: CanvasBoardSnapshot): CanvasBoardSnapshot {
  const snapshot = createEmptyCanvasBoardSnapshot()
  if (base) {
    snapshot.viewport = { ...base.viewport }
  }
  return snapshot
}

function toFlowNodeId(nodeType: CanvasConnectionNodeType, id: number) {
  return `${nodeType}-${id}`
}

function parseFlowNodeId(
  nodeId: string,
): { nodeType: CanvasConnectionNodeType; id: number } | null {
  const dashIndex = nodeId.indexOf('-')
  if (dashIndex <= 0) return null
  const prefix = nodeId.slice(0, dashIndex)
  if (!isCanvasConnectionNodeType(prefix)) return null
  const idPart = nodeId.slice(dashIndex + 1)
  if (!/^\d+(?:\.\d+)?$/.test(idPart)) return null
  const parsedId = Number(idPart)
  if (!Number.isFinite(parsedId)) return null
  return { nodeType: prefix, id: parsedId }
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

function resolveNodeTitle(nodeType: CanvasConnectionNodeType, node: { title?: unknown }): string {
  const title = typeof node.title === 'string' ? node.title.trim() : ''
  return title || getDefaultCanvasNodeTitle(nodeType)
}

function getSnapshotNodeCollection(
  snapshot: CanvasBoardSnapshot,
  key: CanvasSnapshotNodeCollectionKey,
): CanvasNodeItem[] {
  return snapshot[key] as CanvasNodeItem[]
}

function getSnapshotNodeCollectionsByType(snapshot: CanvasBoardSnapshot) {
  const collections = new Map<CanvasConnectionNodeType, CanvasNodeItem[]>()
  CANVAS_NODE_REGISTRY_LIST.forEach(({ kind, snapshotCollection }) => {
    collections.set(kind, getSnapshotNodeCollection(snapshot, snapshotCollection))
  })
  return collections
}

function toFlowDataFromCanvasNode(
  nodeType: CanvasConnectionNodeType,
  node: CanvasNodeItem,
): FlowNodeData {
  return {
    kind: nodeType,
    nodeType,
    title: resolveNodeTitle(nodeType, node),
    summary: `Canvas ${nodeType} node`,
    snapshotNode: clone(node),
  }
}

// Vue Flow replaces this after render; the estimate gives the minimap stable initial geometry.
const PRE_RENDER_MINIMAP_NODE_HEADER_HEIGHT_ESTIMATE = 22

function hasDisplayableImageSource(node: ImageNodeItem): boolean {
  const sourceUrl = node.sourceImageUrl.trim()
  if (sourceUrl) return true
  return Array.isArray(node.outputImages)
    ? node.outputImages.some((item) => item.url.trim().length > 0)
    : false
}

function resolveEstimatedFlowNodeDimensionsForPreRenderMiniMap(
  nodeType: CanvasConnectionNodeType,
  node: CanvasNodeItem,
): { width: number; height: number } {
  const width = Math.max(1, Math.round(node.width))
  const bodyHeight =
    nodeType === 'image'
      ? resolveImageNodeStageHeight(
          width,
          resolveImageNodeDisplayRatio(node as ImageNodeItem),
          hasDisplayableImageSource(node as ImageNodeItem),
        )
      : width
  return {
    width,
    height: Math.max(1, Math.round(PRE_RENDER_MINIMAP_NODE_HEADER_HEIGHT_ESTIMATE + bodyHeight)),
  }
}

export function canvasBoardSnapshotToFlow(snapshot: CanvasBoardSnapshot): {
  nodes: FlowAdapterNode[]
  edges: FlowAdapterEdge[]
} {
  const nodes: FlowAdapterNode[] = []
  const edges: FlowAdapterEdge[] = []
  const flowNodeIds = new Set<string>()

  CANVAS_NODE_REGISTRY_LIST.forEach(({ snapshotCollection, kind }) => {
    getSnapshotNodeCollection(snapshot, snapshotCollection).forEach((node) => {
      const id = toFlowNodeId(kind, node.id)
      nodes.push({
        id,
        type: 'canvas',
        dragHandle: resolveFlowNodeDragHandle(kind),
        position: { x: node.x, y: node.y },
        dimensions: resolveEstimatedFlowNodeDimensionsForPreRenderMiniMap(kind, node),
        data: toFlowDataFromCanvasNode(kind, node),
      })
      flowNodeIds.add(id)
    })
  })

  snapshot.connections.forEach((connection) => {
    const sourceNodeId = toFlowNodeId(connection.fromNodeType, connection.fromNodeId)
    const targetNodeId = toFlowNodeId(connection.toNodeType, connection.toNodeId)
    if (!flowNodeIds.has(sourceNodeId) || !flowNodeIds.has(targetNodeId)) return
    const sourceHandle = `out:${connection.fromPort}`
    const targetHandle = `in:${connection.toPort}`
    edges.push({
      id: `connection-${connection.id}`,
      source: sourceNodeId,
      sourceHandle,
      target: targetNodeId,
      targetHandle,
      animated: false,
      data: { connectionSnapshot: clone(connection) },
    })
  })

  return { nodes, edges }
}

function hasRenderableImageSource(node: CanvasNodeItem): boolean {
  if (node.type !== 'image-node') return true
  const imageNode = node as ImageNodeItem
  const sourceUrl = imageNode.sourceImageUrl.trim()
  const hasOutputImage = Array.isArray(imageNode.outputImages)
    ? imageNode.outputImages.some(
        (item) => typeof item?.url === 'string' && item.url.trim().length > 0,
      )
    : false
  const pendingTaskId =
    typeof imageNode.pendingTaskId === 'string' ? imageNode.pendingTaskId.trim() : ''
  if (imageNode.outputStatus === 'running' && !sourceUrl && !hasOutputImage && !pendingTaskId) {
    return false
  }
  return true
}

function getRestoredConnectionId(edge: FlowAdapterEdge): number | null {
  const data = edge.data
  if (!data || typeof data !== 'object') return null
  const restoredId = (data as { connectionSnapshot?: { id?: unknown } }).connectionSnapshot?.id
  return typeof restoredId === 'number' && Number.isFinite(restoredId) ? restoredId : null
}

function getInitialConnectionId(
  base: CanvasBoardSnapshot | undefined,
  edges: readonly FlowAdapterEdge[],
) {
  let maxConnectionId = 0
  if (base) {
    for (const connection of base.connections) {
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

export function flowToCanvasBoardSnapshot(
  input: { nodes: FlowAdapterNode[]; edges: FlowAdapterEdge[] },
  base?: CanvasBoardSnapshot,
): CanvasBoardSnapshot {
  const snapshot = createSnapshotShell(base)

  const snapshotNodeCollections = getSnapshotNodeCollectionsByType(snapshot)
  const usedConnectionIds = new Set<number>()
  const persistedFlowNodeRefs = new Map<
    string,
    { nodeType: CanvasConnectionNodeType; id: number }
  >()
  const flowNodeDataMap = new Map<string, FlowNodeData>()
  type NormalizedFlowEdgeForResolution = Pick<FlowAdapterEdge, 'id' | 'target' | 'targetHandle'>
  const normalizedEdgesByTarget = new Map<string, NormalizedFlowEdgeForResolution[]>()
  const emptyNormalizedEdges: readonly NormalizedFlowEdgeForResolution[] = []
  const singleInputConnectionIndex = new Map<string, number>()
  const inputLimitCache = new Map<string, number>()
  const connections: Array<CanvasConnection | null> = []
  let nextGeneratedConnectionId = getInitialConnectionId(base, input.edges)

  const nextConnectionId = (edge: FlowAdapterEdge) => {
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

  const allowsMultipleInputs = (nodeType: CanvasConnectionNodeType, port: CanvasConnectionPort) => {
    const key = getInputLimitKey(nodeType, port)
    const cachedLimit = inputLimitCache.get(key)
    if (cachedLimit !== undefined) return cachedLimit > 1
    const limit = getCanvasNodePortInputLimit(nodeType, port)
    inputLimitCache.set(key, limit)
    return limit > 1
  }

  input.nodes.forEach((node) => {
    const ref = parseFlowNodeId(node.id)
    if (!ref) return
    const data = (node.data ?? {}) as FlowNodeData
    const snapshotNode = data.snapshotNode as CanvasNodeItem | undefined
    if (!snapshotNode || typeof snapshotNode !== 'object') return
    if (!hasRenderableImageSource(snapshotNode)) return

    const positionedNode = clone(snapshotNode) as CanvasNodeItem
    positionedNode.x = node.position.x
    positionedNode.y = node.position.y

    snapshotNodeCollections.get(ref.nodeType)?.push(positionedNode)
    persistedFlowNodeRefs.set(node.id, ref)
    flowNodeDataMap.set(node.id, data)
  })

  input.edges.forEach((edge) => {
    const fromRef = persistedFlowNodeRefs.get(edge.source)
    const toRef = persistedFlowNodeRefs.get(edge.target)
    if (!fromRef || !toRef) return
    const normalizedEdge = normalizeFlowConnection(
      edge,
      (nodeId) => flowNodeDataMap.get(nodeId),
      normalizedEdgesByTarget.get(edge.target) ?? emptyNormalizedEdges,
    )
    const sourceHandle = normalizedEdge?.sourceHandle ?? edge.sourceHandle
    const targetHandle = normalizedEdge?.targetHandle ?? edge.targetHandle
    const fromPort = parsePortFromHandle(sourceHandle ?? null, 'out')
    const toPort = parsePortFromHandle(targetHandle ?? null, 'in')
    if (!fromPort || !toPort) return

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

    const connectionId = nextConnectionId(edge)
    if (!allowsMultipleInputs(toRef.nodeType, toPort)) {
      // Remove any existing connection targeting the same input port (last-write wins).
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
  })

  snapshot.connections = connections.filter(
    (connection): connection is CanvasConnection => connection !== null,
  )
  return snapshot
}
