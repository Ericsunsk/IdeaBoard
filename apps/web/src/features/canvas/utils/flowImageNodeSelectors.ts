import {
  resolveImageNodeDisplayRatio,
  resolveImageNodeLatestOutput,
  type ImageNodeItem,
} from '../../canvas-board/domain'
import { buildMediaBrowserURL, type MediaRef } from '../../../shared/media/MediaRef'

export type ImageNodeDisplayPayload = {
  url: string
  ratio: number
  mediaRef?: MediaRef
  /** Internal snapshot migration field; public API uses MediaRef.assetId. */
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
  imageNode: Pick<ImageNodeItem, 'sourceImageUrl' | 'sourceMediaRef' | 'outputImages'>,
): string {
  const latestOutput = resolveImageNodeLatestOutput(imageNode)
  if (latestOutput?.mediaRef) return buildMediaBrowserURL(latestOutput.mediaRef)
  if (imageNode.sourceMediaRef) return buildMediaBrowserURL(imageNode.sourceMediaRef)
  return `${latestOutput?.url || imageNode.sourceImageUrl || ''}`.trim()
}

function resolveImageNodeMediaIdentity(
  imageNode: Pick<ImageNodeItem, 'sourceMediaRef' | 'sourceMediaAssetId' | 'outputImages'>,
): { mediaRef?: MediaRef; mediaAssetId?: number } {
  const latestOutput = resolveImageNodeLatestOutput(imageNode)
  const mediaRef = latestOutput ? latestOutput.mediaRef : imageNode.sourceMediaRef
  if (mediaRef) return { mediaRef }

  const legacyID = Number(latestOutput ? latestOutput.mediaAssetId : imageNode.sourceMediaAssetId)
  return Number.isInteger(legacyID) && legacyID > 0 ? { mediaAssetId: legacyID } : {}
}

export function resolveImageNodeDisplayPayload(
  imageNode: Pick<
    ImageNodeItem,
    'sourceImageUrl' | 'sourceMediaRef' | 'sourceMediaAssetId' | 'sourceImageRatio' | 'outputImages'
  >,
): ImageNodeDisplayPayload | null {
  const url = resolveImageNodeDisplayUrl(imageNode)
  if (!url) return null
  return {
    url,
    ratio: resolveImageNodeDisplayRatio(imageNode),
    ...resolveImageNodeMediaIdentity(imageNode),
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
    ...resolveImageNodeMediaIdentity(imageNode),
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
