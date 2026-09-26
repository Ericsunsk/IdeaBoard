import type { CanvasGroup } from '../../../board'
import {
  isCanvasGraphNodeId,
  type CanvasGraphGroupId,
  type CanvasGraphGroupGeometry,
  type CanvasGraphNodeId,
} from '.'

/**
 * Minimal structural Flow node shape the pure group geometry needs.
 *
 * The domain must not import renderer DTOs (`@vue-flow/core`): real Flow
 * nodes satisfy this shape structurally, and the application layer keeps
 * owning the concrete `Node` type.
 */
export type CanvasFlowNodeLike = {
  id: string
  position: { x: number; y: number }
  zIndex?: number | undefined
  width?: unknown
  height?: unknown
  dimensions?: { width?: unknown; height?: unknown } | undefined
  data?: unknown
}

export const CANVAS_GROUP_FRAME_PADDING_PX = 10
const CANVAS_GROUP_FRAME_MIN_WIDTH_PX = 120
const CANVAS_GROUP_FRAME_MIN_HEIGHT_PX = 80
const CANVAS_GROUP_NODE_Z_INDEX_BASE = 1
const CANVAS_GROUP_NODE_Z_INDEX_GROUPED = 2

export type CanvasFlowViewportTransform = {
  x: number
  y: number
  zoom: number
}

export type CanvasPoint = {
  x: number
  y: number
}

export type CanvasRect = {
  left: number
  top: number
  right: number
  bottom: number
  width: number
  height: number
}

export type CanvasGroupFrame = {
  id: CanvasGraphGroupId
  nodeIds: CanvasGraphNodeId[]
  bounds: CanvasRect
  selected: boolean
  order: number
}

export type CanvasGroupResizeHandle = 'n' | 'ne' | 'e' | 'se' | 's' | 'sw' | 'w' | 'nw'

export type CanvasSelectionGroupGeometryAction =
  | {
      kind: 'group'
      nodeIds: readonly CanvasGraphNodeId[]
    }
  | {
      kind: 'ungroup'
      groupId: CanvasGraphGroupId
      nodeIds: readonly CanvasGraphNodeId[]
    }

type FlowNodeSize = {
  width: number
  height: number
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object'
}

function readPositiveNumber(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value) && value > 0 ? value : null
}

function readSnapshotNodeSize(node: CanvasFlowNodeLike): FlowNodeSize | null {
  const data = isRecord(node.data) ? node.data : null
  const snapshotNode = data && isRecord(data.snapshotNode) ? data.snapshotNode : null
  const width = readPositiveNumber(snapshotNode?.width)
  const height = readPositiveNumber(snapshotNode?.height)
  if (!width && !height) return null
  const resolvedWidth = width ?? height ?? 1
  return {
    width: resolvedWidth,
    height: height ?? resolvedWidth,
  }
}

function readFlowNodeSize(node: CanvasFlowNodeLike): FlowNodeSize {
  const dimensions = (node as { dimensions?: { width?: unknown; height?: unknown } }).dimensions
  const dimensionsWidth = readPositiveNumber(dimensions?.width)
  const dimensionsHeight = readPositiveNumber(dimensions?.height)
  if (dimensionsWidth && dimensionsHeight) {
    return { width: dimensionsWidth, height: dimensionsHeight }
  }

  const directWidth = readPositiveNumber((node as { width?: unknown }).width)
  const directHeight = readPositiveNumber((node as { height?: unknown }).height)
  if (directWidth && directHeight) {
    return { width: directWidth, height: directHeight }
  }

  const snapshotSize = readSnapshotNodeSize(node)
  if (snapshotSize) return snapshotSize

  return { width: 1, height: 1 }
}

function readFlowNodeBounds(node: CanvasFlowNodeLike): CanvasRect {
  const size = readFlowNodeSize(node)
  return {
    left: node.position.x,
    top: node.position.y,
    right: node.position.x + size.width,
    bottom: node.position.y + size.height,
    width: size.width,
    height: size.height,
  }
}

function createCanvasRect(left: number, top: number, width: number, height: number): CanvasRect {
  const right = left + width
  const bottom = top + height
  return {
    left,
    top,
    right,
    bottom,
    width,
    height,
  }
}

