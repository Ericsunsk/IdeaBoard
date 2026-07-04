import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useMaskFeatherStrengthControl } from './useMaskFeatherStrengthControl'

type FeatherControl = ReturnType<typeof useMaskFeatherStrengthControl>

function createPointerEvent(type: string, init: Partial<PointerEvent> = {}) {
  return new PointerEvent(type, {
    bubbles: true,
    button: 0,
    pointerType: 'mouse',
    ...init,
  })
}

function createRange(value = 35) {
  const input = document.createElement('input')
  input.type = 'range'
  input.min = '0'
  input.max = '100'
  input.step = '1'
  input.value = `${value}`
  return input
}

function dispatchRangeInput(input: HTMLInputElement, value: number) {
  input.value = `${value}`
  input.dispatchEvent(new Event('input', { bubbles: true }))
}

function bindRange(control: FeatherControl, value = 35) {
  const input = createRange(value)
  input.addEventListener('pointerdown', control.onButtonPointerDown)
  input.addEventListener('input', control.onStrengthInput)
  input.addEventListener('change', control.onStrengthChange)
  input.addEventListener('pointerup', control.onStrengthPointerUp)
  control.setRangeRef(input)
  return input
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
    const input = bindRange(control)

    input.dispatchEvent(createPointerEvent('pointerdown'))
    vi.advanceTimersByTime(259)
    input.dispatchEvent(createPointerEvent('pointerup'))
    control.onButtonClick()

    expect(control.visible.value).toBe(false)
    expect(onApply).toHaveBeenCalledWith(35)
    expect(onApply).toHaveBeenCalledTimes(1)

    control.dispose()
  })

  it('shows the strength slider after long press', () => {
    const onApply = vi.fn()
    const control = useMaskFeatherStrengthControl({ onApply })
    const input = bindRange(control)

    input.dispatchEvent(createPointerEvent('pointerdown'))
    vi.advanceTimersByTime(260)

    expect(control.visible.value).toBe(true)
    expect(onApply).not.toHaveBeenCalled()

    control.dispose()
  })

  it('updates strength with the native range and applies on pointerup', () => {
    const onApply = vi.fn()
    const control = useMaskFeatherStrengthControl({ onApply })
    const input = bindRange(control)

    input.dispatchEvent(createPointerEvent('pointerdown'))
    vi.advanceTimersByTime(260)
    dispatchRangeInput(input, 80)
    input.dispatchEvent(createPointerEvent('pointerup'))

    expect(control.visible.value).toBe(false)
    expect(control.strength.value).toBe(80)
    expect(onApply).toHaveBeenCalledWith(80)
    expect(onApply).toHaveBeenCalledTimes(1)

    control.dispose()
  })

  it('applies keyboard range edits through change', () => {
    const onApply = vi.fn()
    const control = useMaskFeatherStrengthControl({ onApply })
    const input = bindRange(control)

    input.dispatchEvent(createPointerEvent('pointerdown'))
    vi.advanceTimersByTime(260)
    dispatchRangeInput(input, 64)
    input.dispatchEvent(new Event('change', { bubbles: true }))

    expect(control.visible.value).toBe(false)
    expect(control.strength.value).toBe(64)
    expect(onApply).toHaveBeenCalledWith(64)

    control.dispose()
  })

  it('disposes pending long press listeners', () => {
    const onApply = vi.fn()
    const removeEventListener = vi.spyOn(HTMLInputElement.prototype, 'removeEventListener')
    const control = useMaskFeatherStrengthControl({ onApply })
    const input = bindRange(control)

    input.dispatchEvent(createPointerEvent('pointerdown'))
    control.dispose()
    vi.advanceTimersByTime(260)
    input.dispatchEvent(createPointerEvent('pointerup'))

    expect(control.visible.value).toBe(false)
    expect(onApply).not.toHaveBeenCalled()
    expect(removeEventListener).toHaveBeenCalled()
  })

  it('hides the strength slider when disposed during edit mode', () => {
    const onApply = vi.fn()
    const control = useMaskFeatherStrengthControl({ onApply })
    const input = bindRange(control)

    input.dispatchEvent(createPointerEvent('pointerdown'))
    vi.advanceTimersByTime(260)
    control.dispose()

    expect(control.visible.value).toBe(false)
    expect(onApply).not.toHaveBeenCalled()
  })

  it('suppresses the next button click after range edit applies', () => {
    const onApply = vi.fn()
    const control = useMaskFeatherStrengthControl({ onApply })
    const input = bindRange(control)

    input.dispatchEvent(createPointerEvent('pointerdown'))
    vi.advanceTimersByTime(260)
    dispatchRangeInput(input, 60)
    input.dispatchEvent(createPointerEvent('pointerup'))
    control.onButtonClick()

    expect(onApply).toHaveBeenCalledWith(60)
    expect(onApply).toHaveBeenCalledTimes(1)

    control.dispose()
  })
})
