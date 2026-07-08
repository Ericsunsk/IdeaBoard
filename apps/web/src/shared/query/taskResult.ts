import { resolveAssetUrl } from '../config/runtimeUrls'

export type TaskImageResult = {
  url: string
  width?: number
  height?: number
  mimeType?: string
  mediaAssetId?: number
}

type TaskImageResultInput = {
  url?: unknown
  width?: unknown
  height?: unknown
  mimeType?: unknown
  mediaAssetId?: unknown
}

type TaskResultPayloadLike = {
  outputs?: Array<{
    role?: string
    url?: string
    width?: unknown
    height?: unknown
    mime_type?: unknown
    media_asset_id?: unknown
  }>
  result?: string | null
}

function normalizeTaskImageUrl(url: string): string {
  return resolveAssetUrl(url.trim())
}

function toPositiveNumber(value: unknown): number | undefined {
  const num = Number(value)
  return Number.isFinite(num) && num > 0 ? num : undefined
}

function toPositiveInteger(value: unknown): number | undefined {
  const num = Number(value)
  return Number.isInteger(num) && num > 0 ? num : undefined
}

function extractImageResultsFromTaskResult(result: string | null | undefined): TaskImageResult[] {
  if (!result) return []
  try {
    const parsed = JSON.parse(result)
    if (Array.isArray(parsed?.image_urls)) {
      return parsed.image_urls
        .filter((url: unknown): url is string => typeof url === 'string' && url.trim().length > 0)
        .map((url: string) => ({ url: normalizeTaskImageUrl(url) }))
    }
    if (typeof parsed?.image_url === 'string' && parsed.image_url.trim())
      return [{ url: normalizeTaskImageUrl(parsed.image_url) }]
    return []
  } catch {
    return []
  }
}

export function normalizeTaskImageResult(result: TaskImageResultInput): TaskImageResult | null {
  const url = typeof result?.url === 'string' ? normalizeTaskImageUrl(result.url) : ''
  if (!url) return null
  const width = toPositiveNumber(result?.width)
  const height = toPositiveNumber(result?.height)
  const mimeType = `${result?.mimeType ?? ''}`.trim()
  const mediaAssetId = toPositiveInteger(result?.mediaAssetId)
  return {
    url,
    ...(width ? { width } : {}),
    ...(height ? { height } : {}),
    ...(mimeType ? { mimeType } : {}),
    ...(mediaAssetId ? { mediaAssetId } : {}),
  }
}

export function normalizeTaskImageResults(results: TaskImageResultInput[]): TaskImageResult[] {
  return results
    .map(normalizeTaskImageResult)
    .filter((result): result is TaskImageResult => result !== null)
}

export function extractImageResultsFromTaskPayload(
  task: TaskResultPayloadLike,
): TaskImageResult[] {
  const outputResults = Array.isArray(task.outputs)
    ? task.outputs
        .filter((output) => output?.role === 'image' && typeof output.url === 'string')
        .map((output) =>
          normalizeTaskImageResult({
            url: output.url ?? '',
            width: output.width,
            height: output.height,
            mimeType: output.mime_type,
            mediaAssetId: output.media_asset_id,
          }),
        )
        .filter((result): result is TaskImageResult => result !== null)
    : []
  if (outputResults.length > 0) return outputResults
  return extractImageResultsFromTaskResult(task.result)
}
