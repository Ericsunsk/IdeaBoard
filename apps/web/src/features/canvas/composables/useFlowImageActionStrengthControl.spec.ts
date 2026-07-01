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

function dispatchRangeInput(input: HTMLInputElement, value: number) {
  input.value = `${value}`
  input.dispatchEvent(new Event('input', { bubbles: true }))
}

function bindRange(control: StrengthControl, action: ImageStrengthAction, value = 35) {
  const input = createRange(value)
  input.addEventListener('pointerdown', (event) =>
    control.onStrengthRangePointerDown(action, event),
  )
  input.addEventListener('input', (event) => control.onStrengthInput(action, event))
  input.addEventListener('change', (event) => control.onStrengthChange(action, event))
  input.addEventListener('pointerup', (event) => control.onStrengthPointerUp(action, event))
  control.setStrengthRangeRef(action, input)
  return input
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
    const input = bindRange(control, action)

    input.dispatchEvent(createPointerEvent('pointerdown'))
    vi.advanceTimersByTime(259)
    input.dispatchEvent(createPointerEvent('pointerup'))

    expect(control.activeStrengthAction.value).toBeNull()
    expect(control.suppressClickAction.value).toBeNull()
    expect(onCommit).not.toHaveBeenCalled()

    control.dispose()
  })

  it('updates strength with the native range and commits on pointerup', () => {
    const onCommit = vi.fn()
    const control = useFlowImageActionStrengthControl({ onCommit })
    const action = IMAGE_NODE_ACTION.styleVariation
    const input = bindRange(control, action)

    input.dispatchEvent(createPointerEvent('pointerdown'))
    vi.advanceTimersByTime(260)
    expect(control.activeStrengthAction.value).toBe(action)

    dispatchRangeInput(input, 80)
    input.dispatchEvent(createPointerEvent('pointerup'))

    expect(control.activeStrengthAction.value).toBeNull()
    expect(control.getActionStrength(action)).toBe(80)
    expect(control.suppressClickAction.value).toBe(action)
    expect(onCommit).toHaveBeenCalledWith(action, 80)
    expect(onCommit).toHaveBeenCalledTimes(1)

    control.dispose()
  })

  it('commits keyboard range edits through change', () => {
    const onCommit = vi.fn()
    const control = useFlowImageActionStrengthControl({ onCommit })
    const action = IMAGE_NODE_ACTION.hdUpscale
    const input = bindRange(control, action, 35)

    input.dispatchEvent(createPointerEvent('pointerdown'))
    vi.advanceTimersByTime(260)
    dispatchRangeInput(input, 62)
    input.dispatchEvent(new Event('change', { bubbles: true }))

    expect(control.activeStrengthAction.value).toBeNull()
    expect(control.getActionStrength(action)).toBe(62)
    expect(onCommit).toHaveBeenCalledWith(action, 62)

    control.dispose()
  })

  it('clears pending long press and suppression when hidden', () => {
    const onCommit = vi.fn()
    const control = useFlowImageActionStrengthControl({ onCommit })
    const action = IMAGE_NODE_ACTION.styleVariation
    const input = bindRange(control, action)

    input.dispatchEvent(createPointerEvent('pointerdown'))
    control.hideStrengthControl()
    vi.advanceTimersByTime(260)
    input.dispatchEvent(createPointerEvent('pointerup'))

    expect(control.activeStrengthAction.value).toBeNull()
    expect(control.suppressClickAction.value).toBeNull()
    expect(onCommit).not.toHaveBeenCalled()

    control.dispose()
  })
})
