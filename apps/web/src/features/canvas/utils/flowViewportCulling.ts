type FlowViewport = {
  x: number
  y: number
  zoom: number
}

type FlowViewportSize = {
  width: number
  height: number
}

type FlowGraphNodeLike = {
  id: string
  position: { x: number; y: number }
  width?: unknown
  height?: unknown
  dimensions?: { width?: unknown; height?: unknown } | null
  measured?: { width?: unknown; height?: unknown } | null
  data?: unknown
}

type FlowGraphEdgeLike = {
  source: string
  target: string
}

export type FlowViewportCullingOptions = {
  viewport: FlowViewport
  viewportSize: FlowViewportSize
  overscanPx?: number
}

export type FlowViewportCullingResult<NodeT, EdgeT> = {
  nodes: NodeT[]
  edges: EdgeT[]
  visibleNodeIds: Set<string>
}

export const FLOW_GRAPH_VIEWPORT_OVERSCAN_PX = 700
export const FALLBACK_FLOW_VIEWPORT_SIZE: FlowViewportSize = { width: 1440, height: 900 }

const DEFAULT_NODE_WIDTH = 320
const DEFAULT_NODE_HEIGHT = 420

export function isCanvasFlowCullingEnabled(): boolean {
  return import.meta.env.VITE_CANVAS_FLOW_ENABLE_CULLING !== 'false'
}

function toPositiveFiniteNumber(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value) && value > 0 ? value : null
}

function readSnapshotNodeSize(node: FlowGraphNodeLike): {
  width: number | null
  height: number | null
} {
  const data = node.data
  if (!data || typeof data !== 'object') return { width: null, height: null }
  const snapshotNode = (data as { snapshotNode?: unknown }).snapshotNode
  if (!snapshotNode || typeof snapshotNode !== 'object') return { width: null, height: null }
  const snapshot = snapshotNode as { width?: unknown; height?: unknown }
  return {
    width: toPositiveFiniteNumber(snapshot.width),
    height: toPositiveFiniteNumber(snapshot.height),
  }
}

function resolveNodeBounds(node: FlowGraphNodeLike) {
  const snapshotSize = readSnapshotNodeSize(node)
  const width =
    toPositiveFiniteNumber(node.width) ??
    toPositiveFiniteNumber(node.dimensions?.width) ??
    toPositiveFiniteNumber(node.measured?.width) ??
    snapshotSize.width ??
    DEFAULT_NODE_WIDTH
  const height =
    toPositiveFiniteNumber(node.height) ??
    toPositiveFiniteNumber(node.dimensions?.height) ??
    toPositiveFiniteNumber(node.measured?.height) ??
    snapshotSize.height ??
    DEFAULT_NODE_HEIGHT

  return {
    left: node.position.x,
    top: node.position.y,
    right: node.position.x + width,
    bottom: node.position.y + height,
  }
}

function normalizeViewportSize(size: FlowViewportSize): FlowViewportSize {
  return {
    width: toPositiveFiniteNumber(size.width) ?? FALLBACK_FLOW_VIEWPORT_SIZE.width,
    height: toPositiveFiniteNumber(size.height) ?? FALLBACK_FLOW_VIEWPORT_SIZE.height,
  }
}

function resolveVisibleFlowRect(options: FlowViewportCullingOptions) {
  const viewportSize = normalizeViewportSize(options.viewportSize)
  const zoom = toPositiveFiniteNumber(options.viewport.zoom) ?? 1
  const overscan = Math.max(0, options.overscanPx ?? FLOW_GRAPH_VIEWPORT_OVERSCAN_PX)
  return {
    left: (-options.viewport.x - overscan) / zoom,
    top: (-options.viewport.y - overscan) / zoom,
    right: (viewportSize.width - options.viewport.x + overscan) / zoom,
    bottom: (viewportSize.height - options.viewport.y + overscan) / zoom,
  }
}

function intersects(
  a: { left: number; top: number; right: number; bottom: number },
  b: { left: number; top: number; right: number; bottom: number },
) {
  return a.left <= b.right && a.right >= b.left && a.top <= b.bottom && a.bottom >= b.top
}

export function cullFlowGraphForViewport<
  NodeT extends FlowGraphNodeLike,
  EdgeT extends FlowGraphEdgeLike,
>(
  graph: { nodes: readonly NodeT[]; edges: readonly EdgeT[] },
  options: FlowViewportCullingOptions,
): FlowViewportCullingResult<NodeT, EdgeT> {
  const visibleRect = resolveVisibleFlowRect(options)
  const visibleNodeIds = new Set<string>()
  const nodes: NodeT[] = []

  for (const node of graph.nodes) {
    if (!intersects(resolveNodeBounds(node), visibleRect)) continue
    visibleNodeIds.add(node.id)
    nodes.push(node)
  }

  const edges = graph.edges.filter(
    (edge) => visibleNodeIds.has(edge.source) && visibleNodeIds.has(edge.target),
  )

  return { nodes, edges, visibleNodeIds }
}
