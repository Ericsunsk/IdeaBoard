import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, ref, watch } from 'vue'
import { useCanvasImageQualityZoom } from './useCanvasImageQualityZoom'
import { useCanvasImagePreviewQuality } from './useCanvasImagePreviewQuality'

function createHarness(isToolActive = false) {
  const scope = effectScope()
  const zoomLevel = ref(30)
  const boardId = ref('board-a')
  const state = scope.run(() => {
    const quality = useCanvasImageQualityZoom(zoomLevel, () => boardId.value)
    const shouldForceFullQuality = ref(isToolActive)
    const preview = useCanvasImagePreviewQuality({
      imageQuality: quality.imageQuality,
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
    expect(harness.maxSide.value).toBe(256)
    expect(harness.changedTier).not.toHaveBeenCalled()

    finish()
    vi.advanceTimersByTime(79)
    expect(harness.maxSide.value).toBe(256)
    vi.advanceTimersByTime(1)
    expect(harness.maxSide.value).toBe(1280)
    expect(harness.changedTier).toHaveBeenCalledOnce()
    harness.scope.stop()
  })

  it('coalesces fragmented gestures and zoom changes without movement events', () => {
    const harness = createHarness()
    const finish = harness.beginMovement()
    harness.zoomLevel.value = 80
    finish()
    vi.advanceTimersByTime(60)
    const finishNext = harness.beginMovement()
    harness.zoomLevel.value = 120
    vi.advanceTimersByTime(300)
    expect(harness.imageQuality.zoomLevel.value).toBe(30)
    finishNext()
    vi.advanceTimersByTime(60)
    harness.zoomLevel.value = 160
    vi.advanceTimersByTime(79)
    expect(harness.imageQuality.zoomLevel.value).toBe(30)
    vi.advanceTimersByTime(1)
    expect(harness.imageQuality.zoomLevel.value).toBe(160)
    harness.scope.stop()
  })

  it('waits for every overlapping movement before applying tool quality', () => {
    const harness = createHarness()
    const finishFirst = harness.beginMovement()
    const finishSecond = harness.beginMovement()
    harness.zoomLevel.value = 80
    finishFirst()
    vi.advanceTimersByTime(500)
    expect(harness.imageQuality.zoomLevel.value).toBe(30)
    harness.shouldForceFullQuality.value = true
    expect(harness.maxSide.value).toBe(256)
    finishSecond()
    vi.advanceTimersByTime(80)
    expect(harness.maxSide.value).toBeNull()
    expect(harness.changedTier).toHaveBeenCalledOnce()
    harness.scope.stop()
  })

  it('holds tool quality from activation through focus zoom and its idle window', () => {
    const harness = createHarness()
    harness.shouldForceFullQuality.value = true
    expect(harness.maxSide.value).toBe(256)
    vi.advanceTimersByTime(16)
    const finish = harness.beginMovement()
    harness.zoomLevel.value = 80
    vi.advanceTimersByTime(220)
    harness.zoomLevel.value = 160
    expect(harness.maxSide.value).toBe(256)
    finish()
    vi.advanceTimersByTime(79)
    expect(harness.maxSide.value).toBe(256)
    vi.advanceTimersByTime(1)
    expect(harness.maxSide.value).toBeNull()
    expect(harness.changedTier).toHaveBeenCalledOnce()
    harness.scope.stop()
  })

  it('settles tool quality when focus does not change zoom', () => {
    const harness = createHarness()
    harness.shouldForceFullQuality.value = true
    const finish = harness.beginMovement()
    vi.advanceTimersByTime(220)
    expect(harness.maxSide.value).toBe(256)
    finish()
    vi.advanceTimersByTime(79)
    expect(harness.maxSide.value).toBe(256)
    vi.advanceTimersByTime(1)
    expect(harness.maxSide.value).toBeNull()
    harness.scope.stop()
  })

  it('settles an initially active tool without viewport movement', () => {
    const harness = createHarness(true)
    expect(harness.maxSide.value).toBe(256)
    vi.advanceTimersByTime(79)
    expect(harness.maxSide.value).toBe(256)
    vi.advanceTimersByTime(1)
    expect(harness.maxSide.value).toBeNull()
    harness.scope.stop()
  })

  it('discards full-quality demand when a tool exits before focus finishes', () => {
    const harness = createHarness()
    harness.shouldForceFullQuality.value = true
    const finish = harness.beginMovement()
    harness.zoomLevel.value = 160
    harness.shouldForceFullQuality.value = false
    vi.advanceTimersByTime(500)
    expect(harness.maxSide.value).toBe(256)
    finish()
    vi.advanceTimersByTime(80)
    expect(harness.maxSide.value).toBe(1280)
    expect(harness.changedTier).toHaveBeenCalledOnce()
    harness.scope.stop()
  })

  it('keeps the original while exiting a tool and restoring the viewport', () => {
    const harness = createHarness(true)
    vi.advanceTimersByTime(80)
    expect(harness.maxSide.value).toBeNull()
    harness.shouldForceFullQuality.value = false
    const finish = harness.beginMovement()
    harness.zoomLevel.value = 80
    vi.advanceTimersByTime(220)
    expect(harness.maxSide.value).toBeNull()
    finish()
    vi.advanceTimersByTime(80)
    expect(harness.maxSide.value).toBe(640)
    harness.scope.stop()
  })

  it('does not publish a tier change when the settled zoom stays in the same tier', () => {
    const harness = createHarness()
    harness.zoomLevel.value = 25
    vi.advanceTimersByTime(80)
    expect(harness.imageQuality.zoomLevel.value).toBe(25)
    expect(harness.changedTier).not.toHaveBeenCalled()
    harness.scope.stop()
  })

  it('discards pending work and old movement completions across board changes and disposal', () => {
    const harness = createHarness()
    const finishOld = harness.beginMovement()
    harness.zoomLevel.value = 80
    finishOld()
    harness.boardId.value = 'board-b'
    expect(harness.imageQuality.zoomLevel.value).toBe(80)
    const finishCurrent = harness.beginMovement()
    harness.zoomLevel.value = 160
    finishOld()
    vi.advanceTimersByTime(500)
    expect(harness.imageQuality.zoomLevel.value).toBe(80)
    finishCurrent()
    harness.scope.stop()
    vi.advanceTimersByTime(500)
    expect(harness.imageQuality.zoomLevel.value).toBe(80)
    expect(vi.getTimerCount()).toBe(0)
  })
})
