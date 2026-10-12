import { afterEach, describe, expect, it, vi } from 'vitest'
import { effectScope } from 'vue'
import {
  CANVAS_IMAGE_PREVIEW_MAX_SIDE,
  resolveCanvasImagePreviewMaxSide,
  useCanvasImagePreviewQuality,
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

  it('forces full quality while retouch or outpaint tools are active', () => {
    expect(
      resolveCanvasImagePreviewMaxSide({
        stageWidth: 280,
        stageHeight: 180,
        zoomLevel: 20,
        devicePixelRatio: 2,
        shouldForceFullQuality: true,
      }),
    ).toBe(CANVAS_IMAGE_PREVIEW_MAX_SIDE.full)
  })

  describe('system devicePixelRatio tracking', () => {
    afterEach(() => {
      vi.unstubAllGlobals()
      vi.restoreAllMocks()
    })

    it('follows system devicePixelRatio changes without a zoom or stage change', async () => {
      let currentDpr = 1
      const changeListeners = new Set<() => void>()
      Object.defineProperty(window, 'devicePixelRatio', {
        configurable: true,
        get: () => currentDpr,
      })
      vi.stubGlobal(
        'matchMedia',
        vi.fn(
          () =>
            ({
              matches: true,
              addEventListener: (_type: string, listener: () => void) => {
                changeListeners.add(listener)
              },
              removeEventListener: (_type: string, listener: () => void) => {
                changeListeners.delete(listener)
              },
            }) satisfies Pick<MediaQueryList, 'matches'> & Record<string, unknown>,
        ),
      )

      const scope = effectScope()
      const quality = scope.run(() =>
        useCanvasImagePreviewQuality({
          stageWidth: 400,
          stageHeight: 400,
          zoomLevel: 100,
        }),
      )
      if (!quality) throw new Error('preview quality was not created')

      // 400 rendered px at DPR 1 lands in the balanced tier.
      expect(quality.maxSide.value).toBe(CANVAS_IMAGE_PREVIEW_MAX_SIDE.balanced)

      currentDpr = 2
      for (const listener of [...changeListeners]) listener()
      expect(quality.maxSide.value).toBe(CANVAS_IMAGE_PREVIEW_MAX_SIDE.detailed)

      scope.stop()
      expect(changeListeners.size).toBe(0)
    })

    it('prefers an explicit devicePixelRatio over the system value', () => {
      const scope = effectScope()
      const quality = scope.run(() =>
        useCanvasImagePreviewQuality({
          stageWidth: 400,
          stageHeight: 400,
          zoomLevel: 100,
          devicePixelRatio: 3,
        }),
      )
      if (!quality) throw new Error('preview quality was not created')

      // 400 * 3 = 1200 rendered px exceeds the detailed limit: full quality.
      expect(quality.maxSide.value).toBe(CANVAS_IMAGE_PREVIEW_MAX_SIDE.full)
      scope.stop()
    })
  })
})
