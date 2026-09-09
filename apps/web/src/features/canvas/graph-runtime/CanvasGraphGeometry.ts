import {
  resolveImageNodeStageHeight,
  resolveImageNodeDisplayRatio,
  type CanvasNodeItem,
  type ImageNodeItem,
} from '../../canvas-board/domain'
import type { CanvasGraphNodeId, CanvasGraphNodeKind, CanvasGraphStore } from './canvasGraphStore'

const CANVAS_NODE_HEADER_HEIGHT = 22

export type CanvasGraphNodeDimensions = {
  width: number
  height: number
}

export type CanvasGraphBounds = {
  left: number
  top: number
  right: number
  bottom: number
  width: number
  height: number
}

function hasDisplayableImageSource(node: ImageNodeItem): boolean {
  if (node.sourceMediaRef) return true
  if (node.sourceImageUrl && node.sourceImageUrl.trim()) return true
  return node.outputImages.some(
    (output) => output.mediaRef !== undefined || (output.url && output.url.trim().length > 0),
  )
}

export function resolveCanvasGraphNodeDimensions(
  kind: CanvasGraphNodeKind,
  node: CanvasNodeItem,
): CanvasGraphNodeDimensions {
  const width = Math.max(1, Math.round(node.width))
  const bodyHeight =
    kind === 'image'
      ? resolveImageNodeStageHeight(
          width,
          resolveImageNodeDisplayRatio(node as ImageNodeItem),
          hasDisplayableImageSource(node as ImageNodeItem),
        )
      : width
  return {
    width,
    height: Math.max(1, Math.round(CANVAS_NODE_HEADER_HEIGHT + bodyHeight)),
  }
}

export function readCanvasGraphNodeBounds(
  store: CanvasGraphStore,
  nodeId: CanvasGraphNodeId,
): CanvasGraphBounds | null {
  const node = store.getNode(nodeId)
  if (!node) return null
  const dimensions = resolveCanvasGraphNodeDimensions(node.kind, node.snapshot)
  return {
    left: node.snapshot.x,
    top: node.snapshot.y,
    right: node.snapshot.x + dimensions.width,
    bottom: node.snapshot.y + dimensions.height,
    ...dimensions,
  }
}

export function resolveCanvasGraphNodesBounds(
  store: CanvasGraphStore,
  nodeIds: readonly CanvasGraphNodeId[],
  padding = 0,
): CanvasGraphBounds | null {
  let bounds: CanvasGraphBounds | null = null
  for (const nodeId of nodeIds) {
    const nodeBounds = readCanvasGraphNodeBounds(store, nodeId)
    if (!nodeBounds) return null
    if (!bounds) {
      bounds = { ...nodeBounds }
      continue
    }
    const left = Math.min(bounds.left, nodeBounds.left)
    const top = Math.min(bounds.top, nodeBounds.top)
    const right = Math.max(bounds.right, nodeBounds.right)
    const bottom = Math.max(bounds.bottom, nodeBounds.bottom)
    bounds = {
      left,
      top,
      right,
      bottom,
      width: right - left,
      height: bottom - top,
    }
  }
  if (!bounds) return null
  return {
    left: bounds.left - padding,
    top: bounds.top - padding,
    right: bounds.right + padding,
    bottom: bounds.bottom + padding,
    width: bounds.width + padding * 2,
    height: bounds.height + padding * 2,
  }
}
