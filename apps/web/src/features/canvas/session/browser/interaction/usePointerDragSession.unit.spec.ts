import { effectScope } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { usePointerDragSession, type PointerDragSessionContext, type PointerDragSessionController } from './usePointerDragSession'

type DragPayload = {
  id: string
}

type DragController = PointerDragSessionController<DragPayload>

function createPointerEvent(type: string, init: Partial<PointerEvent> = {}) {
  return new PointerEvent(type, {
    bubbles: true,
    pointerId: 1,
    clientX: 10,
    clientY: 20,
    pointerType: 'mouse',
    ...init,
  })
}

function dispatchPointerEvent(target: EventTarget, type: string, init: Partial<PointerEvent> = {}) {
  const event = createPointerEvent(type, init)
  target.dispatchEvent(event)
  return event
}

function beginDragFromTarget(
  controller: DragController,
  target: EventTarget,
  payload: DragPayload,
  init: Partial<PointerEvent> = {},
) {
  target.addEventListener(
    'pointerdown',
    (event) => {
      controller.beginDrag(event as PointerEvent, payload)
    },
    { once: true },
  )
  return dispatchPointerEvent(target, 'pointerdown', init)
}

function createCaptureTarget(hasCapture = true) {
  const target = document.createElement('div')
  target.setPointerCapture = vi.fn()
  target.releasePointerCapture = vi.fn()
  target.hasPointerCapture = vi.fn(() => hasCapture)
  return target
}

function createScopedDragController(
  options: Parameters<typeof usePointerDragSession<DragPayload>>[0],
) {
  const scope = effectScope()
  const controller = scope.run(() => usePointerDragSession<DragPayload>(options))
  if (!controller) throw new Error('expected drag controller')
  return { scope, controller }
}

