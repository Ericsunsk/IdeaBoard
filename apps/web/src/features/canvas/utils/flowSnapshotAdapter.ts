import {
  CANVAS_NODE_REGISTRY_LIST,
  createEmptyCanvasBoardSnapshot,
  isCanvasConnectionNodeType,
  rebuildCanvasBoardMediaRefs,
  resolveImageNodeStageHeight,
  type CanvasSnapshotNodeCollectionKey,
  type CanvasBoardSnapshot,
  type CanvasConnection,
  type CanvasConnectionNodeType,
  type CanvasGroupNodeId,
  type CanvasNodeItem,
  type ImageNodeItem,
} from '../../canvas-board/domain'
import type { FlowNodeData } from '../types/flowTypes'
import { getDefaultCanvasNodeTitle } from '../config/canvasNodeRegistry'
import { materializeFlowSnapshotConnections } from './flowSnapshotConnectionMaterializer'
import { resolveImageNodeDisplayRatio } from './flowImageNodeSelectors'
import { resolveFlowNodeDragHandle } from './flowNodeDragHandle'

export { resolveNextCanvasConnectionId } from './flowSnapshotConnectionMaterializer'

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

export type FlowSnapshotProjection = 'persistable' | 'runtime'

export type FlowToCanvasBoardSnapshotOptions = {
  projection?: FlowSnapshotProjection
}

function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

function createSnapshotShell(base?: CanvasBoardSnapshot): CanvasBoardSnapshot {
  const snapshot = createEmptyCanvasBoardSnapshot()
  const boardRefs = (base?.mediaRefs ?? [])
    .filter((ref) => ref.refType === 'board')
    .map((ref) => ({ ...ref }))
  if (boardRefs.length > 0) snapshot.mediaRefs = boardRefs
  return snapshot
}

function appendCurrentGroups(
  snapshot: CanvasBoardSnapshot,
  base: CanvasBoardSnapshot | undefined,
  persistedCanvasNodeIds: ReadonlySet<string>,
) {
  const assignedCanvasNodeIds = new Set<string>()
  snapshot.groups = (base ? base.groups : []).map((group) => {
    const nodeIds: CanvasGroupNodeId[] = []
    group.nodeIds.forEach((nodeId) => {
      if (!persistedCanvasNodeIds.has(nodeId) || assignedCanvasNodeIds.has(nodeId)) return
      assignedCanvasNodeIds.add(nodeId)
      nodeIds.push(nodeId)
    })
    return {
      id: group.id,
      nodeIds,
      x: group.x,
      y: group.y,
      width: group.width,
      height: group.height,
    }
  })
}

export function toFlowNodeId(nodeType: CanvasConnectionNodeType, id: number) {
  return `${nodeType}-${id}`
}

export function parseFlowNodeId(
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

export function resolveFlowNodeCanvasRef(
  node: Pick<FlowAdapterNode, 'id' | 'data'>,
): { nodeType: CanvasConnectionNodeType; id: number } | null {
  const parsed = parseFlowNodeId(node.id)
  if (parsed) return parsed

  const data = (node.data ?? {}) as Partial<FlowNodeData>
  const snapshotNode = data.snapshotNode as Partial<CanvasNodeItem> | undefined
  const nodeType = isCanvasConnectionNodeType(data.nodeType)
    ? data.nodeType
    : isCanvasConnectionNodeType(data.kind)
      ? data.kind
      : snapshotNode?.type === 'image-node'
        ? 'image'
        : isCanvasConnectionNodeType(snapshotNode?.type)
          ? snapshotNode.type
          : null
  const id = Number(snapshotNode?.id)
  if (!nodeType || !Number.isInteger(id) || id < 1) return null
  return { nodeType, id }
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
    edges.push(canvasConnectionToFlowEdge(connection))
  })

  return { nodes, edges }
}

function hasRenderableImageSource(
  node: CanvasNodeItem,
  options: FlowToCanvasBoardSnapshotOptions,
): boolean {
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
    const projection = options.projection ?? 'persistable'
    return projection === 'runtime'
  }
  return true
}

export function canvasConnectionToFlowEdge(connection: CanvasConnection): FlowAdapterEdge {
  return {
    id: `connection-${connection.id}`,
    source: toFlowNodeId(connection.fromNodeType, connection.fromNodeId),
    sourceHandle: `out:${connection.fromPort}`,
    target: toFlowNodeId(connection.toNodeType, connection.toNodeId),
    targetHandle: `in:${connection.toPort}`,
    animated: false,
    data: { connectionSnapshot: clone(connection) },
  }
}

export function flowToCanvasBoardSnapshot(
  input: { nodes: FlowAdapterNode[]; edges: FlowAdapterEdge[] },
  base?: CanvasBoardSnapshot,
  options: FlowToCanvasBoardSnapshotOptions = {},
): CanvasBoardSnapshot {
  const snapshot = createSnapshotShell(base)

  const snapshotNodeCollections = getSnapshotNodeCollectionsByType(snapshot)
  const persistedFlowNodeRefs = new Map<
    string,
    { nodeType: CanvasConnectionNodeType; id: number }
  >()
  const persistedCanvasNodeIds = new Set<string>()
  const flowNodeDataMap = new Map<string, FlowNodeData>()

  input.nodes.forEach((node) => {
    const ref = resolveFlowNodeCanvasRef(node)
    if (!ref) return
    const data = (node.data ?? {}) as FlowNodeData
    const snapshotNode = data.snapshotNode as CanvasNodeItem | undefined
    if (!snapshotNode || typeof snapshotNode !== 'object') return
    if (!hasRenderableImageSource(snapshotNode, options)) return

    const positionedNode = clone(snapshotNode) as CanvasNodeItem
    positionedNode.x = node.position.x
    positionedNode.y = node.position.y

    snapshotNodeCollections.get(ref.nodeType)?.push(positionedNode)
    persistedFlowNodeRefs.set(node.id, ref)
    persistedCanvasNodeIds.add(toFlowNodeId(ref.nodeType, ref.id))
    flowNodeDataMap.set(node.id, data)
  })

  snapshot.connections = materializeFlowSnapshotConnections({
    edges: input.edges,
    baseConnections: base?.connections,
    persistedFlowNodeRefs,
    lookupData: (nodeId) => flowNodeDataMap.get(nodeId),
  })
  appendCurrentGroups(snapshot, base, persistedCanvasNodeIds)
  rebuildCanvasBoardMediaRefs(snapshot, base)

  return snapshot
}
