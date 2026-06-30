import type { Rect } from '@vue-flow/core'

export interface CanvasMiniMapViewportRectParams {
  viewportBounds: Rect
  nodeBounds: Rect | null
  width: number
  height: number
  offsetScale: number
}

export interface CanvasMiniMapGeometry {
  viewportRect: Rect
  viewBox: Rect
}

export function resolveCanvasMiniMapViewportRect({
  viewportBounds,
  nodeBounds,
  width,
  height,
  offsetScale,
}: CanvasMiniMapViewportRectParams): Rect | null {
  return (
    resolveCanvasMiniMapGeometry({
      viewportBounds,
      nodeBounds,
      width,
      height,
      offsetScale,
    })?.viewportRect ?? null
  )
}

export function resolveCanvasMiniMapGeometry({
  viewportBounds,
  nodeBounds,
  width,
  height,
  offsetScale,
}: CanvasMiniMapViewportRectParams): CanvasMiniMapGeometry | null {
  if (
    width <= 0 ||
    height <= 0 ||
    !isFiniteRect(viewportBounds) ||
    (nodeBounds !== null && !isFiniteRect(nodeBounds))
  ) {
    return null
  }

  const bounds = nodeBounds ? getBoundsOfRects(nodeBounds, viewportBounds) : viewportBounds
  const scale = Math.max(bounds.width / width, bounds.height / height)
  if (!Number.isFinite(scale) || scale <= 0) return null

  const scaledWidth = scale * width
  const scaledHeight = scale * height
  const offset = offsetScale * scale
  const viewBox = {
    x: bounds.x - (scaledWidth - bounds.width) / 2 - offset,
    y: bounds.y - (scaledHeight - bounds.height) / 2 - offset,
    width: scaledWidth + offset * 2,
    height: scaledHeight + offset * 2,
  }

  return {
    viewBox,
    viewportRect: {
      x: ((viewportBounds.x - viewBox.x) / viewBox.width) * width,
      y: ((viewportBounds.y - viewBox.y) / viewBox.height) * height,
      width: (viewportBounds.width / viewBox.width) * width,
      height: (viewportBounds.height / viewBox.height) * height,
    },
  }
}

export function resolveCanvasMiniMapViewportForPoint(params: {
  point: { x: number; y: number }
  viewportBounds: Rect
  viewBox: Rect
  width: number
  height: number
  zoom: number
}) {
  const { point, viewportBounds, viewBox, width, height, zoom } = params
  if (
    width <= 0 ||
    height <= 0 ||
    zoom <= 0 ||
    !Number.isFinite(point.x) ||
    !Number.isFinite(point.y) ||
    !isFiniteRect(viewportBounds) ||
    !isFiniteRect(viewBox)
  ) {
    return null
  }

  const centerX = viewBox.x + (point.x / width) * viewBox.width
  const centerY = viewBox.y + (point.y / height) * viewBox.height

  return {
    x: -(centerX - viewportBounds.width / 2) * zoom,
    y: -(centerY - viewportBounds.height / 2) * zoom,
    zoom,
  }
}

function isFiniteRect(rect: Rect) {
  return (
    Number.isFinite(rect.x) &&
    Number.isFinite(rect.y) &&
    Number.isFinite(rect.width) &&
    Number.isFinite(rect.height) &&
    rect.width > 0 &&
    rect.height > 0
  )
}

function getBoundsOfRects(a: Rect, b: Rect): Rect {
  const x1 = Math.min(a.x, b.x)
  const y1 = Math.min(a.y, b.y)
  const x2 = Math.max(a.x + a.width, b.x + b.width)
  const y2 = Math.max(a.y + a.height, b.y + b.height)

  return {
    x: x1,
    y: y1,
    width: x2 - x1,
    height: y2 - y1,
  }
}
