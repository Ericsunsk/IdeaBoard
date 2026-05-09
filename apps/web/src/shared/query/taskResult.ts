export function extractImageUrlsFromTaskResult(result: string | null | undefined): string[] {
  if (!result) return []
  try {
    const parsed = JSON.parse(result)
    if (Array.isArray(parsed?.image_urls)) {
      return parsed.image_urls.filter((url: unknown): url is string => typeof url === 'string' && url.trim().length > 0)
    }
    if (typeof parsed?.image_url === 'string' && parsed.image_url.trim()) return [parsed.image_url.trim()]
    return []
  } catch {
    return []
  }
}
