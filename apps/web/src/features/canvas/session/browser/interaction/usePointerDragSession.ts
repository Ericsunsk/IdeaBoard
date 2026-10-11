import { tryOnScopeDispose, useEventListener } from '@vueuse/core'
import { computed, shallowRef, type ComputedRef } from 'vue'

type PointerDragTarget = EventTarget & {
  setPointerCapture?: (pointerId: number) => void
  releasePointerCapture?: (pointerId: number) => void
  hasPointerCapture?: (pointerId: number) => boolean
}

export type PointerDragSessionContext<TPayload> = {
  event: PointerEvent
  payload: TPayload
}

export type UsePointerDragSessionOptions<TPayload> = {
  onStart?: (context: PointerDragSessionContext<TPayload>) => void
  onMove?: (context: PointerDragSessionContext<TPayload>) => void
  onEnd?: (context: PointerDragSessionContext<TPayload>) => void
  onCancel?: (context: PointerDragSessionContext<TPayload>) => void
}

type BeginPointerDragSessionOptions = {
  target?: PointerDragTarget | null
}

type PointerDragSession<TPayload> = {
  pointerId: number
  payload: TPayload
  target: PointerDragTarget | null
  stopListeners: (() => void) | null
}

export type PointerDragSessionController<TPayload = void> = {
  isDragging: ComputedRef<boolean>
  beginDrag: (
    event: PointerEvent,
    payload: TPayload,
    options?: BeginPointerDragSessionOptions,
  ) => void
  endDrag: (event?: PointerEvent) => PointerDragSession<TPayload> | null
  cancelDrag: (event?: PointerEvent) => PointerDragSession<TPayload> | null
}

function resolveWindow() {
  return typeof window === 'undefined' ? null : window
}

function resolveDragTarget(event: PointerEvent): PointerDragTarget | null {
  const target = event.currentTarget ?? event.target
  return target ? (target as PointerDragTarget) : null
}

export function usePointerDragSession<TPayload = void>(
  options: UsePointerDragSessionOptions<TPayload> = {},
): PointerDragSessionController<TPayload> {
  const activeSession = shallowRef<PointerDragSession<TPayload> | null>(null)
  const isDragging = computed(() => activeSession.value !== null)

  function createContext(
    event: PointerEvent,
    payload: TPayload,
  ): PointerDragSessionContext<TPayload> {
    return { event, payload }
  }

  function tryCapturePointer(target: PointerDragTarget | null, pointerId: number) {
    if (!target?.setPointerCapture) return
    try {
      target.setPointerCapture(pointerId)
    } catch {
      // Proceed captureless: window-level move/up listeners still drive the session.
    }
  }

  function releasePointerCapture(session: PointerDragSession<TPayload>) {
    const target = session.target
    if (!target?.releasePointerCapture) return
    try {
      if (target.hasPointerCapture && !target.hasPointerCapture(session.pointerId)) return
      target.releasePointerCapture(session.pointerId)
    } catch {
      // Capture was already lost or never acquired; session state is
      // already rolled back by clearSession, so there is nothing to do.
    }
  }

  function stopSessionListeners(session: PointerDragSession<TPayload>) {
    session.stopListeners?.()
    session.stopListeners = null
  }

  function clearSession(session: PointerDragSession<TPayload>) {
    if (activeSession.value !== session) return false
    activeSession.value = null
    stopSessionListeners(session)
    releasePointerCapture(session)
    return true
  }

  function finishDrag(
    event: PointerEvent,
    callback: ((context: PointerDragSessionContext<TPayload>) => void) | undefined,
  ) {
    const session = activeSession.value
    if (!session || event.pointerId !== session.pointerId) return null
    if (!clearSession(session)) return null
    callback?.(createContext(event, session.payload))
    return session
  }

  function cancelActiveDrag(event: PointerEvent) {
    const session = activeSession.value
    if (!session) return null
    if (!clearSession(session)) return null
    options.onCancel?.(createContext(event, session.payload))
    return session
  }

  function onWindowPointerMove(event: PointerEvent) {
    const session = activeSession.value
    if (!session || event.pointerId !== session.pointerId) return
    options.onMove?.(createContext(event, session.payload))
  }

  function onWindowPointerUp(event: PointerEvent) {
    finishDrag(event, options.onEnd)
  }

  function onWindowPointerCancel(event: PointerEvent) {
    finishDrag(event, options.onCancel)
  }

  function onLostPointerCapture(event: PointerEvent) {
    finishDrag(event, options.onCancel)
  }

  function addSessionListeners(session: PointerDragSession<TPayload>) {
    const stopPointerMove = useEventListener<PointerEvent>(
      resolveWindow(),
      'pointermove',
      onWindowPointerMove,
      true,
    )
    const stopPointerUp = useEventListener<PointerEvent>(
      resolveWindow(),
      'pointerup',
      onWindowPointerUp,
      true,
    )
    const stopPointerCancel = useEventListener<PointerEvent>(
      resolveWindow(),
      'pointercancel',
      onWindowPointerCancel,
      true,
    )
    const stopLostPointerCapture = useEventListener<PointerEvent>(
      session.target,
      'lostpointercapture',
      onLostPointerCapture,
    )

    session.stopListeners = () => {
      stopPointerMove()
      stopPointerUp()
      stopPointerCancel()
      stopLostPointerCapture()
    }
  }

  function beginDrag(
    event: PointerEvent,
    payload: TPayload,
    beginOptions: BeginPointerDragSessionOptions = {},
  ) {
    cancelActiveDrag(event)

    const session: PointerDragSession<TPayload> = {
      pointerId: event.pointerId,
      payload,
      target: beginOptions.target ?? resolveDragTarget(event),
      stopListeners: null,
    }

    activeSession.value = session
    addSessionListeners(session)
    tryCapturePointer(session.target, event.pointerId)
    options.onStart?.(createContext(event, payload))
  }

  function endDrag(event?: PointerEvent) {
    const session = activeSession.value
    if (!session) return null
    if (event && event.pointerId !== session.pointerId) return null
    if (!clearSession(session)) return null
    if (event) options.onEnd?.(createContext(event, session.payload))
    return session
  }

  function cancelDrag(event?: PointerEvent) {
    const session = activeSession.value
    if (!session) return null
    if (event && event.pointerId !== session.pointerId) return null
    if (!clearSession(session)) return null
    if (event) options.onCancel?.(createContext(event, session.payload))
    return session
  }

  // Intentional `tryOnScopeDispose`: this drag session is a reusable utility
  // that stays safe when instantiated outside a component scope.
  tryOnScopeDispose(() => {
    cancelDrag()
  })

  return {
    isDragging,
    beginDrag,
    endDrag,
    cancelDrag,
  }
}
