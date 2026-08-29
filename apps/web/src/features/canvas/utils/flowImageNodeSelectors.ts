import {
  resolveImageNodeDisplayRatio,
  resolveImageNodeLatestOutput,
  type ImageNodeItem,
} from '../../canvas-board/domain'

export type ImageNodeDisplayPayload = {
  url: string
  ratio: number
  mediaAssetId?: number
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

export function resolveImageNodeDisplayUrl(
  imageNode: Pick<ImageNodeItem, 'sourceImageUrl' | 'outputImages'>,
): string {
  const latestOutput = resolveImageNodeLatestOutput(imageNode)
  return `${latestOutput?.url || imageNode.sourceImageUrl || ''}`.trim()
}

export function resolveImageNodeDisplayPayload(
  imageNode: Pick<
    ImageNodeItem,
    'sourceImageUrl' | 'sourceMediaAssetId' | 'sourceImageRatio' | 'outputImages'
  >,
): ImageNodeDisplayPayload | null {
  const url = resolveImageNodeDisplayUrl(imageNode)
  if (!url) return null
  const latestOutput = resolveImageNodeLatestOutput(imageNode)
  const mediaAssetId = Number(
    latestOutput ? latestOutput.mediaAssetId : imageNode.sourceMediaAssetId,
  )
  return {
    url,
    ratio: resolveImageNodeDisplayRatio(imageNode),
    ...(Number.isInteger(mediaAssetId) && mediaAssetId > 0 ? { mediaAssetId } : {}),
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
  const latestOutput = resolveImageNodeLatestOutput(imageNode)
  const mediaAssetId = Number(latestOutput ? latestOutput.mediaAssetId : imageNode.sourceMediaAssetId)
  return {
    url,
    ratio: resolveImageNodeDisplayRatio(imageNode),
    ...(Number.isInteger(mediaAssetId) && mediaAssetId > 0 ? { mediaAssetId } : {}),
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
