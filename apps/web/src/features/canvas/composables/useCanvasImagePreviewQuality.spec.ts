import { describe, expect, it } from 'vitest'
import {
  CANVAS_IMAGE_PREVIEW_MAX_SIDE,
  resolveCanvasImagePreviewMaxSide,
} from './useCanvasImagePreviewQuality'

describe('useCanvasImagePreviewQuality', () => {
  it('chooses coarse preview tiers from rendered image size', () => {
    expect(
      resolveCanvasImagePreviewMaxSide({
        stageWidth: 280,
        stageHeight: 180,
        zoomLevel: 30,
        devicePixelRatio: 2,
      }),
    ).toBe(CANVAS_IMAGE_PREVIEW_MAX_SIDE.compact)

    expect(
      resolveCanvasImagePreviewMaxSide({
        stageWidth: 280,
        stageHeight: 180,
        zoomLevel: 80,
        devicePixelRatio: 2,
      }),
    ).toBe(CANVAS_IMAGE_PREVIEW_MAX_SIDE.balanced)

    expect(
      resolveCanvasImagePreviewMaxSide({
        stageWidth: 360,
        stageHeight: 260,
        zoomLevel: 120,
        devicePixelRatio: 2,
      }),
    ).toBe(CANVAS_IMAGE_PREVIEW_MAX_SIDE.detailed)

    expect(
      resolveCanvasImagePreviewMaxSide({
        stageWidth: 480,
        stageHeight: 360,
        zoomLevel: 160,
        devicePixelRatio: 2,
      }),
    ).toBe(CANVAS_IMAGE_PREVIEW_MAX_SIDE.full)
  })

  it('forces full quality while image editing or expand tools are active', () => {
    expect(
      resolveCanvasImagePreviewMaxSide({
        stageWidth: 280,
        stageHeight: 180,
        zoomLevel: 20,
        devicePixelRatio: 2,
        forceFullQuality: true,
      }),
    ).toBe(CANVAS_IMAGE_PREVIEW_MAX_SIDE.full)
  })
})
