import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { IMAGE_NODE_ACTION } from '../../../shared/domain/imageActions'
import { useFlowImageActionStrengthControl } from './useFlowImageActionStrengthControl'
import type { ImageStrengthAction } from '../execution'

type StrengthControl = ReturnType<typeof useFlowImageActionStrengthControl>

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

function stubRangeBounds(input: HTMLInputElement, bounds: { left: number; width: number }) {
  input.getBoundingClientRect = vi.fn(
    () =>
      ({
        left: bounds.left,
        right: bounds.left + bounds.width,
        width: bounds.width,
        top: 0,
        bottom: 20,
        height: 20,
        x: bounds.left,
        y: 0,
        toJSON: () => ({}),
      }) as DOMRect,
  )
}

function bindRange(control: StrengthControl, action: ImageStrengthAction, value = 35) {
  const input = createRange(value)
  const trigger = document.createElement('button')
  input.addEventListener('pointerdown', (event) =>
    control.onStrengthRangePointerDown(action, event),
  )
  input.addEventListener('input', (event) => control.onStrengthInput(action, event))
  input.addEventListener('change', (event) => control.onStrengthChange(action, event))
  input.addEventListener('pointerup', (event) => control.onStrengthPointerUp(action, event))
  control.setStrengthRangeRef(action, input)
  control.setStrengthTriggerRef(action, trigger)
  return { input, trigger }
}

