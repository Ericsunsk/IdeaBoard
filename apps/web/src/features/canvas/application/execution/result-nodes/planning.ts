import type { CanvasGraphNodeRecord } from '../../../domain/graph'
import type { CanvasRendererMetricsPort } from '../../../infrastructure/vue-flow-adapter'
import { resolveImageNodeStageHeight, type ImageNodeItem } from '../../../../board/ports/canvas'
import type { ImageResultAnchor } from '../planning/imageExecutionResultTypes'
import { resolveImageNodeLayoutAnchor } from '../../../infrastructure/vue-flow-adapter/flowImageNodeSelectors.ts'

export type ImageResultLayout = ImageResultAnchor & { ratio: number }
export type PendingImageResultLookup = {
  nodeById: Map<string, CanvasGraphNodeRecord>
  imageChildIdsBySource: Map<string, string[]>
}
export type PendingImageResultPlacementOptions = {
  anchor?: ImageResultAnchor | undefined
  targetRatio?: number | undefined
  targetWidth?: number | undefined
}

const IMAGE_RESULT_NODE_X_GAP = 100
const IMAGE_RESULT_NODE_Y_GAP = 14
const IMAGE_NODE_HEADER_HEIGHT = 26

function estimateImageNodeHeight(source: Pick<ImageResultLayout, 'width' | 'ratio'>): number {
  return IMAGE_NODE_HEADER_HEIGHT + resolveImageNodeStageHeight(source.width, source.ratio, true)
}

function readImageNodeLayout(
  node: CanvasGraphNodeRecord,
  rendererMetrics: CanvasRendererMetricsPort,
): ImageResultLayout | null {
  if (node.kind !== 'image') return null
  const snapshot = node.snapshot as ImageNodeItem
  const metrics = rendererMetrics.readNodeMetrics(node.id)
  const anchor = resolveImageNodeLayoutAnchor(
    { position: metrics?.position ?? { x: snapshot.x, y: snapshot.y } },
    snapshot,
  )
  if (!anchor) return null
  return {
    x: anchor.x,
    y: anchor.y,
    width: metrics?.measured ? metrics.dimensions.width : anchor.width,
    ratio: anchor.ratio,
  }
}

export function readOutputAnchorLayout(
  nodeId: string,
  fallback: ImageResultLayout,
  lookup: PendingImageResultLookup,
  rendererMetrics: CanvasRendererMetricsPort,
  anchorHint?: ImageResultAnchor,
): ImageResultLayout {
  const node = lookup.nodeById.get(nodeId)
  if (node) {
    if (node.kind === 'image') return readImageNodeLayout(node, rendererMetrics) ?? fallback
    const snapshot = node.snapshot as { width?: unknown }
    const metrics = rendererMetrics.readNodeMetrics(nodeId)
    const width = metrics?.measured ? metrics.dimensions.width : Number(snapshot.width)
    return {
      ratio: 1,
      x: metrics?.position.x ?? Number(node.snapshot.x ?? 0),
      y: metrics?.position.y ?? Number(node.snapshot.y ?? 0),
      width: Number.isFinite(width) && width > 0 ? width : fallback.width,
    }
  }
  return anchorHint ? { ...anchorHint, ratio: fallback.ratio } : fallback
}

function readImageLayoutBounds(
  imageLayout: Pick<ImageResultLayout, 'x' | 'y' | 'width' | 'ratio'>,
  rendererMetrics: CanvasRendererMetricsPort,
  nodeId: string,
): { x: number; y: number; width: number; height: number } {
  const metrics = rendererMetrics.readNodeMetrics(nodeId)
  const height = metrics?.measured
    ? metrics.dimensions.height
    : estimateImageNodeHeight(imageLayout)
  return { x: imageLayout.x, y: imageLayout.y, width: imageLayout.width, height }
}

function readGenericNodeBounds(
  snapshot: { width?: unknown; height?: unknown },
  rendererMetrics: CanvasRendererMetricsPort,
  nodeId: string,
  fallbackX: number,
  fallbackY: number,
): { x: number; y: number; width: number; height: number } | null {
  const metrics = rendererMetrics.readNodeMetrics(nodeId)
  const width = metrics?.measured ? metrics.dimensions.width : Number(snapshot.width)
  if (!Number.isFinite(width) || width <= 0) return null
  const height = metrics?.measured ? metrics.dimensions.height : Number(snapshot.height)
  return {
    x: metrics?.position.x ?? fallbackX,
    y: metrics?.position.y ?? fallbackY,
    width,
    height: Number.isFinite(height) && height > 0 ? height : width,
  }
}

function readNodeBounds(
  node: CanvasGraphNodeRecord,
  rendererMetrics: CanvasRendererMetricsPort,
): { x: number; y: number; width: number; height: number } | null {
  const imageLayout = readImageNodeLayout(node, rendererMetrics)
  if (imageLayout) return readImageLayoutBounds(imageLayout, rendererMetrics, node.id)
  const snapshot = node.snapshot as { width?: unknown; height?: unknown }
  return readGenericNodeBounds(
    snapshot,
    rendererMetrics,
    node.id,
    Number(node.snapshot.x ?? 0),
    Number(node.snapshot.y ?? 0),
  )
}

export function resolvePendingImageResultPosition(
  anchor: ImageResultLayout,
  lookup: PendingImageResultLookup,
  rendererMetrics: CanvasRendererMetricsPort,
  target: Pick<ImageResultLayout, 'width' | 'ratio'>,
) {
  const preferredPosition = { x: anchor.x + anchor.width + IMAGE_RESULT_NODE_X_GAP, y: anchor.y }
  const occupiedBounds = Array.from(lookup.nodeById.values())
    .map((node) => readNodeBounds(node, rendererMetrics))
    .filter((bounds): bounds is NonNullable<typeof bounds> => !!bounds)
  const targetHeight = estimateImageNodeHeight(target)
  const isCollisionFree = (position: { x: number; y: number }) =>
    occupiedBounds.every(
      (bounds) =>
        position.x + target.width <= bounds.x ||
        position.x >= bounds.x + bounds.width ||
        position.y + targetHeight <= bounds.y ||
        position.y >= bounds.y + bounds.height,
    )
  if (isCollisionFree(preferredPosition)) return preferredPosition
  let preferredColumnBottom = preferredPosition.y
  occupiedBounds.forEach((bounds) => {
    const overlaps =
      preferredPosition.x < bounds.x + bounds.width && preferredPosition.x + target.width > bounds.x
    if (overlaps && bounds.y + bounds.height > preferredPosition.y) {
      preferredColumnBottom = Math.max(
        preferredColumnBottom,
        bounds.y + bounds.height + IMAGE_RESULT_NODE_Y_GAP,
      )
    }
  })
  const candidate = { x: preferredPosition.x, y: preferredColumnBottom }
  if (isCollisionFree(candidate)) return candidate
  const escapePosition = {
    x: preferredPosition.x,
    y:
      occupiedBounds.reduce(
        (bottom, bounds) => Math.max(bottom, bounds.y + bounds.height),
        preferredPosition.y,
      ) + IMAGE_RESULT_NODE_Y_GAP,
  }
  if (isCollisionFree(escapePosition)) return escapePosition
  throw new Error('Unable to find a collision-free position for derived image node')
}
