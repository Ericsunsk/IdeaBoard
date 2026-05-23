import type {
  CanvasBoardSnapshot,
  CanvasConnection,
  CanvasConnectionNodeType,
  CanvasConnectionPort,
  CanvasNodeItem,
  ImageNodeItem,
} from '../types/canvasNodes'
import type { FlowNodeData } from '../types/flowTypes'
import { getDefaultCanvasNodeTitle } from '../types/canvasNodes'
import { normalizeFlowConnection } from './flowConnectionAdapter'
import { resolveFlowNodeDragHandle } from './flowNodeDragHandle'

export interface FlowAdapterNode {
  id: string
  type?: 'canvas' | string
  dragHandle?: string
  position: { x: number; y: number }
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

type SnapshotNodeCollectionKey =
  | 'imageNodes'
  | 'promptNodes'
  | 'modelNodes'
  | 'viewpointNodes'
  | 'editNodes'
  | 'fusionNodes'

const SNAPSHOT_NODE_COLLECTIONS: ReadonlyArray<{
  key: SnapshotNodeCollectionKey
  nodeType: CanvasConnectionNodeType
}> = [
  { key: 'imageNodes', nodeType: 'image' },
  { key: 'promptNodes', nodeType: 'prompt' },
  { key: 'modelNodes', nodeType: 'model' },
  { key: 'viewpointNodes', nodeType: 'viewpoint' },
  { key: 'editNodes', nodeType: 'edit' },
  { key: 'fusionNodes', nodeType: 'fusion' },
]

function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

function cloneSnapshot(snapshot: CanvasBoardSnapshot): CanvasBoardSnapshot {
  return clone(snapshot)
}

export function createEmptyCanvasBoardSnapshot(): CanvasBoardSnapshot {
  return {
    imageNodes: [],
    promptNodes: [],
    modelNodes: [],
    viewpointNodes: [],
    editNodes: [],
    fusionNodes: [],
    connections: [],
    viewport: {
      canvasX: 0,
      canvasY: 0,
      zoomLevel: 100,
    },
  }
}

function toFlowNodeId(nodeType: CanvasConnectionNodeType, id: number) {
  return `${nodeType}-${id}`
}

function isCanvasConnectionNodeType(value: string): value is CanvasConnectionNodeType {
  return SNAPSHOT_NODE_COLLECTIONS.some((item) => item.nodeType === value)
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

function toFlowDataFromCanvasNode(
  nodeType: CanvasConnectionNodeType,
  node: CanvasNodeItem,
): FlowNodeData {
  return {
    kind: nodeType,
    nodeType,
    title: resolveNodeTitle(nodeType, node),
    summary: `Canvas ${nodeType} node`,
    mode:
      nodeType === 'edit'
        ? (node as { mode?: FlowNodeData['mode'] }).mode
        : undefined,
    snapshotNode: clone(node),
  }
}

export function canvasBoardSnapshotToFlow(snapshot: CanvasBoardSnapshot): {
  nodes: FlowAdapterNode[]
  edges: FlowAdapterEdge[]
} {
  const nodes: FlowAdapterNode[] = []
  const edges: FlowAdapterEdge[] = []

  SNAPSHOT_NODE_COLLECTIONS.forEach(({ key, nodeType }) => {
    snapshot[key].forEach((node) => {
      nodes.push({
        id: toFlowNodeId(nodeType, node.id),
        type: 'canvas',
        dragHandle: resolveFlowNodeDragHandle(nodeType),
        position: { x: node.x, y: node.y },
        data: toFlowDataFromCanvasNode(nodeType, node),
      })
    })
  })

  snapshot.connections.forEach((connection) => {
    const sourceHandle = `out:${connection.fromPort}`
    const targetHandle = `in:${connection.toPort}`
    edges.push({
      id: `connection-${connection.id}`,
      source: toFlowNodeId(connection.fromNodeType, connection.fromNodeId),
      sourceHandle,
      target: toFlowNodeId(connection.toNodeType, connection.toNodeId),
      targetHandle,
      animated: false,
      data: { connectionSnapshot: clone(connection) },
    })
  })

  return { nodes, edges }
}

function resolveCollectionKeyByNodeType(
  nodeType: CanvasConnectionNodeType,
): SnapshotNodeCollectionKey {
  switch (nodeType) {
    case 'image':
      return 'imageNodes'
    case 'prompt':
      return 'promptNodes'
    case 'model':
      return 'modelNodes'
    case 'viewpoint':
      return 'viewpointNodes'
    case 'edit':
      return 'editNodes'
    case 'fusion':
      return 'fusionNodes'
  }
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
  if (imageNode.outputStatus === 'running' && !sourceUrl && !hasOutputImage) return false
  return true
}

export function flowToCanvasBoardSnapshot(
  input: { nodes: FlowAdapterNode[]; edges: FlowAdapterEdge[] },
  base?: CanvasBoardSnapshot,
): CanvasBoardSnapshot {
  const snapshot = base ? cloneSnapshot(base) : createEmptyCanvasBoardSnapshot()

  SNAPSHOT_NODE_COLLECTIONS.forEach(({ key }) => {
    snapshot[key] = []
  })
  snapshot.connections = []
  const usedConnectionIds = new Set<number>()
  const persistedFlowNodeIds = new Set<string>()
  const flowNodeDataMap = new Map<string, FlowNodeData>()
  const normalizedEdgesForResolution: FlowAdapterEdge[] = []

  const nextConnectionId = () => {
    const existingIds = snapshot.connections.map((connection) => connection.id)
    const maxId = existingIds.length ? Math.max(...existingIds) : 0
    return maxId + 1
  }

  input.nodes.forEach((node) => {
    const ref = parseFlowNodeId(node.id)
    if (!ref) return
    const data = (node.data ?? {}) as FlowNodeData
    flowNodeDataMap.set(node.id, data)
    const snapshotNode = data.snapshotNode as CanvasNodeItem | undefined
    if (!snapshotNode || typeof snapshotNode !== 'object') return
    const positionedNode = {
      ...snapshotNode,
      x: node.position.x,
      y: node.position.y,
    } as CanvasNodeItem
    if (!hasRenderableImageSource(positionedNode)) return

    const collectionKey = resolveCollectionKeyByNodeType(ref.nodeType)
    ;(snapshot[collectionKey] as CanvasNodeItem[]).push(positionedNode as CanvasNodeItem)
    persistedFlowNodeIds.add(node.id)
  })

  input.edges.forEach((edge) => {
    if (!persistedFlowNodeIds.has(edge.source) || !persistedFlowNodeIds.has(edge.target)) return
    const fromRef = parseFlowNodeId(edge.source)
    const toRef = parseFlowNodeId(edge.target)
    if (!fromRef || !toRef) return
    const normalizedEdge = normalizeFlowConnection(
      edge,
      (nodeId) => flowNodeDataMap.get(nodeId),
      normalizedEdgesForResolution,
    )
    const sourceHandle = normalizedEdge?.sourceHandle ?? edge.sourceHandle
    const targetHandle = normalizedEdge?.targetHandle ?? edge.targetHandle
    const fromPort = parsePortFromHandle(sourceHandle ?? null, 'out')
    const toPort = parsePortFromHandle(targetHandle ?? null, 'in')
    if (!fromPort || !toPort) return
    normalizedEdgesForResolution.push({
      ...edge,
      sourceHandle: `out:${fromPort}`,
      targetHandle: `in:${toPort}`,
    })

    const existingConnection = (edge.data as { connectionSnapshot?: CanvasConnection } | undefined)
      ?.connectionSnapshot
    const restoredId = existingConnection?.id
    const connectionId =
      typeof restoredId === 'number' &&
      Number.isFinite(restoredId) &&
      !usedConnectionIds.has(restoredId)
        ? restoredId
        : nextConnectionId()
    usedConnectionIds.add(connectionId)
    const allowsMultipleInputs =
      (toRef.nodeType === 'prompt' || toRef.nodeType === 'fusion') && toPort === 'image'
    if (!allowsMultipleInputs) {
      // Remove any existing connection targeting the same input port (last-write wins).
      const dupIndex = snapshot.connections.findIndex(
        (c) => c.toNodeType === toRef.nodeType && c.toNodeId === toRef.id && c.toPort === toPort,
      )
      if (dupIndex >= 0) {
        snapshot.connections.splice(dupIndex, 1)
      }
    }
    snapshot.connections.push({
      id: connectionId,
      fromNodeType: fromRef.nodeType,
      fromNodeId: fromRef.id,
      fromPort,
      toNodeType: toRef.nodeType,
      toNodeId: toRef.id,
      toPort,
    })
  })

  return snapshot
}
