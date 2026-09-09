export const IMAGE_NODE_EMPTY_SIZE = 280
const IMAGE_NODE_TARGET_SHORT_SIDE = 260
const IMAGE_NODE_MAX_LONG_SIDE = 640
const IMAGE_NODE_MIN_SHORT_SIDE = 120

export type ImageNodeFrameSize = {
  width: number
  height: number
}

export function resolveImageNodeLatestOutput(
  imageNode: Pick<ImageNodeItem, 'outputImages'>,
): ImageNodeOutputItem | null {
  const outputs = Array.isArray(imageNode.outputImages) ? imageNode.outputImages : []
  return outputs.length > 0 ? (outputs[outputs.length - 1] ?? null) : null
}

export function resolveImageNodeDisplayRatio(
  imageNode: Pick<ImageNodeItem, 'sourceImageRatio' | 'outputImages'>,
): number {
  const latestOutput = resolveImageNodeLatestOutput(imageNode)
  const outputRatio = Number(latestOutput?.ratio)
  if (Number.isFinite(outputRatio) && outputRatio > 0) return outputRatio
  return normalizeImageNodeRatio(imageNode.sourceImageRatio)
}

function normalizeImageNodeRatio(value: unknown): number {
  const ratio = Number(value)
  return Number.isFinite(ratio) && ratio > 0 ? ratio : 1
}

export function resolveImageNodeFrameSize(
  ratioValue: unknown,
  hasImage: boolean,
): ImageNodeFrameSize {
  if (!hasImage) {
    return { width: IMAGE_NODE_EMPTY_SIZE, height: IMAGE_NODE_EMPTY_SIZE }
  }

  const ratio = normalizeImageNodeRatio(ratioValue)
  const isTall = ratio >= 1
  let width = isTall ? IMAGE_NODE_TARGET_SHORT_SIDE : IMAGE_NODE_TARGET_SHORT_SIDE / ratio
  let height = isTall ? IMAGE_NODE_TARGET_SHORT_SIDE * ratio : IMAGE_NODE_TARGET_SHORT_SIDE
  const longSide = Math.max(width, height)

  if (longSide > IMAGE_NODE_MAX_LONG_SIDE) {
    const scale = IMAGE_NODE_MAX_LONG_SIDE / longSide
    width *= scale
    height *= scale
  }

  const shortSide = Math.min(width, height)
  if (shortSide > 0 && shortSide < IMAGE_NODE_MIN_SHORT_SIDE) {
    const scale = IMAGE_NODE_MIN_SHORT_SIDE / shortSide
    width *= scale
    height *= scale
  }

  return {
    width: Math.max(1, Math.round(width)),
    height: Math.max(1, Math.round(height)),
  }
}

export function resolveImageNodeStageHeight(
  widthValue: unknown,
  ratioValue: unknown,
  hasImage: boolean,
): number {
  if (!hasImage) return IMAGE_NODE_EMPTY_SIZE
  const width = Number(widthValue)
  const safeWidth = Number.isFinite(width) && width > 0 ? width : IMAGE_NODE_EMPTY_SIZE
  return Math.max(1, Math.round(safeWidth * normalizeImageNodeRatio(ratioValue)))
}
import type { ImageNodeItem, ImageNodeOutputItem } from './nodes'
