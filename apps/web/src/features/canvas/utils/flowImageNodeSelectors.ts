import type { ImageNodeItem, ImageNodeOutputItem } from '../../canvas-board/domain'

export type ImageNodeDisplayPayload = {
  url: string
  ratio: number
}

export type ImageNodeLayoutAnchor = ImageNodeDisplayPayload & {
  x: number
  y: number
  width: number
}

type ImageNodePositionSource = {
  position?: {
    x?: number
    y?: number
  }
}

type ResolveImageNodeLayoutAnchorOptions = {
  fallbackWidth?: number
  requireUrl?: boolean
}

export const DEFAULT_IMAGE_NODE_LAYOUT_WIDTH = 280

export function resolveImageNodeLatestOutput(
  imageNode: Pick<ImageNodeItem, 'outputImages'>,
): ImageNodeOutputItem | null {
  const outputs = Array.isArray(imageNode.outputImages) ? imageNode.outputImages : []
  return outputs.length > 0 ? outputs[outputs.length - 1] : null
}

export function resolveImageNodeDisplayUrl(
  imageNode: Pick<ImageNodeItem, 'sourceImageUrl' | 'outputImages'>,
): string {
  const latestOutput = resolveImageNodeLatestOutput(imageNode)
  return `${latestOutput?.url || imageNode.sourceImageUrl || ''}`.trim()
}

export function resolveImageNodeDisplayRatio(
  imageNode: Pick<ImageNodeItem, 'sourceImageRatio' | 'outputImages'>,
): number {
  const latestOutput = resolveImageNodeLatestOutput(imageNode)
  const outputRatio = Number(latestOutput?.ratio)
  if (Number.isFinite(outputRatio) && outputRatio > 0) return outputRatio

  const sourceRatio = Number(imageNode.sourceImageRatio)
  return Number.isFinite(sourceRatio) && sourceRatio > 0 ? sourceRatio : 1
}

export function resolveImageNodeDisplayPayload(
  imageNode: Pick<ImageNodeItem, 'sourceImageUrl' | 'sourceImageRatio' | 'outputImages'>,
): ImageNodeDisplayPayload | null {
  const url = resolveImageNodeDisplayUrl(imageNode)
  if (!url) return null
  return {
    url,
    ratio: resolveImageNodeDisplayRatio(imageNode),
  }
}

export function resolveImageNodeLayoutAnchor(
  node: ImageNodePositionSource,
  imageNode: ImageNodeItem,
  options: ResolveImageNodeLayoutAnchorOptions = {},
): ImageNodeLayoutAnchor | null {
  const url = resolveImageNodeDisplayUrl(imageNode)
  if (options.requireUrl && !url) return null

  const width = Number(imageNode.width)
  const fallbackWidth = Number(options.fallbackWidth)
  return {
    url,
    ratio: resolveImageNodeDisplayRatio(imageNode),
    x: Number(node.position?.x ?? imageNode.x ?? 0),
    y: Number(node.position?.y ?? imageNode.y ?? 0),
    width:
      Number.isFinite(width) && width > 0
        ? width
        : Number.isFinite(fallbackWidth) && fallbackWidth > 0
          ? fallbackWidth
          : DEFAULT_IMAGE_NODE_LAYOUT_WIDTH,
  }
}
