import { extractImageUrlsFromTaskResult } from '../../../shared/query/taskResult'

type TaskResultLike = {
  outputs?: Array<{ role?: string; url?: string }>
  result?: string | null
}

type ImageSize = {
  width: number
  height: number
}

type ImageSizeLoader = (url: string) => Promise<ImageSize | null>

export function extractTaskImageUrls(task: TaskResultLike): string[] {
  const outputUrls = Array.isArray(task.outputs)
    ? task.outputs
        .filter((output) => output?.role === 'image' && typeof output.url === 'string')
        .map((output) => output.url?.trim() ?? '')
        .filter((url) => url.length > 0)
    : []
  if (outputUrls.length > 0) return outputUrls
  return extractImageUrlsFromTaskResult(task.result)
}

export function pickFirstValidUrl(urls: string[]): string {
  return urls.find((url) => typeof url === 'string' && url.trim())?.trim() ?? ''
}

export function normalizeTaskImageUrls(urls: string[]): string[] {
  return urls
    .map((url) => (typeof url === 'string' ? url.trim() : ''))
    .filter((url) => url.length > 0)
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
