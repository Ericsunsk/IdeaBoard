import type {
  CanvasBoardSnapshot,
  CanvasConnection,
  CanvasConnectionNodeType,
  CanvasConnectionPort,
  CanvasImageItem,
  CanvasNodeItem,
} from '../types/canvasNodes'
import type { FlowNodeData } from '../types/flowTypes'
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
  | 'maskNodes'
  | 'colorNodes'
  | 'editNodes'
  | 'fusionNodes'
  | 'enhanceNodes'
  | 'wearNodes'

const SNAPSHOT_NODE_COLLECTIONS: ReadonlyArray<{
  key: SnapshotNodeCollectionKey
  nodeType: CanvasConnectionNodeType
}> = [
  { key: 'imageNodes', nodeType: 'image' },
  { key: 'promptNodes', nodeType: 'prompt' },
  { key: 'modelNodes', nodeType: 'model' },
  { key: 'viewpointNodes', nodeType: 'viewpoint' },
  { key: 'maskNodes', nodeType: 'mask' },
  { key: 'colorNodes', nodeType: 'color' },
  { key: 'editNodes', nodeType: 'edit' },
  { key: 'fusionNodes', nodeType: 'fusion' },
  { key: 'enhanceNodes', nodeType: 'enhance' },
  { key: 'wearNodes', nodeType: 'wear' },
]

function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

function cloneSnapshot(snapshot: CanvasBoardSnapshot): CanvasBoardSnapshot {
  return clone(snapshot)
}

export function createEmptyCanvasBoardSnapshot(): CanvasBoardSnapshot {
  return {
    messages: [],
    imageNodes: [],
    promptNodes: [],
    modelNodes: [],
    viewpointNodes: [],
    maskNodes: [],
    colorNodes: [],
    editNodes: [],
    fusionNodes: [],
    enhanceNodes: [],
    wearNodes: [],
    connections: [],
    viewport: {
      canvasX: 0,
      canvasY: 0,
      zoomLevel: 100,
    },
  }
}

function toFlowNodeId(nodeType: CanvasConnectionNodeType | 'message', id: number) {
  return `${nodeType}-${id}`
}

function parseFlowNodeId(
  nodeId: string,
): { nodeType: CanvasConnectionNodeType | 'message'; id: number } | null {
  const dashIndex = nodeId.indexOf('-')
  if (dashIndex <= 0) return null
  const prefix = nodeId.slice(0, dashIndex)
  const idPart = nodeId.slice(dashIndex + 1)
  if (!/^\d+(?:\.\d+)?$/.test(idPart)) return null
  const parsedId = Number(idPart)
  if (!Number.isFinite(parsedId)) return null
  return { nodeType: prefix as CanvasConnectionNodeType | 'message', id: parsedId }
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

function resolveNodeTitle(
  nodeType: CanvasConnectionNodeType | 'message',
  node: { id: number },
): string {
  if (nodeType === 'message') return `Message #${node.id}`
  return `${nodeType[0].toUpperCase()}${nodeType.slice(1)} #${node.id}`
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
      nodeType === 'edit' || nodeType === 'enhance'
        ? (node as { mode?: FlowNodeData['mode'] }).mode
        : undefined,
    snapshotNode: clone(node),
  }
}

function toFlowDataFromMessageNode(node: CanvasImageItem): FlowNodeData {
  return {
    kind: 'message',
    title: resolveNodeTitle('message', node),
    summary: 'Canvas message image',
    snapshotNode: clone(node),
  }
}

export function canvasBoardSnapshotToFlow(snapshot: CanvasBoardSnapshot): {
  nodes: FlowAdapterNode[]
  edges: FlowAdapterEdge[]
} {
  const nodes: FlowAdapterNode[] = []
  const edges: FlowAdapterEdge[] = []

  snapshot.messages.forEach((message) => {
    nodes.push({
      id: toFlowNodeId('message', message.id),
      type: 'canvas',
      dragHandle: resolveFlowNodeDragHandle('message'),
      position: { x: message.x, y: message.y },
      data: toFlowDataFromMessageNode(message),
    })
  })

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
    case 'mask':
      return 'maskNodes'
    case 'color':
      return 'colorNodes'
    case 'edit':
      return 'editNodes'
    case 'fusion':
      return 'fusionNodes'
    case 'enhance':
      return 'enhanceNodes'
    case 'wear':
      return 'wearNodes'
  }
}

export function flowToCanvasBoardSnapshot(
  input: { nodes: FlowAdapterNode[]; edges: FlowAdapterEdge[] },
  base?: CanvasBoardSnapshot,
): CanvasBoardSnapshot {
  const snapshot = base ? cloneSnapshot(base) : createEmptyCanvasBoardSnapshot()

  snapshot.messages = []
  SNAPSHOT_NODE_COLLECTIONS.forEach(({ key }) => {
    snapshot[key] = []
  })
  snapshot.connections = []
  const usedConnectionIds = new Set<number>()

  const nextConnectionId = () => {
    const existingIds = snapshot.connections.map((connection) => connection.id)
    const maxId = existingIds.length ? Math.max(...existingIds) : 0
    return maxId + 1
  }

  input.nodes.forEach((node) => {
    const ref = parseFlowNodeId(node.id)
    if (!ref) return
    const data = (node.data ?? {}) as FlowNodeData
    const snapshotNode = data.snapshotNode as CanvasNodeItem | CanvasImageItem | undefined
    if (!snapshotNode || typeof snapshotNode !== 'object') return
    const positionedNode = {
      ...snapshotNode,
      x: node.position.x,
      y: node.position.y,
    } as CanvasNodeItem | CanvasImageItem

    if (ref.nodeType === 'message') {
      snapshot.messages.push(positionedNode as CanvasImageItem)
      return
    }
    const collectionKey = resolveCollectionKeyByNodeType(ref.nodeType)
    ;(snapshot[collectionKey] as CanvasNodeItem[]).push(positionedNode as CanvasNodeItem)
  })

  input.edges.forEach((edge) => {
    const fromRef = parseFlowNodeId(edge.source)
    const toRef = parseFlowNodeId(edge.target)
    if (!fromRef || !toRef) return
    if (fromRef.nodeType === 'message' || toRef.nodeType === 'message') return
    const fromPort = parsePortFromHandle(edge.sourceHandle ?? null, 'out')
    const toPort = parsePortFromHandle(edge.targetHandle ?? null, 'in')
    if (!fromPort || !toPort) return

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
    // Remove any existing connection targeting the same input port (last-write wins)
    const dupIndex = snapshot.connections.findIndex(
      (c) => c.toNodeType === toRef.nodeType && c.toNodeId === toRef.id && c.toPort === toPort,
    )
    if (dupIndex >= 0) {
      snapshot.connections.splice(dupIndex, 1)
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
