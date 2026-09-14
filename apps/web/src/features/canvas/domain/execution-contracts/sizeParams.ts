import type { SubmitTaskParams } from '../../../../shared/api/task'

export interface ExecutionImageSize {
  width: number
  height: number
}

type ExecutionSizeTaskParams = Pick<
  SubmitTaskParams,
  'custom_width' | 'custom_height' | 'original_image_width' | 'original_image_height'
>

export function normalizeExecutionImageSize(
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
): ExecutionSizeTaskParams {
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
