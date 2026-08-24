import type { Edge, Node } from '@vue-flow/core'

export type CanvasProjectionViewport = Readonly<{
  x: number
  y: number
  zoom: number
}>

export type CanvasProjectionViewportSize = Readonly<{
  width: number
  height: number
}>

export type CanvasVisibleGraphProjectionOptions = Readonly<{
  viewport: CanvasProjectionViewport
  viewportSize?: CanvasProjectionViewportSize
  overscanPx?: number
  alwaysIncludeNodeIds?: ReadonlySet<string>
}>

const DEFAULT_VIEWPORT_SIZE = { width: 1440, height: 900 }
const DEFAULT_OVERSCAN_PX = 720
const DEFAULT_NODE_WIDTH = 280
const DEFAULT_NODE_HEIGHT = 220

function finitePositive(value: number | undefined, fallback: number): number {
  return Number.isFinite(value) && (value as number) > 0 ? (value as number) : fallback
}

function nodeBounds(node: Node) {
  const dimensions = (node as Node & { dimensions?: { width?: number; height?: number } }).dimensions
  const width = finitePositive(dimensions?.width, DEFAULT_NODE_WIDTH)
  const height = finitePositive(dimensions?.height, DEFAULT_NODE_HEIGHT)
  return {
    left: node.position.x,
    top: node.position.y,
    right: node.position.x + width,
    bottom: node.position.y + height,
  }
}

function intersects(
  bounds: ReturnType<typeof nodeBounds>,
  viewport: { left: number; top: number; right: number; bottom: number },
) {
  return (
    bounds.right >= viewport.left &&
    bounds.left <= viewport.right &&
    bounds.bottom >= viewport.top &&
    bounds.top <= viewport.bottom
  )
}

/**
 * Keeps the complete graph out of the Vue Flow renderer while preserving a
 * generous overscan window for smooth panning. The returned arrays retain the
 * canonical order and object identity for items that remain in the window.
 */
export function projectVisibleCanvasGraph(
  graph: Readonly<{ nodes: readonly Node[]; edges: readonly Edge[] }>,
  options: CanvasVisibleGraphProjectionOptions,
): { nodes: Node[]; edges: Edge[] } {
  const zoom = finitePositive(options.viewport.zoom, 1)
  const viewportSize = options.viewportSize ?? DEFAULT_VIEWPORT_SIZE
  const overscanPx = finitePositive(options.overscanPx, DEFAULT_OVERSCAN_PX)
  const overscan = overscanPx / zoom
  const left = (-options.viewport.x - overscan) / zoom
  const top = (-options.viewport.y - overscan) / zoom
  const right = (viewportSize.width - options.viewport.x) / zoom + overscan
  const bottom = (viewportSize.height - options.viewport.y) / zoom + overscan
  const worldViewport = { left, top, right, bottom }
  const alwaysIncludeNodeIds = options.alwaysIncludeNodeIds ?? new Set<string>()
  const visibleNodeIds = new Set<string>()
  const nodes = graph.nodes.filter((node) => {
    const visible = alwaysIncludeNodeIds.has(node.id) || intersects(nodeBounds(node), worldViewport)
    if (visible) visibleNodeIds.add(node.id)
    return visible
  })
  const edges = graph.edges.filter(
    (edge) => visibleNodeIds.has(edge.source) && visibleNodeIds.has(edge.target),
  )
  return { nodes, edges }
}
