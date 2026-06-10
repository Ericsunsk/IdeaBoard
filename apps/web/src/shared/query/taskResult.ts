import { resolveAssetUrl } from '../config/runtimeUrls'

function normalizeTaskImageUrl(url: string): string {
  return resolveAssetUrl(url.trim())
}

export function extractImageUrlsFromTaskResult(result: string | null | undefined): string[] {
  if (!result) return []
  try {
    const parsed = JSON.parse(result)
    if (Array.isArray(parsed?.image_urls)) {
      return parsed.image_urls
        .filter((url: unknown): url is string => typeof url === 'string' && url.trim().length > 0)
        .map(normalizeTaskImageUrl)
    }
    if (typeof parsed?.image_url === 'string' && parsed.image_url.trim())
      return [normalizeTaskImageUrl(parsed.image_url)]
    return []
  } catch {
    return []
  }
}
