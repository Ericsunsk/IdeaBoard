export const IMAGE_NODE_EMPTY_SIZE = 280
const IMAGE_NODE_TARGET_AREA = IMAGE_NODE_EMPTY_SIZE * IMAGE_NODE_EMPTY_SIZE
const IMAGE_NODE_MAX_SIDE = 360

export type ImageNodeFrameSize = {
  width: number
  height: number
}

export function normalizeImageNodeRatio(value: unknown): number {
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
  let width = Math.sqrt(IMAGE_NODE_TARGET_AREA / ratio)
  let height = width * ratio
  const scale = Math.min(1, IMAGE_NODE_MAX_SIDE / width, IMAGE_NODE_MAX_SIDE / height)
  width *= scale
  height *= scale

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
