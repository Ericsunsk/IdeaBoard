import {
  inject,
  onBeforeUnmount,
  onMounted,
  onScopeDispose,
  provide,
  ref,
  toValue,
  unref,
  watch,
  type InjectionKey,
  type MaybeRef,
  type MaybeRefOrGetter,
} from 'vue'
import {
  createCanvasHandleProximityCoordinator,
  type CanvasHandleProximityGeometry,
  type CanvasHandleProximityCoordinator,
  type CanvasHandleProximityDirection,
  type CanvasHandleProximityPoint,
} from './canvasHandleProximityCoordinator'

type UseCanvasHandleProximityOptions = {
  nodeId: MaybeRefOrGetter<string>
  direction: CanvasHandleProximityDirection
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
  window.addEventListener('resize', requestRefresh)
  window.addEventListener('scroll', requestRefresh, true)

  return {
    requestRefresh,
    dispose: () => {
      window.removeEventListener('pointermove', onPointerMove, true)
      window.removeEventListener('pointerleave', onPointerLeave, true)
      window.removeEventListener('blur', onPointerLeave)
      window.removeEventListener('resize', requestRefresh)
      window.removeEventListener('scroll', requestRefresh, true)
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
  geometry: CanvasHandleProximityGeometry,
  coordinator = createCanvasHandleProximityCoordinator(geometry),
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

  function registerCurrentNode() {
    unregister?.()
    unregister = null
    const nodeId = toValue(options.nodeId)
    unregister = coordinator.register({
      nodeId,
      direction: options.direction,
      isEnabled: () => unref(options.enabled),
      onStateChange: (state) => {
        isActive.value = state.isActive
        point.value = state.point
      },
    })
  }

  onMounted(() => {
    registerCurrentNode()
  })

  watch(
    () => toValue(options.nodeId),
    (nodeId, previousNodeId) => {
      if (nodeId === previousNodeId) return
      // Vue Flow may reuse a rail instance for a different node id; the
      // proximity registration captures the id once, so re-register to keep
      // geometry resolution pointed at the current node.
      if (!unregister) return
      registerCurrentNode()
      surface.requestRefresh()
    },
  )

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
