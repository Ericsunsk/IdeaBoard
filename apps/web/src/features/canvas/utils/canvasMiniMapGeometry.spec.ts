import { describe, expect, it } from 'vitest'
import {
  resolveCanvasMiniMapGeometry,
  resolveCanvasMiniMapViewportForPoint,
  resolveCanvasMiniMapViewportRect,
} from './canvasMiniMapGeometry'

describe('canvasMiniMapGeometry', () => {
  it('maps the viewport to the full minimap when there are no node bounds', () => {
    expect(
      resolveCanvasMiniMapViewportRect({
        viewportBounds: { x: 0, y: 0, width: 1000, height: 500 },
        nodeBounds: null,
        width: 100,
        height: 50,
        offsetScale: 0,
      }),
    ).toEqual({ x: 0, y: 0, width: 100, height: 50 })
  })

  it('maps the viewport into the union of node bounds and viewport bounds', () => {
    const rect = resolveCanvasMiniMapViewportRect({
      viewportBounds: { x: 100, y: 50, width: 200, height: 100 },
      nodeBounds: { x: 0, y: 0, width: 400, height: 200 },
      width: 100,
      height: 50,
      offsetScale: 0,
    })

    expect(rect).toEqual({ x: 25, y: 12.5, width: 50, height: 25 })
  })

  it('returns null for invalid dimensions', () => {
    expect(
      resolveCanvasMiniMapViewportRect({
        viewportBounds: { x: 0, y: 0, width: 1000, height: 500 },
        nodeBounds: null,
        width: 0,
        height: 50,
        offsetScale: 0,
      }),
    ).toBeNull()
  })

  it('resolves a viewport transform centered on a minimap point', () => {
    const geometry = resolveCanvasMiniMapGeometry({
      viewportBounds: { x: 100, y: 50, width: 200, height: 100 },
      nodeBounds: { x: 0, y: 0, width: 400, height: 200 },
      width: 100,
      height: 50,
      offsetScale: 0,
    })

    expect(geometry).not.toBeNull()
    expect(
      resolveCanvasMiniMapViewportForPoint({
        point: { x: 50, y: 25 },
        viewportBounds: { x: 100, y: 50, width: 200, height: 100 },
        viewBox: geometry!.viewBox,
        width: 100,
        height: 50,
        zoom: 2,
      }),
    ).toEqual({ x: -200, y: -100, zoom: 2 })
  })
})
