export type OutpaintRect = {
  left: number
  top: number
  width: number
  height: number
}

export type OutpaintResolvedIntent = {
  frameRect: OutpaintRect
  originalImageRect: OutpaintRect
  targetRatio: number
  hasExtension: boolean
}

export type OutpaintPixelGeometry = {
  targetWidth: number
  targetHeight: number
  imageOffsetX: number
  imageOffsetY: number
}

export type OutpaintDragAnchor = 'n' | 'e' | 's' | 'w' | 'nw' | 'ne' | 'sw' | 'se'

const DEFAULT_OUTPAINT_PADDING_MIN = 24
const DEFAULT_OUTPAINT_PADDING_RATIO = 0.1
const DEFAULT_OUTPAINT_PADDING_MAX = 96

export function createOutpaintRect(
  left: number,
  top: number,
  width: number,
  height: number,
): OutpaintRect {
  return { left, top, width, height }
}

export function containImageRect(
  containerWidth: number,
  containerHeight: number,
  imageRatio: number,
): OutpaintRect | null {
  if (containerWidth <= 0 || containerHeight <= 0) return null
  const ratio = Number.isFinite(imageRatio) && imageRatio > 0 ? imageRatio : 1
  const containerRatio = containerHeight / containerWidth
  if (ratio >= containerRatio) {
    const height = containerHeight
    const width = height / ratio
    const left = (containerWidth - width) / 2
    return createOutpaintRect(left, 0, width, height)
  }
  const width = containerWidth
  const height = width * ratio
  const top = (containerHeight - height) / 2
  return createOutpaintRect(0, top, width, height)
}

export function clampFrameToImageRect(
  frame: OutpaintRect,
  imageRect: OutpaintRect,
): OutpaintRect {
  const originalLeft = imageRect.left
  const originalTop = imageRect.top
  const originalRight = imageRect.left + imageRect.width
  const originalBottom = imageRect.top + imageRect.height
  const left = Math.min(frame.left, originalLeft)
  const top = Math.min(frame.top, originalTop)
  const right = Math.max(frame.left + frame.width, originalRight)
  const bottom = Math.max(frame.top + frame.height, originalBottom)
  return createOutpaintRect(left, top, right - left, bottom - top)
}

export function resizeOutpaintFrameFromDrag(
  initialFrame: OutpaintRect,
  imageRect: OutpaintRect,
  anchor: OutpaintDragAnchor,
  deltaX: number,
  deltaY: number,
): OutpaintRect {
  const originalLeft = imageRect.left
  const originalTop = imageRect.top
  const originalRight = imageRect.left + imageRect.width
  const originalBottom = imageRect.top + imageRect.height

  const initialLeft = initialFrame.left
  const initialTop = initialFrame.top
  const initialRight = initialFrame.left + initialFrame.width
  const initialBottom = initialFrame.top + initialFrame.height

  let nextLeft = initialLeft
  let nextTop = initialTop
  let nextRight = initialRight
  let nextBottom = initialBottom

  if (anchor.includes('w')) {
    nextLeft = Math.min(initialLeft + deltaX, originalLeft)
  }
  if (anchor.includes('e')) {
    nextRight = Math.max(initialRight + deltaX, originalRight)
  }
  if (anchor.includes('n')) {
    nextTop = Math.min(initialTop + deltaY, originalTop)
  }
  if (anchor.includes('s')) {
    nextBottom = Math.max(initialBottom + deltaY, originalBottom)
  }

  return clampFrameToImageRect(
    createOutpaintRect(
      nextLeft,
      nextTop,
      Math.max(1, nextRight - nextLeft),
      Math.max(1, nextBottom - nextTop),
    ),
    imageRect,
  )
}

export function resolveDefaultOutpaintFrame(imageRect: OutpaintRect): OutpaintRect {
  const shortEdge = Math.min(imageRect.width, imageRect.height)
  const outpaintPadding = Math.max(
    DEFAULT_OUTPAINT_PADDING_MIN,
    Math.min(DEFAULT_OUTPAINT_PADDING_MAX, Math.round(shortEdge * DEFAULT_OUTPAINT_PADDING_RATIO)),
  )
  return createOutpaintRect(
    imageRect.left - outpaintPadding,
    imageRect.top - outpaintPadding,
    imageRect.width + outpaintPadding * 2,
    imageRect.height + outpaintPadding * 2,
  )
}

export function resolveOutpaintIntent(
  frameRect: OutpaintRect,
  originalImageRect: OutpaintRect,
): OutpaintResolvedIntent {
  const originalLeft = originalImageRect.left
  const originalTop = originalImageRect.top
  const originalWidth = Math.max(1, originalImageRect.width)
  const originalHeight = Math.max(1, originalImageRect.height)
  const originalRight = originalLeft + originalWidth
  const originalBottom = originalTop + originalHeight
  const frameLeft = Math.min(frameRect.left, originalLeft)
  const frameTop = Math.min(frameRect.top, originalTop)
  const frameRight = Math.max(frameRect.left + frameRect.width, originalRight)
  const frameBottom = Math.max(frameRect.top + frameRect.height, originalBottom)
  const normalizedFrameRect = createOutpaintRect(
    frameLeft,
    frameTop,
    Math.max(1, frameRight - frameLeft),
    Math.max(1, frameBottom - frameTop),
  )
  const extendLeftPx = Math.max(0, originalLeft - frameLeft)
  const extendTopPx = Math.max(0, originalTop - frameTop)
  const extendRightPx = Math.max(0, frameRight - originalRight)
  const extendBottomPx = Math.max(0, frameBottom - originalBottom)
  const hasExtension =
    extendTopPx > 0 || extendRightPx > 0 || extendBottomPx > 0 || extendLeftPx > 0
  return {
    frameRect: normalizedFrameRect,
    originalImageRect: createOutpaintRect(
      originalLeft,
      originalTop,
      originalWidth,
      originalHeight,
    ),
    targetRatio: normalizedFrameRect.height / normalizedFrameRect.width,
    hasExtension,
  }
}

export function resolveOutpaintPixelGeometry(
  imageSize: { width: number; height: number },
  frameRect: OutpaintRect,
  originalImageRect: OutpaintRect,
): OutpaintPixelGeometry {
  const sourceWidth = Math.max(1, Math.round(imageSize.width))
  const sourceHeight = Math.max(1, Math.round(imageSize.height))
  const originalWidth = Math.max(1, originalImageRect.width)
  const originalHeight = Math.max(1, originalImageRect.height)
  const originalRight = originalImageRect.left + originalWidth
  const originalBottom = originalImageRect.top + originalHeight
  const frameRight = frameRect.left + frameRect.width
  const frameBottom = frameRect.top + frameRect.height
  const imageOffsetX = Math.max(
    0,
    Math.round(sourceWidth * ((originalImageRect.left - frameRect.left) / originalWidth)),
  )
  const imageOffsetY = Math.max(
    0,
    Math.round(sourceHeight * ((originalImageRect.top - frameRect.top) / originalHeight)),
  )
  const rightExtension = Math.max(
    0,
    Math.round(sourceWidth * ((frameRight - originalRight) / originalWidth)),
  )
  const bottomExtension = Math.max(
    0,
    Math.round(sourceHeight * ((frameBottom - originalBottom) / originalHeight)),
  )
  return {
    targetWidth: sourceWidth + imageOffsetX + rightExtension,
    targetHeight: sourceHeight + imageOffsetY + bottomExtension,
    imageOffsetX,
    imageOffsetY,
  }
}