function expandRect(rect: CanvasRect, padding: number): CanvasRect {
  const left = rect.left - padding
  const top = rect.top - padding
  const right = rect.right + padding
  const bottom = rect.bottom + padding
  return {
    left,
    top,
    right,
    bottom,
    width: right - left,
    height: bottom - top,
  }
}

function mergeRects(left: CanvasRect, right: CanvasRect): CanvasRect {
  const nextLeft = Math.min(left.left, right.left)
  const nextTop = Math.min(left.top, right.top)
  const nextRight = Math.max(left.right, right.right)
  const nextBottom = Math.max(left.bottom, right.bottom)
  return {
    left: nextLeft,
    top: nextTop,
    right: nextRight,
    bottom: nextBottom,
    width: nextRight - nextLeft,
    height: nextBottom - nextTop,
  }
}

function pointInRect(point: CanvasPoint, rect: CanvasRect): boolean {
  return (
    point.x >= rect.left && point.x <= rect.right && point.y >= rect.top && point.y <= rect.bottom
  )
}

export function rectContainsRect(container: CanvasRect, subject: CanvasRect): boolean {
  return (
    subject.left >= container.left &&
    subject.right <= container.right &&
    subject.top >= container.top &&
    subject.bottom <= container.bottom
  )
}

export function rectsIntersect(left: CanvasRect, right: CanvasRect): boolean {
  return (
    left.left < right.right &&
    left.right > right.left &&
    left.top < right.bottom &&
    left.bottom > right.top
  )
}

export function moveCanvasRect(rect: CanvasRect, delta: CanvasPoint): CanvasRect {
  return createCanvasRect(rect.left + delta.x, rect.top + delta.y, rect.width, rect.height)
}

export function canvasRectToGroupGeometry(rect: CanvasRect): CanvasGraphGroupGeometry {
  return {
    x: rect.left,
    y: rect.top,
    width: rect.width,
    height: rect.height,
  }
}

function canvasGroupToRect(group: Pick<CanvasGroup, 'x' | 'y' | 'width' | 'height'>) {
  return createCanvasRect(group.x, group.y, group.width, group.height)
}

function resolveCanvasGroupBoundsFromNodes(options: {
  nodes: readonly CanvasFlowNodeLike[]
  nodeIds: readonly CanvasGraphNodeId[]
  padding?: number
}): CanvasRect | null {
  const nodesById = new Map(options.nodes.map((node) => [node.id, node]))
  let bounds: CanvasRect | null = null
  for (const nodeId of options.nodeIds) {
    const node = nodesById.get(nodeId)
    if (!node) return null
    const nodeBounds = readFlowNodeBounds(node)
    bounds = bounds ? mergeRects(bounds, nodeBounds) : nodeBounds
  }
  return bounds ? expandRect(bounds, options.padding ?? CANVAS_GROUP_FRAME_PADDING_PX) : null
}

export function resolveCanvasSelectionGroupActionBounds(options: {
  action: CanvasSelectionGroupGeometryAction | null
  nodes: readonly CanvasFlowNodeLike[]
  frames: readonly CanvasGroupFrame[]
  requireSelectedGroupFrame?: boolean
}): CanvasRect | null {
  const { action } = options
  if (!action) return null

  if (action.kind === 'ungroup') {
    const frame = options.frames.find(
      (item) =>
        item.id === action.groupId && (options.requireSelectedGroupFrame !== true || item.selected),
    )
    if (frame) return frame.bounds
  }

  return resolveCanvasGroupBoundsFromNodes({
    nodes: options.nodes,
    nodeIds: action.nodeIds,
    padding: 0,
  })
}

function flowPointToViewportPoint(
  point: CanvasPoint,
  viewport: CanvasFlowViewportTransform,
): CanvasPoint {
  const zoom = Number.isFinite(viewport.zoom) && viewport.zoom > 0 ? viewport.zoom : 1
  return {
    x: viewport.x + point.x * zoom,
    y: viewport.y + point.y * zoom,
  }
}

