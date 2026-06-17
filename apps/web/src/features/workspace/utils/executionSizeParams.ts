export interface ExecutionImageSize {
  width: number
  height: number
}

function normalizeExecutionImageSize(
  size: ExecutionImageSize | null | undefined,
): ExecutionImageSize | null {
  if (!size) return null
  const width = Math.max(1, Math.round(size.width))
  const height = Math.max(1, Math.round(size.height))
  if (!Number.isFinite(width) || !Number.isFinite(height)) return null
  return { width, height }
}

export function buildExecutionSizeParams(
  sourceSize: ExecutionImageSize | null | undefined,
  targetSize?: ExecutionImageSize | null,
): Record<string, number> {
  const normalizedSource = normalizeExecutionImageSize(sourceSize)
  const normalizedTarget = normalizeExecutionImageSize(targetSize ?? sourceSize)
  if (!normalizedSource || !normalizedTarget) return {}
  return {
    custom_width: normalizedTarget.width,
    custom_height: normalizedTarget.height,
    original_image_width: normalizedSource.width,
    original_image_height: normalizedSource.height,
  }
}

export function extendExecutionImageSize(
  size: ExecutionImageSize | null | undefined,
  extend: { top: number; right: number; bottom: number; left: number },
): ExecutionImageSize | null {
  const normalized = normalizeExecutionImageSize(size)
  if (!normalized) return null
  const widthScale = 1 + ((extend.left || 0) + (extend.right || 0)) / 100
  const heightScale = 1 + ((extend.top || 0) + (extend.bottom || 0)) / 100
  return {
    width: Math.max(1, Math.round(normalized.width * widthScale)),
    height: Math.max(1, Math.round(normalized.height * heightScale)),
  }
}
