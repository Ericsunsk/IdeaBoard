import { effectScope } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useRafScheduler } from './useRafScheduler'

describe('useRafScheduler', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('runs the latest scheduled callback on the next animation frame', () => {
    const callback = vi.fn()
    const latestCallback = vi.fn()
    const frameCallbacks: FrameRequestCallback[] = []
    const cancelAnimationFrame = vi.fn()

    vi.stubGlobal(
      'requestAnimationFrame',
      vi.fn((frameCallback: FrameRequestCallback) => {
        frameCallbacks.push(frameCallback)
        return frameCallbacks.length
      }),
    )
    vi.stubGlobal('cancelAnimationFrame', cancelAnimationFrame)

    const scheduler = useRafScheduler()

    scheduler.schedule(callback)
    scheduler.schedule(latestCallback)
    frameCallbacks.forEach((frameCallback) => frameCallback(0))

    expect(cancelAnimationFrame).toHaveBeenCalledWith(1)
    expect(callback).not.toHaveBeenCalled()
    expect(latestCallback).toHaveBeenCalledTimes(1)
  })

  it('falls back to a timeout when requestAnimationFrame is unavailable', () => {
    vi.stubGlobal('requestAnimationFrame', undefined)
    const scheduler = useRafScheduler()
    const callback = vi.fn()

    scheduler.schedule(callback)
    vi.runOnlyPendingTimers()

    expect(callback).toHaveBeenCalledTimes(1)
  })

  it('cancels pending work when the scope is disposed', () => {
    const callback = vi.fn()
    const frameCallbacks: FrameRequestCallback[] = []
    const cancelAnimationFrame = vi.fn()

    vi.stubGlobal(
      'requestAnimationFrame',
      vi.fn((frameCallback: FrameRequestCallback) => {
        frameCallbacks.push(frameCallback)
        return frameCallbacks.length
      }),
    )
    vi.stubGlobal('cancelAnimationFrame', cancelAnimationFrame)

    const scope = effectScope()
    scope.run(() => {
      const scheduler = useRafScheduler()
      scheduler.schedule(callback)
    })

    scope.stop()
    frameCallbacks.forEach((frameCallback) => frameCallback(0))

    expect(cancelAnimationFrame).toHaveBeenCalledWith(1)
    expect(callback).not.toHaveBeenCalled()
  })
})