describe('usePointerDragSession', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('captures the pointer, forwards movement, and ends on pointerup', () => {
    const payload = { id: 'drag-1' }
    const onStart = vi.fn()
    const onMove = vi.fn()
    const onEnd = vi.fn()
    const onCancel = vi.fn()
    const controller = usePointerDragSession<DragPayload>({ onStart, onMove, onEnd, onCancel })
    const target = createCaptureTarget()

    const startEvent = beginDragFromTarget(controller, target, payload, { pointerId: 7 })
    const moveEvent = dispatchPointerEvent(window, 'pointermove', {
      pointerId: 7,
      clientX: 24,
    })
    dispatchPointerEvent(window, 'pointermove', { pointerId: 8, clientX: 40 })
    const endEvent = dispatchPointerEvent(window, 'pointerup', { pointerId: 7 })

    expect(target.setPointerCapture).toHaveBeenCalledWith(7)
    expect(target.releasePointerCapture).toHaveBeenCalledWith(7)
    expect(controller.isDragging.value).toBe(false)
    const expectedStart: PointerDragSessionContext<DragPayload> = { event: startEvent, payload }
    expect(onStart).toHaveBeenCalledWith(expectedStart)
    expect(onMove).toHaveBeenCalledTimes(1)
    expect(onMove).toHaveBeenCalledWith({ event: moveEvent, payload })
    expect(onEnd).toHaveBeenCalledWith({ event: endEvent, payload })
    expect(onCancel).not.toHaveBeenCalled()
  })

  it('cancels on pointercancel and ignores repeated cleanup events', () => {
    const payload = { id: 'drag-1' }
    const onEnd = vi.fn()
    const onCancel = vi.fn()
    const controller = usePointerDragSession<DragPayload>({ onEnd, onCancel })
    const target = createCaptureTarget()

    beginDragFromTarget(controller, target, payload, { pointerId: 3 })
    const cancelEvent = dispatchPointerEvent(window, 'pointercancel', { pointerId: 3 })
    dispatchPointerEvent(window, 'pointerup', { pointerId: 3 })
    dispatchPointerEvent(window, 'pointercancel', { pointerId: 3 })
    dispatchPointerEvent(target, 'lostpointercapture', { pointerId: 3 })

    expect(controller.isDragging.value).toBe(false)
    expect(target.releasePointerCapture).toHaveBeenCalledTimes(1)
    expect(onCancel).toHaveBeenCalledTimes(1)
    expect(onCancel).toHaveBeenCalledWith({ event: cancelEvent, payload })
    expect(onEnd).not.toHaveBeenCalled()
  })

  it('cancels on lostpointercapture without releasing an already-lost capture', () => {
    const payload = { id: 'drag-1' }
    const onEnd = vi.fn()
    const onCancel = vi.fn()
    const controller = usePointerDragSession<DragPayload>({ onEnd, onCancel })
    const target = createCaptureTarget(false)

    beginDragFromTarget(controller, target, payload, { pointerId: 11 })
    const lostCaptureEvent = dispatchPointerEvent(target, 'lostpointercapture', { pointerId: 11 })
    dispatchPointerEvent(window, 'pointerup', { pointerId: 11 })

    expect(controller.isDragging.value).toBe(false)
    expect(target.releasePointerCapture).not.toHaveBeenCalled()
    expect(onCancel).toHaveBeenCalledTimes(1)
    expect(onCancel).toHaveBeenCalledWith({ event: lostCaptureEvent, payload })
    expect(onEnd).not.toHaveBeenCalled()
  })

  it('cancels the previous session when a new pointer drag starts', () => {
    const firstPayload = { id: 'drag-1' }
    const nextPayload = { id: 'drag-2' }
    const onCancel = vi.fn()
    const controller = usePointerDragSession<DragPayload>({ onCancel })
    const firstTarget = createCaptureTarget()
    const nextTarget = createCaptureTarget()

    beginDragFromTarget(controller, firstTarget, firstPayload, { pointerId: 21 })
    const nextEvent = beginDragFromTarget(controller, nextTarget, nextPayload, { pointerId: 22 })

    expect(firstTarget.releasePointerCapture).toHaveBeenCalledWith(21)
    expect(nextTarget.setPointerCapture).toHaveBeenCalledWith(22)
    expect(controller.isDragging.value).toBe(true)
    expect(onCancel).toHaveBeenCalledOnce()
    expect(onCancel).toHaveBeenCalledWith({ event: nextEvent, payload: firstPayload })
  })

  it('cleans up on scope disposal without duplicating later end or cancel callbacks', () => {
    const payload = { id: 'drag-1' }
    const onEnd = vi.fn()
    const onCancel = vi.fn()
    const target = createCaptureTarget()
    const { scope, controller } = createScopedDragController({ onEnd, onCancel })

    beginDragFromTarget(controller, target, payload, { pointerId: 5 })
    scope.stop()
    dispatchPointerEvent(window, 'pointerup', { pointerId: 5 })
    dispatchPointerEvent(window, 'pointercancel', { pointerId: 5 })

    expect(controller.isDragging.value).toBe(false)
    expect(target.releasePointerCapture).toHaveBeenCalledTimes(1)
    expect(onEnd).not.toHaveBeenCalled()
    expect(onCancel).not.toHaveBeenCalled()
  })

  it('works with targets that do not support pointer capture', () => {
    const payload = { id: 'drag-1' }
    const onMove = vi.fn()
    const onEnd = vi.fn()
    const controller = usePointerDragSession<DragPayload>({ onMove, onEnd })
    const target = new EventTarget()

    beginDragFromTarget(controller, target, payload, { pointerId: 13 })
    const moveEvent = dispatchPointerEvent(window, 'pointermove', { pointerId: 13 })
    const endEvent = dispatchPointerEvent(window, 'pointerup', { pointerId: 13 })

    expect(controller.isDragging.value).toBe(false)
    expect(onMove).toHaveBeenCalledWith({ event: moveEvent, payload })
    expect(onEnd).toHaveBeenCalledWith({ event: endEvent, payload })
  })

  it('proceeds captureless when setPointerCapture throws and still ends cleanly', () => {
    const payload = { id: 'drag-1' }
    const onMove = vi.fn()
    const onEnd = vi.fn()
    const controller = usePointerDragSession<DragPayload>({ onMove, onEnd })
    const target = createCaptureTarget()
    target.setPointerCapture = vi.fn(() => {
      throw new Error('capture unavailable')
    })
    target.releasePointerCapture = vi.fn(() => {
      throw new Error('capture already lost')
    })

    beginDragFromTarget(controller, target, payload, { pointerId: 17 })
    expect(controller.isDragging.value).toBe(true)
    const moveEvent = dispatchPointerEvent(window, 'pointermove', { pointerId: 17 })
    const endEvent = dispatchPointerEvent(window, 'pointerup', { pointerId: 17 })

    expect(controller.isDragging.value).toBe(false)
    expect(onMove).toHaveBeenCalledWith({ event: moveEvent, payload })
    expect(onEnd).toHaveBeenCalledWith({ event: endEvent, payload })
  })
})
