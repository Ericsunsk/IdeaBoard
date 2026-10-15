import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, ref, watch } from 'vue'
import { useCanvasImageQualityZoom } from './useCanvasImageQualityZoom'
import { useCanvasImagePreviewQuality } from './useCanvasImagePreviewQuality'

function createHarness() {
  const scope = effectScope()
  const zoomLevel = ref(30)
  const boardId = ref('board-a')
  const state = scope.run(() => {
    const quality = useCanvasImageQualityZoom(zoomLevel, () => boardId.value)
    const shouldForceFullQuality = ref(false)
    const preview = useCanvasImagePreviewQuality({
      stageWidth: 800,
      stageHeight: 600,
      zoomLevel: quality.qualityZoomLevel,
      devicePixelRatio: 1,
      shouldForceFullQuality,
    })
    const changedTier = vi.fn()
    watch(preview.maxSide, changedTier, { flush: 'sync' })
    return { ...quality, ...preview, shouldForceFullQuality, changedTier }
  })
  if (!state) throw new Error('quality state was not created')
  return { scope, zoomLevel, boardId, ...state }
}

describe('useCanvasImageQualityZoom', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it('keeps one tier throughout a gesture and selects only its final zoom after settling', () => {
    const harness = createHarness()
    const finish = harness.beginMovement()
    harness.zoomLevel.value = 80
    vi.advanceTimersByTime(500)
    harness.zoomLevel.value = 160
    vi.advanceTimersByTime(500)
    expect(harness.maxSide.value).toBe(320)
    expect(harness.changedTier).not.toHaveBeenCalled()

    finish()
    vi.advanceTimersByTime(199)
    expect(harness.maxSide.value).toBe(320)
    vi.advanceTimersByTime(1)
    expect(harness.maxSide.value).toBeNull()
    expect(harness.changedTier).toHaveBeenCalledOnce()
    harness.scope.stop()
  })

  it('coalesces fragmented gestures and zoom changes without movement events', () => {
    const harness = createHarness()
    const finish = harness.beginMovement()
    harness.zoomLevel.value = 80
    finish()
    vi.advanceTimersByTime(150)
    const finishNext = harness.beginMovement()
    harness.zoomLevel.value = 120
    vi.advanceTimersByTime(300)
    expect(harness.qualityZoomLevel.value).toBe(30)
    finishNext()
    vi.advanceTimersByTime(150)
    harness.zoomLevel.value = 160
    vi.advanceTimersByTime(199)
    expect(harness.qualityZoomLevel.value).toBe(30)
    vi.advanceTimersByTime(1)
    expect(harness.qualityZoomLevel.value).toBe(160)
    harness.scope.stop()
  })

  it('waits for every overlapping movement and permits immediate tool quality', () => {
    const harness = createHarness()
    const finishFirst = harness.beginMovement()
    const finishSecond = harness.beginMovement()
    harness.zoomLevel.value = 80
    finishFirst()
    vi.advanceTimersByTime(500)
    expect(harness.qualityZoomLevel.value).toBe(30)
    harness.shouldForceFullQuality.value = true
    expect(harness.maxSide.value).toBeNull()
    harness.shouldForceFullQuality.value = false
    expect(harness.maxSide.value).toBe(320)
    finishSecond()
    vi.advanceTimersByTime(200)
    expect(harness.maxSide.value).toBe(1280)
    harness.scope.stop()
  })

  it('does not publish a tier change when the settled zoom stays in the same tier', () => {
    const harness = createHarness()
    harness.zoomLevel.value = 25
    vi.advanceTimersByTime(200)
    expect(harness.qualityZoomLevel.value).toBe(25)
    expect(harness.changedTier).not.toHaveBeenCalled()
    harness.scope.stop()
  })

  it('discards pending work and old movement completions across board changes and disposal', () => {
    const harness = createHarness()
    const finishOld = harness.beginMovement()
    harness.zoomLevel.value = 80
    finishOld()
    harness.boardId.value = 'board-b'
    expect(harness.qualityZoomLevel.value).toBe(80)
    const finishCurrent = harness.beginMovement()
    harness.zoomLevel.value = 160
    finishOld()
    vi.advanceTimersByTime(500)
    expect(harness.qualityZoomLevel.value).toBe(80)
    finishCurrent()
    harness.scope.stop()
    vi.advanceTimersByTime(500)
    expect(harness.qualityZoomLevel.value).toBe(80)
    expect(vi.getTimerCount()).toBe(0)
  })
})
