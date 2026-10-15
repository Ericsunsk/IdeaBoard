import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, ref } from 'vue'
import {
  resolveCanvasImagePreviewMaxSide,
  useCanvasImagePreviewQuality,
} from './useCanvasImagePreviewQuality'
import { useCanvasImageQualityZoom } from './useCanvasImageQualityZoom'

describe('useCanvasImagePreviewQuality', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())
  it.each([
    [15, 256],
    [76.999, 256],
    [77, 640],
    [100, 640],
    [137.999, 640],
    [138, 1280],
    [200, 1280],
    [1000, 1280],
    [Number.NaN, 256],
    [Number.POSITIVE_INFINITY, 256],
    [Number.NEGATIVE_INFINITY, 256],
  ])('selects %s percent zoom as max side %s', (zoomLevel, maxSide) => {
    expect(resolveCanvasImagePreviewMaxSide({ zoomLevel })).toBe(maxSide)
  })

  it('requests original only while an image tool explicitly forces full quality', () => {
    const zoomLevel = ref(30)
    const shouldForceFullQuality = ref(false)
    const scope = effectScope()
    const quality = scope.run(() => {
      const { imageQuality } = useCanvasImageQualityZoom(zoomLevel, () => 'board-a')
      return useCanvasImagePreviewQuality({ imageQuality, shouldForceFullQuality })
    })
    if (!quality) throw new Error('quality state was not created')
    expect(quality.maxSide.value).toBe(256)
    shouldForceFullQuality.value = true
    vi.advanceTimersByTime(80)
    expect(quality.maxSide.value).toBeNull()
    zoomLevel.value = 200
    expect(quality.maxSide.value).toBeNull()
    shouldForceFullQuality.value = false
    vi.advanceTimersByTime(80)
    expect(quality.maxSide.value).toBe(1280)
    scope.stop()
  })
})
