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

const COORDINATOR_KEY: InjectionKey<CanvasHandleProximityCoordinator> = Symbol(
  'canvas:handle-proximity-coordinator',
)

function installPointerAdapter(coordinator: CanvasHandleProximityCoordinator) {
  if (typeof window === 'undefined') return () => undefined

  let evaluationFrame: number | null = null
  let pendingPointer: CanvasHandleProximityPoint | null = null

  function evaluatePendingPointer() {
    evaluationFrame = null
    coordinator.movePointer(pendingPointer)
  }

  function schedule(pointer: CanvasHandleProximityPoint | null) {
    pendingPointer = pointer
    if (evaluationFrame !== null) return
    evaluationFrame = window.requestAnimationFrame(evaluatePendingPointer)
  }

  function onPointerMove(event: PointerEvent) {
    schedule(event.pointerType === 'touch' ? null : { x: event.clientX, y: event.clientY })
  }

  function onPointerLeave() {
    schedule(null)
  }

  window.addEventListener('pointermove', onPointerMove, true)
  window.addEventListener('pointerleave', onPointerLeave, true)
  window.addEventListener('blur', onPointerLeave)

  return () => {
    window.removeEventListener('pointermove', onPointerMove, true)
    window.removeEventListener('pointerleave', onPointerLeave, true)
    window.removeEventListener('blur', onPointerLeave)
    if (evaluationFrame !== null) window.cancelAnimationFrame(evaluationFrame)
    evaluationFrame = null
    pendingPointer = null
    coordinator.clear()
  }
}

export function provideCanvasHandleProximityCoordinator(
  coordinator = createCanvasHandleProximityCoordinator(),
) {
  const disposePointerAdapter = installPointerAdapter(coordinator)
  provide(COORDINATOR_KEY, coordinator)
  onScopeDispose(disposePointerAdapter)
  return coordinator
}

export function useCanvasHandleProximity(options: UseCanvasHandleProximityOptions) {
  const coordinator = inject(COORDINATOR_KEY)
  if (!coordinator) {
    throw new Error('Canvas handle proximity requires a Canvas surface coordinator')
  }

  const isActive = ref(false)
  const isReturning = ref(false)
  const point = ref<CanvasHandleProximityPoint>({ x: 0, y: 0 })
  let unregister: (() => void) | null = null
  let isUnmounting = false

  onMounted(() => {
    unregister = coordinator.register({
      direction: options.direction,
      getAnchorRect: options.getAnchorRect,
      isEnabled: () => unref(options.enabled),
      onStateChange: (state) => {
        if (isUnmounting) return
        isActive.value = state.isActive
        isReturning.value = state.isReturning
        point.value = state.point
      },
    })
  })

  watch(
    () => unref(options.enabled),
    () => coordinator.refresh(),
    { flush: 'sync' },
  )

  onBeforeUnmount(() => {
    isUnmounting = true
    unregister?.()
    unregister = null
    isActive.value = false
    isReturning.value = false
    point.value = { x: 0, y: 0 }
  })

  return {
    isActive,
    isReturning,
    point,
    refreshCoordinator: coordinator.refresh,
  }
}
