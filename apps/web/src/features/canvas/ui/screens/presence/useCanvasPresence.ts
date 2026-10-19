import { onScopeDispose, watch, type Ref } from 'vue'
import {
  useCanvasPresenceSession,
  type CanvasPresenceSessionOptions,
} from '../../../session/collaboration'

type CanvasPresenceOptions = CanvasPresenceSessionOptions & {
  flow: CanvasPresenceSessionOptions['flow'] & {
    screenToFlowCoordinate: (point: { x: number; y: number }) => { x: number; y: number }
  }
  element: Ref<HTMLElement | null>
}

/** Adapts Canvas pointer events to the collaboration session. */
export function useCanvasPresence(options: CanvasPresenceOptions) {
  const { publishPointer, ...presence } = useCanvasPresenceSession(options)
  let pointerElement: HTMLElement | null = null

  function onPointerMove(event: PointerEvent): void {
    const point = options.flow.screenToFlowCoordinate({ x: event.clientX, y: event.clientY })
    if (!Number.isFinite(point.x) || !Number.isFinite(point.y)) return
    publishPointer(point, event.buttons !== 0)
  }

  function onPointerLeave(): void {
    publishPointer(null)
  }

  function detachPointerListeners(): void {
    pointerElement?.removeEventListener('pointermove', onPointerMove)
    pointerElement?.removeEventListener('pointerleave', onPointerLeave)
    pointerElement = null
  }

  watch(
    options.element,
    (next) => {
      if (next === pointerElement) return
      detachPointerListeners()
      pointerElement = next
      next?.addEventListener('pointermove', onPointerMove)
      next?.addEventListener('pointerleave', onPointerLeave)
    },
    { immediate: true, flush: 'sync' },
  )
  onScopeDispose(detachPointerListeners)
  return presence
}
