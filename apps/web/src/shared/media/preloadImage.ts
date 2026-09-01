/** Decode an image before it becomes visible in the Canvas. */
export async function preloadImage(url: string): Promise<boolean> {
  const source = url.trim()
  if (!source) return false
  if (typeof Image === 'undefined') return true

  const image = new Image()
  image.decoding = 'async'
  image.src = source

  try {
    await image.decode()
    return true
  } catch {
    return false
  }
}
