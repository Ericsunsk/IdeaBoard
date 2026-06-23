import {
  normalizeTaskImageResult,
  normalizeTaskImageResults,
  type TaskImageResult,
} from '../../../shared/query/taskResult'

type ImageSize = {
  width: number
  height: number
}

type ImageSizeLoader = (url: string) => Promise<ImageSize | null>

export type ResolvedTaskImageResult = {
  url: string
  ratio: number
  width?: number
  height?: number
  mimeType?: string
}

function normalizePositiveImageSize(width: unknown, height: unknown): ImageSize | null {
  const widthNumber = Number(width)
  const heightNumber = Number(height)
  if (
    !Number.isFinite(widthNumber) ||
    widthNumber <= 0 ||
    !Number.isFinite(heightNumber) ||
    heightNumber <= 0
  ) {
    return null
  }
  const normalizedWidth = Math.max(1, Math.round(widthNumber))
  const normalizedHeight = Math.max(1, Math.round(heightNumber))
  return { width: normalizedWidth, height: normalizedHeight }
}

export function pickFirstValidImageResult(results: TaskImageResult[]): TaskImageResult | null {
  return normalizeTaskImageResults(results)[0] ?? null
}

export async function resolveTaskImageRatio(
  url: string,
  fallbackRatio: number,
  loadImageSize: ImageSizeLoader,
): Promise<number> {
  try {
    const size = await loadImageSize(url)
    if (size && size.width > 0 && size.height > 0) {
      return size.height / size.width
    }
  } catch (error) {
    console.warn('[workspace] failed to probe task image size', error)
  }
  return fallbackRatio > 0 ? fallbackRatio : 1
}

export async function resolveTaskImageResultRatio(
  result: TaskImageResult,
  fallbackRatio: number,
  loadImageSize: ImageSizeLoader,
): Promise<number> {
  const resolved = await resolveTaskImageResultDisplay(result, fallbackRatio, loadImageSize)
  return resolved?.ratio ?? (fallbackRatio > 0 ? fallbackRatio : 1)
}

export async function resolveTaskImageResultDisplay(
  result: TaskImageResult,
  fallbackRatio: number,
  loadImageSize: ImageSizeLoader,
): Promise<ResolvedTaskImageResult | null> {
  const normalized = normalizeTaskImageResult(result)
  if (!normalized) return null

  let size = normalizePositiveImageSize(normalized.width, normalized.height)
  if (!size) {
    try {
      size = await loadImageSize(normalized.url)
    } catch (error) {
      console.warn('[workspace] failed to probe task image size', error)
    }
  }

  const ratio = size ? size.height / size.width : fallbackRatio > 0 ? fallbackRatio : 1
  return {
    url: normalized.url,
    ratio,
    ...(size ? { width: size.width, height: size.height } : {}),
    ...(normalized.mimeType ? { mimeType: normalized.mimeType } : {}),
  }
}
