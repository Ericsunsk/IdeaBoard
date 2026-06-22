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
  const normalized = normalizeTaskImageResult(result)
  if (!normalized) return fallbackRatio > 0 ? fallbackRatio : 1
  if (normalized.width && normalized.height) return normalized.height / normalized.width
  return resolveTaskImageRatio(normalized.url, fallbackRatio, loadImageSize)
}
