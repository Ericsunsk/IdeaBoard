export const PNG_PREVIEW_MIME_TYPE = 'image/png'
export const WEBP_PREVIEW_MIME_TYPE = 'image/webp'
export const WEBP_PREVIEW_QUALITY = 0.82

export type PreviewMimeType = typeof PNG_PREVIEW_MIME_TYPE | typeof WEBP_PREVIEW_MIME_TYPE

export function imageDataHasTransparentPixels(data: Uint8ClampedArray): boolean {
  for (let alphaIndex = 3; alphaIndex < data.length; alphaIndex += 4) {
    if (data[alphaIndex] < 255) return true
  }
  return false
}

export function detectCanvasTransparency(
  ctx: CanvasRenderingContext2D,
  width: number,
  height: number,
): boolean | null {
  try {
    return imageDataHasTransparentPixels(ctx.getImageData(0, 0, width, height).data)
  } catch {
    return null
  }
}

export function resolvePreviewMimeType(
  sourceMime: string,
  hasTransparentPixels: boolean | null,
): PreviewMimeType {
  // Prefer PNG for alpha previews so cutout results are not obscured by lossy edge artifacts.
  if (hasTransparentPixels === true) return PNG_PREVIEW_MIME_TYPE
  if (hasTransparentPixels === false) return WEBP_PREVIEW_MIME_TYPE
  return sourceMime.trim().toLowerCase() === PNG_PREVIEW_MIME_TYPE
    ? PNG_PREVIEW_MIME_TYPE
    : WEBP_PREVIEW_MIME_TYPE
}
