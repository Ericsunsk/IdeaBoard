import {
  resolveImageNodeDisplayRatio,
  resolveImageNodeLatestOutput,
  type ImageNodeItem,
} from '../../../board'
import { buildMediaBrowserURL, type MediaRef } from '../../../../shared/media/mediaRef'

export type ImageNodeDisplayPayload = {
  url: string
  ratio: number
  mediaRef?: MediaRef | undefined
  /** String delivery identity resolved from the node's MediaRef. */
  assetId?: string | undefined
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

export function resolveImageNodeDisplayPayload(
  imageNode: Pick<
    ImageNodeItem,
    'sourceImageUrl' | 'sourceMediaRef' | 'sourceImageRatio' | 'outputImages'
  >,
): ImageNodeDisplayPayload | null {
  const url = resolveImageNodeDisplayUrl(imageNode)
  if (!url) return null
  const latestOutput = resolveImageNodeLatestOutput(imageNode)
  const mediaRef = latestOutput?.mediaRef ?? imageNode.sourceMediaRef
  const assetId = mediaRef?.assetId?.trim()
  return {
    url,
    ratio: resolveImageNodeDisplayRatio(imageNode),
    ...(mediaRef ? { mediaRef } : {}),
    ...(assetId ? { assetId } : {}),
  }
}

function resolveAnchorWidth(imageNode: ImageNodeItem, fallbackWidth: number | undefined): number {
  const width = Number(imageNode.width)
  if (Number.isFinite(width) && width > 0) return width
  const fallback = Number(fallbackWidth)
  if (Number.isFinite(fallback) && fallback > 0) return fallback
  return DEFAULT_IMAGE_NODE_LAYOUT_WIDTH
}

export function resolveImageNodeLayoutAnchor(
  node: ImageNodePositionSource,
  imageNode: ImageNodeItem,
  options: ResolveImageNodeLayoutAnchorOptions = {},
): ImageNodeLayoutAnchor | null {
  const url = resolveImageNodeDisplayUrl(imageNode)
  if (options.requireUrl && !url) return null
  const payload = resolveImageNodeDisplayPayload(imageNode)

  return {
    url,
    ratio: resolveImageNodeDisplayRatio(imageNode),
    ...(payload?.mediaRef ? { mediaRef: payload.mediaRef } : {}),
    ...(payload?.assetId ? { assetId: payload.assetId } : {}),
    x: Number(node.position?.x ?? imageNode.x ?? 0),
    y: Number(node.position?.y ?? imageNode.y ?? 0),
    width: resolveAnchorWidth(imageNode, options.fallbackWidth),
  }
}