describe('useFlowImageActionStrengthControl', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('keeps ordinary pointer down as a pending click without committing strength', () => {
    const onCommit = vi.fn()
    const control = useFlowImageActionStrengthControl({ onCommit })
    const action = IMAGE_NODE_ACTION.styleVariation
    const { trigger } = bindRange(control, action)

    trigger.dispatchEvent(createPointerEvent('pointerdown'))
    vi.advanceTimersByTime(259)
    trigger.dispatchEvent(createPointerEvent('pointerup'))

    expect(control.activeStrengthAction.value).toBeNull()
    expect(control.consumeSuppressedStrengthClick(action)).toBe(false)
    expect(onCommit).not.toHaveBeenCalled()

    control.dispose()
  })

  it('clamps same-gesture strength drags to the native range bounds', () => {
    const onCommit = vi.fn()
    const control = useFlowImageActionStrengthControl({ onCommit })
    const action = IMAGE_NODE_ACTION.styleVariation
    const { input, trigger } = bindRange(control, action)
    stubRangeBounds(input, { left: 100, width: 200 })

    trigger.dispatchEvent(createPointerEvent('pointerdown', { pointerId: 7, clientX: 150 }))
    vi.advanceTimersByTime(260)
    expect(control.activeStrengthAction.value).toBe(action)

    document.dispatchEvent(createPointerEvent('pointermove', { pointerId: 7, clientX: 350 }))
    document.dispatchEvent(createPointerEvent('pointerup', { pointerId: 7, clientX: 350 }))

    expect(control.activeStrengthAction.value).toBeNull()
    expect(control.getActionStrength(action)).toBe(50)
    expect(control.consumeSuppressedStrengthClick(action)).toBe(true)
    expect(onCommit).toHaveBeenCalledWith(action, 100)
    expect(onCommit).toHaveBeenCalledTimes(1)

    control.dispose()
  })

  it('drags the visible strength range during the same long-press gesture', () => {
    const onCommit = vi.fn()
    const control = useFlowImageActionStrengthControl({ onCommit })
    const action = IMAGE_NODE_ACTION.styleVariation
    const { input, trigger } = bindRange(control, action, 35)
    stubRangeBounds(input, { left: 100, width: 200 })

    trigger.dispatchEvent(createPointerEvent('pointerdown', { pointerId: 7, clientX: 150 }))
    vi.advanceTimersByTime(260)

    expect(control.activeStrengthAction.value).toBe(action)
    expect(control.getActionStrength(action)).toBe(50)
    expect(input.value).toBe('50')

    document.dispatchEvent(createPointerEvent('pointermove', { pointerId: 7, clientX: 280 }))
    document.dispatchEvent(createPointerEvent('pointerup', { pointerId: 7, clientX: 280 }))

    expect(onCommit).toHaveBeenCalledWith(action, 90)
    expect(onCommit).toHaveBeenCalledTimes(1)
    expect(control.activeStrengthAction.value).toBeNull()
    expect(control.getActionStrength(action)).toBe(50)
    expect(control.consumeSuppressedStrengthClick(action)).toBe(true)

    control.dispose()
  })

  it('closes without committing when a long press ends before strength changes', () => {
    const onCommit = vi.fn()
    const onCancel = vi.fn()
    const control = useFlowImageActionStrengthControl({ onCommit, onCancel })
    const action = IMAGE_NODE_ACTION.hdUpscale
    const { input, trigger } = bindRange(control, action, 35)
    stubRangeBounds(input, { left: 100, width: 200 })

    trigger.dispatchEvent(createPointerEvent('pointerdown', { pointerId: 8, clientX: 150 }))
    vi.advanceTimersByTime(260)
    document.dispatchEvent(createPointerEvent('pointerup', { pointerId: 8, clientX: 150 }))

    expect(onCommit).not.toHaveBeenCalled()
    expect(onCancel).toHaveBeenCalledTimes(1)
    expect(control.activeStrengthAction.value).toBeNull()

    control.dispose()
  })

  it('clears stale strength click suppression on the next ordinary trigger press', () => {
    const onCommit = vi.fn()
    const control = useFlowImageActionStrengthControl({ onCommit })
    const action = IMAGE_NODE_ACTION.styleVariation
    const { input, trigger } = bindRange(control, action)
    stubRangeBounds(input, { left: 100, width: 200 })

    trigger.dispatchEvent(createPointerEvent('pointerdown', { pointerId: 7, clientX: 150 }))
    vi.advanceTimersByTime(260)
    document.dispatchEvent(createPointerEvent('pointermove', { pointerId: 7, clientX: 280 }))
    document.dispatchEvent(createPointerEvent('pointerup', { pointerId: 7, clientX: 280 }))

    trigger.dispatchEvent(createPointerEvent('pointerdown', { pointerId: 8, clientX: 150 }))

    expect(onCommit).toHaveBeenCalledWith(action, 90)
    expect(control.consumeSuppressedStrengthClick(action)).toBe(false)

    control.dispose()
  })

  it('ignores drag movement from a different pointer gesture', () => {
    const onCommit = vi.fn()
    const onCancel = vi.fn()
    const control = useFlowImageActionStrengthControl({ onCommit, onCancel })
    const action = IMAGE_NODE_ACTION.hdUpscale
    const { input, trigger } = bindRange(control, action, 35)
    stubRangeBounds(input, { left: 100, width: 200 })

    trigger.dispatchEvent(createPointerEvent('pointerdown', { pointerId: 8, clientX: 150 }))
    vi.advanceTimersByTime(260)
    document.dispatchEvent(createPointerEvent('pointermove', { pointerId: 99, clientX: 280 }))
    document.dispatchEvent(createPointerEvent('pointerup', { pointerId: 8, clientX: 150 }))

    expect(control.activeStrengthAction.value).toBeNull()
    expect(control.getActionStrength(action)).toBe(50)
    expect(onCommit).not.toHaveBeenCalled()
    expect(onCancel).toHaveBeenCalledTimes(1)

    control.dispose()
  })

  it('cancels an interrupted strength drag without committing', () => {
    const onCommit = vi.fn()
    const onCancel = vi.fn()
    const control = useFlowImageActionStrengthControl({ onCommit, onCancel })
    const action = IMAGE_NODE_ACTION.styleVariation
    const { input, trigger } = bindRange(control, action, 35)
    stubRangeBounds(input, { left: 100, width: 200 })

    trigger.dispatchEvent(createPointerEvent('pointerdown', { pointerId: 7, clientX: 150 }))
    vi.advanceTimersByTime(260)
    document.dispatchEvent(createPointerEvent('pointermove', { pointerId: 7, clientX: 280 }))
    document.dispatchEvent(createPointerEvent('pointercancel', { pointerId: 7, clientX: 280 }))

    expect(onCommit).not.toHaveBeenCalled()
    expect(onCancel).toHaveBeenCalledTimes(1)
    expect(control.activeStrengthAction.value).toBeNull()
    expect(control.getActionStrength(action)).toBe(50)

    control.dispose()
  })

  it('clears pending long press and suppression when hidden', () => {
    const onCommit = vi.fn()
    const control = useFlowImageActionStrengthControl({ onCommit })
    const action = IMAGE_NODE_ACTION.styleVariation
    const { trigger } = bindRange(control, action)

    trigger.dispatchEvent(createPointerEvent('pointerdown'))
    control.hideStrengthControl()
    vi.advanceTimersByTime(260)
    trigger.dispatchEvent(createPointerEvent('pointerup'))

    expect(control.activeStrengthAction.value).toBeNull()
    expect(control.consumeSuppressedStrengthClick(action)).toBe(false)
    expect(onCommit).not.toHaveBeenCalled()

    control.dispose()
  })
})
