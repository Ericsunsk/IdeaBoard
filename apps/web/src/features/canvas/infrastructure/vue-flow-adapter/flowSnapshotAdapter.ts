import {
  CANVAS_NODE_REGISTRY_LIST,
  createEmptyBoardSnapshot,
  isCanvasConnectionNodeType,
  rebuildBoardMediaRefs,
  type CanvasSnapshotNodeCollectionKey,
  type BoardSnapshot,
  type CanvasConnection,
  type CanvasConnectionNodeType,
  type CanvasGroupNodeId,
  type CanvasNodeItem,
  type ImageNodeItem,
} from '../../../board/domain'
import { resolveCanvasGraphNodeDimensions } from '../../domain/graph'
import type { FlowNodeData } from '../../domain/nodes/flowTypes.ts'
import { getDefaultCanvasNodeTitle } from '../../domain/nodes/canvasNodeRegistry.ts'
import { materializeFlowSnapshotConnections } from './flowSnapshotConnectionMaterializer'
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

export type FlowToBoardSnapshotOptions = {
  projection?: FlowSnapshotProjection
}

function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

function createSnapshotShell(base?: BoardSnapshot): BoardSnapshot {
  const snapshot = createEmptyBoardSnapshot()
  const boardRefs = (base?.mediaRefs ?? [])
    .filter((ref) => ref.refType === 'board')
    .map((ref) => ({ ...ref }))
  if (boardRefs.length > 0) snapshot.mediaRefs = boardRefs
  return snapshot
}

function appendCurrentGroups(
  snapshot: BoardSnapshot,
  base: BoardSnapshot | undefined,
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

function toFlowNodeId(nodeType: CanvasConnectionNodeType, id: number) {
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
  snapshot: BoardSnapshot,
  key: CanvasSnapshotNodeCollectionKey,
): CanvasNodeItem[] {
  return snapshot[key] as CanvasNodeItem[]
}

function getSnapshotNodeCollectionsByType(snapshot: BoardSnapshot) {
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

/**
 * Projects one canonical Canvas node record into the Flow shape. Keeping this
 * conversion public lets commit projection replace only changed node IDs
 * instead of rebuilding a Board snapshot and every Flow node on each edit.
 */
export function canvasNodeToFlowNode(
  nodeType: CanvasConnectionNodeType,
  node: CanvasNodeItem,
  selected = false,
): FlowAdapterNode {
  return {
    id: toFlowNodeId(nodeType, node.id),
    type: 'canvas',
    dragHandle: resolveFlowNodeDragHandle(nodeType),
    position: { x: node.x, y: node.y },
    dimensions: resolveCanvasGraphNodeDimensions(nodeType, node),
    data: toFlowDataFromCanvasNode(nodeType, node),
    ...(selected ? { selected: true } : {}),
  }
}

export function boardSnapshotToFlow(snapshot: BoardSnapshot): {
  nodes: FlowAdapterNode[]
  edges: FlowAdapterEdge[]
} {
  const nodes: FlowAdapterNode[] = []
  const edges: FlowAdapterEdge[] = []
  const flowNodeIds = new Set<string>()

  CANVAS_NODE_REGISTRY_LIST.forEach(({ snapshotCollection, kind }) => {
    getSnapshotNodeCollection(snapshot, snapshotCollection).forEach((node) => {
      const id = toFlowNodeId(kind, node.id)
      nodes.push(canvasNodeToFlowNode(kind, node))
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
  data: FlowNodeData,
  options: FlowToBoardSnapshotOptions,
): boolean {
  if (node.type !== 'image-node') return true
  if (data.localUpload && (options.projection ?? 'persistable') !== 'runtime') return false
  const imageNode = node as ImageNodeItem
  if (imageNode.libraryImport) return true
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
    data: { connectionSnapshot: clone(connection) },
  }
}

export function flowToBoardSnapshot(
  input: { nodes: FlowAdapterNode[]; edges: FlowAdapterEdge[] },
  base?: BoardSnapshot,
  options: FlowToBoardSnapshotOptions = {},
): BoardSnapshot {
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
    if (!hasRenderableImageSource(snapshotNode, data, options)) return

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
  rebuildBoardMediaRefs(snapshot, base)

  return snapshot
}