export function resolveCanvasSelectionGroupActionButtonPosition(options: {
  action: CanvasSelectionGroupGeometryAction | null
  nodes: readonly CanvasFlowNodeLike[]
  frames: readonly CanvasGroupFrame[]
  viewport: CanvasFlowViewportTransform
  gapPx?: number
}): CanvasPoint | null {
  const bounds = resolveCanvasSelectionGroupActionBounds({
    action: options.action,
    nodes: options.nodes,
    frames: options.frames,
  })
  if (!bounds) return null

  const anchor = flowPointToViewportPoint(
    {
      x: bounds.left + bounds.width / 2,
      y: bounds.top,
    },
    options.viewport,
  )
  return {
    x: anchor.x,
    y: anchor.y - (options.gapPx ?? 0),
  }
}

export function resizeCanvasRectFromHandle(options: {
  bounds: CanvasRect
  handle: CanvasGroupResizeHandle
  delta: CanvasPoint
  minWidth?: number
  minHeight?: number
}): CanvasRect {
  const minWidth = options.minWidth ?? CANVAS_GROUP_FRAME_MIN_WIDTH_PX
  const minHeight = options.minHeight ?? CANVAS_GROUP_FRAME_MIN_HEIGHT_PX
  let left = options.bounds.left
  let top = options.bounds.top
  let right = options.bounds.right
  let bottom = options.bounds.bottom

  if (options.handle.includes('w')) left += options.delta.x
  if (options.handle.includes('e')) right += options.delta.x
  if (options.handle.includes('n')) top += options.delta.y
  if (options.handle.includes('s')) bottom += options.delta.y

  if (right - left < minWidth) {
    if (options.handle.includes('w')) left = right - minWidth
    else right = left + minWidth
  }
  if (bottom - top < minHeight) {
    if (options.handle.includes('n')) top = bottom - minHeight
    else bottom = top + minHeight
  }

  return createCanvasRect(left, top, right - left, bottom - top)
}

export function resolveCanvasGroupFrames(options: {
  groups: readonly CanvasGroup[]
  selectedGroupId?: CanvasGraphGroupId | null
  minimumNodeCount?: number
}): CanvasGroupFrame[] {
  const minimumNodeCount = options.minimumNodeCount ?? 0

  return options.groups.flatMap((group, order): CanvasGroupFrame[] => {
    const nodeIds = group.nodeIds.filter(isCanvasGraphNodeId)
    if (nodeIds.length < minimumNodeCount) return []
    return [
      {
        id: group.id as CanvasGraphGroupId,
        nodeIds,
        bounds: canvasGroupToRect(group),
        selected: group.id === options.selectedGroupId,
        order,
      },
    ]
  })
}

export function findTopmostCanvasGroupFrameAtPoint(
  frames: readonly CanvasGroupFrame[],
  point: CanvasPoint,
): CanvasGroupFrame | null {
  for (let index = frames.length - 1; index >= 0; index -= 1) {
    const frame = frames[index]
    if (!frame) continue
    if (pointInRect(point, frame.bounds)) return frame
  }
  return null
}

function resolveCanvasGroupNodeZIndexes(groups: readonly CanvasGroup[]): Map<string, number> {
  const zIndexes = new Map<string, number>()
  groups.forEach((group) => {
    group.nodeIds.forEach((nodeId) => {
      zIndexes.set(nodeId, CANVAS_GROUP_NODE_Z_INDEX_GROUPED)
    })
  })
  return zIndexes
}

export function projectCanvasGroupNodeLayers(
  nodes: readonly CanvasFlowNodeLike[],
  groups: readonly CanvasGroup[],
) {
  const zIndexes = resolveCanvasGroupNodeZIndexes(groups)
  let nextNodes: CanvasFlowNodeLike[] | null = null

  nodes.forEach((node, index) => {
    const nextZIndex = zIndexes.get(node.id) ?? CANVAS_GROUP_NODE_Z_INDEX_BASE
    if ((node.zIndex ?? 0) === nextZIndex) return
    if (!nextNodes) nextNodes = nodes.slice() as CanvasFlowNodeLike[]
    nextNodes[index] = { ...node, zIndex: nextZIndex }
  })

  return nextNodes ?? nodes
}
