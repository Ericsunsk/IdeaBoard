import { describe, expect, it } from 'vitest'
import { ref } from 'vue'
import {
  resolveCanvasImagePreviewMaxSide,
  useCanvasImagePreviewQuality,
} from './useCanvasImagePreviewQuality'

describe('useCanvasImagePreviewQuality', () => {
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
    const quality = useCanvasImagePreviewQuality({ zoomLevel, shouldForceFullQuality })
    expect(quality.maxSide.value).toBe(256)
    shouldForceFullQuality.value = true
    expect(quality.maxSide.value).toBeNull()
    zoomLevel.value = 200
    expect(quality.maxSide.value).toBeNull()
    shouldForceFullQuality.value = false
    expect(quality.maxSide.value).toBe(1280)
  })
})
