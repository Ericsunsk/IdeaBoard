import {
  inject,
  onBeforeUnmount,
  onMounted,
  onScopeDispose,
  provide,
  ref,
  unref,
  watch,
  type InjectionKey,
  type MaybeRef,
} from 'vue'
import {
  createCanvasHandleProximityCoordinator,
  type CanvasHandleAnchorRect,
  type CanvasHandleProximityCoordinator,
  type CanvasHandleProximityDirection,
  type CanvasHandleProximityPoint,
} from './canvasHandleProximityCoordinator'

type UseCanvasHandleProximityOptions = {
  direction: CanvasHandleProximityDirection
  getAnchorRect: () => CanvasHandleAnchorRect | null
  enabled: MaybeRef<boolean>
}

type CanvasHandleProximitySurface = {
  coordinator: CanvasHandleProximityCoordinator
  requestRefresh: () => void
}

const COORDINATOR_KEY: InjectionKey<CanvasHandleProximitySurface> = Symbol(
  'canvas:handle-proximity-coordinator',
)

function installPointerAdapter(coordinator: CanvasHandleProximityCoordinator) {
  if (typeof window === 'undefined') {
    return {
      dispose: () => undefined,
      requestRefresh: coordinator.refresh,
    }
  }

  let evaluationFrame: number | null = null
  let pendingPointer: CanvasHandleProximityPoint | null = null
  let hasPendingPointer = false
  let isRefreshPending = false

  function evaluatePendingState() {
    evaluationFrame = null
    const shouldMovePointer = hasPendingPointer
    const pointer = pendingPointer
    const shouldRefresh = isRefreshPending
    hasPendingPointer = false
    isRefreshPending = false

    if (shouldMovePointer) {
      coordinator.movePointer(pointer)
    } else if (shouldRefresh) {
      coordinator.refresh()
    }
  }

  function scheduleEvaluation() {
    if (evaluationFrame !== null) return
    evaluationFrame = window.requestAnimationFrame(evaluatePendingState)
  }

  function schedulePointer(pointer: CanvasHandleProximityPoint | null) {
    pendingPointer = pointer
    hasPendingPointer = true
    scheduleEvaluation()
  }

  function requestRefresh() {
    isRefreshPending = true
    scheduleEvaluation()
  }

  function onPointerMove(event: PointerEvent) {
    schedulePointer(event.pointerType === 'touch' ? null : { x: event.clientX, y: event.clientY })
  }

  function onPointerLeave() {
    schedulePointer(null)
  }

  window.addEventListener('pointermove', onPointerMove, true)
  window.addEventListener('pointerleave', onPointerLeave, true)
  window.addEventListener('blur', onPointerLeave)

  return {
    requestRefresh,
    dispose: () => {
      window.removeEventListener('pointermove', onPointerMove, true)
      window.removeEventListener('pointerleave', onPointerLeave, true)
      window.removeEventListener('blur', onPointerLeave)
      if (evaluationFrame !== null) window.cancelAnimationFrame(evaluationFrame)
      evaluationFrame = null
      pendingPointer = null
      hasPendingPointer = false
      isRefreshPending = false
      coordinator.clear()
    },
  }
}

export function provideCanvasHandleProximityCoordinator(
  coordinator = createCanvasHandleProximityCoordinator(),
) {
  const pointerAdapter = installPointerAdapter(coordinator)
  provide(COORDINATOR_KEY, {
    coordinator,
    requestRefresh: pointerAdapter.requestRefresh,
  })
  onScopeDispose(pointerAdapter.dispose)
  return {
    requestRefresh: pointerAdapter.requestRefresh,
    setSuspended: coordinator.setSuspended,
  }
}

export function useCanvasHandleProximity(options: UseCanvasHandleProximityOptions) {
  const surface = inject(COORDINATOR_KEY)
  if (!surface) {
    throw new Error('Canvas handle proximity requires a Canvas surface coordinator')
  }
  const { coordinator } = surface

  const isActive = ref(false)
  const point = ref<CanvasHandleProximityPoint>({ x: 0, y: 0 })
  let unregister: (() => void) | null = null

  onMounted(() => {
    unregister = coordinator.register({
      direction: options.direction,
      getAnchorRect: options.getAnchorRect,
      isEnabled: () => unref(options.enabled),
      onStateChange: (state) => {
        isActive.value = state.isActive
        point.value = state.point
      },
    })
  })

  watch(
    () => unref(options.enabled),
    () => surface.requestRefresh(),
    { flush: 'sync' },
  )

  onBeforeUnmount(() => {
    unregister?.()
    unregister = null
  })

  return {
    isActive,
    point,
  }
}
