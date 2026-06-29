import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { useMaskFeatherStrengthControl } from './useMaskFeatherStrengthControl'

function createMouseEvent(type: string, clientX: number) {
  return new MouseEvent(type, { button: 0, clientX })
}

function createTrack(left = 0, width = 100) {
  const track = document.createElement('div')
  track.getBoundingClientRect = vi.fn(
    () =>
      ({
        left,
        right: left + width,
        width,
        top: 0,
        bottom: 6,
        height: 6,
        x: left,
        y: 0,
        toJSON: () => undefined,
      }) as DOMRect,
  )
  return track
}

describe('useMaskFeatherStrengthControl', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('applies the current strength on ordinary click without entering long press mode', () => {
    const onApply = vi.fn()
    const control = useMaskFeatherStrengthControl({ onApply })

    control.onButtonMouseDown(createMouseEvent('mousedown', 40))
    vi.advanceTimersByTime(259)
    window.dispatchEvent(createMouseEvent('mouseup', 40))
    control.onButtonClick()

    expect(control.visible.value).toBe(false)
    expect(onApply).toHaveBeenCalledWith(35)
    expect(onApply).toHaveBeenCalledTimes(1)

    control.dispose()
  })

  it('shows the strength slider after long press', async () => {
    const onApply = vi.fn()
    const control = useMaskFeatherStrengthControl({ onApply })

    control.onButtonMouseDown(createMouseEvent('mousedown', 40))
    vi.advanceTimersByTime(260)
    await nextTick()

    expect(control.visible.value).toBe(true)
    expect(onApply).not.toHaveBeenCalled()

    control.dispose()
  })

  it('updates strength while dragging and applies on mouseup', async () => {
    const onApply = vi.fn()
    const control = useMaskFeatherStrengthControl({ onApply })
    control.trackRef.value = createTrack()

    control.onButtonMouseDown(createMouseEvent('mousedown', 20))
    vi.advanceTimersByTime(260)
    await nextTick()
    window.dispatchEvent(createMouseEvent('mousemove', 80))
    window.dispatchEvent(createMouseEvent('mouseup', 80))

    expect(control.visible.value).toBe(false)
    expect(control.strength.value).toBe(80)
    expect(onApply).toHaveBeenCalledWith(80)
    expect(onApply).toHaveBeenCalledTimes(1)

    control.dispose()
  })

  it('disposes pending timer and global listeners', () => {
    const onApply = vi.fn()
    const removeEventListener = vi.spyOn(window, 'removeEventListener')
    const control = useMaskFeatherStrengthControl({ onApply })

    control.onButtonMouseDown(createMouseEvent('mousedown', 40))
    control.dispose()
    vi.advanceTimersByTime(260)
    window.dispatchEvent(createMouseEvent('mouseup', 40))

    expect(control.visible.value).toBe(false)
    expect(onApply).not.toHaveBeenCalled()
    expect(removeEventListener).toHaveBeenCalledWith('mousemove', expect.any(Function), true)
    expect(removeEventListener).toHaveBeenCalledWith('mouseup', expect.any(Function), true)
  })

  it('hides the strength slider when disposed during drag mode', async () => {
    const onApply = vi.fn()
    const control = useMaskFeatherStrengthControl({ onApply })

    control.onButtonMouseDown(createMouseEvent('mousedown', 40))
    vi.advanceTimersByTime(260)
    await nextTick()
    control.dispose()

    expect(control.visible.value).toBe(false)
    expect(onApply).not.toHaveBeenCalled()
  })

  it('suppresses the next button click after track drag applies', async () => {
    const onApply = vi.fn()
    const control = useMaskFeatherStrengthControl({ onApply })
    control.trackRef.value = createTrack()

    control.onTrackMouseDown(createMouseEvent('mousedown', 60))
    await nextTick()
    window.dispatchEvent(createMouseEvent('mouseup', 60))
    control.onButtonClick()

    expect(onApply).toHaveBeenCalledWith(60)
    expect(onApply).toHaveBeenCalledTimes(1)

    control.dispose()
  })
})
