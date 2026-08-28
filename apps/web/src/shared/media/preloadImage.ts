/**
 * Decode an image before it becomes visible in the Canvas.
 * Modern browsers expose Image.decode(); when unavailable we let the browser
 * handle decoding on paint rather than blocking uploads in legacy runtimes.
 */
export async function preloadImage(url: string): Promise<boolean> {
  const source = url.trim()
  if (!source) return false
  if (typeof Image === 'undefined') return true

  const image = new Image()
  image.decoding = 'async'
  image.src = source
  if (typeof image.decode !== 'function') return true

  try {
    await image.decode()
    return true
  } catch {
    return false
  }
}
