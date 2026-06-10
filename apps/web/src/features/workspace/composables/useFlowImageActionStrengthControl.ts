import {
  computed,
  nextTick,
  onBeforeUnmount,
  reactive,
  ref,
  type ComponentPublicInstance,
} from 'vue'
import {
  DEFAULT_IMAGE_ACTION_STRENGTH,
  DEFAULT_IMAGE_ACTION_STRENGTHS,
  type ImageStrengthAction,
} from '../types/imageActions'

export function useFlowImageActionStrengthControl(options: {
  onCommit: (action: ImageStrengthAction, strength: number) => void
}) {
  const { onCommit } = options
  const activeStrengthAction = ref<ImageStrengthAction | null>(null)
  const strengths = reactive<Record<ImageStrengthAction, number>>({
    ...DEFAULT_IMAGE_ACTION_STRENGTHS,
  })
  const strengthTrackRef = ref<HTMLElement | HTMLElement[] | null>(null)
  const suppressClickAction = ref<ImageStrengthAction | null>(null)

  let longPressTimer: number | null = null
  let draggingAction: ImageStrengthAction | null = null
  let pointerX = 0

  const activeStrengthValue = computed(() =>
    activeStrengthAction.value
      ? strengths[activeStrengthAction.value]
      : DEFAULT_IMAGE_ACTION_STRENGTH,
  )

  function clearLongPressTimer() {
    if (!longPressTimer) return
    clearTimeout(longPressTimer)
    longPressTimer = null
  }

  function getTrackEl(): HTMLElement | null {
    if (Array.isArray(strengthTrackRef.value)) return strengthTrackRef.value[0] ?? null
    return strengthTrackRef.value
  }

  function setStrengthTrackRef(el: Element | ComponentPublicInstance | null) {
    const targetEl =
      el instanceof HTMLElement
        ? el
        : el && '$el' in el && (el as ComponentPublicInstance).$el instanceof HTMLElement
          ? (el as ComponentPublicInstance).$el
          : null
    strengthTrackRef.value = targetEl
  }

  function updateStrengthFromClientX(action: ImageStrengthAction, clientX: number) {
    const track = getTrackEl()
    if (!track) return
    const rect = track.getBoundingClientRect()
    if (rect.width <= 0) return
    const ratio = (clientX - rect.left) / rect.width
    const clamped = Math.max(0, Math.min(1, ratio))
    strengths[action] = Math.max(0, Math.min(100, Math.round(clamped * 100)))
  }

  function removeGlobalListeners() {
    window.removeEventListener('mousemove', onMouseMove, true)
    window.removeEventListener('mouseup', onMouseUp, true)
  }

  function addGlobalListeners() {
    window.addEventListener('mousemove', onMouseMove, true)
    window.addEventListener('mouseup', onMouseUp, true)
  }

  function beginDragging(action: ImageStrengthAction, clientX: number) {
    activeStrengthAction.value = action
    draggingAction = action
    pointerX = clientX
    addGlobalListeners()
    void nextTick(() => updateStrengthFromClientX(action, pointerX))
  }

  function hideStrengthControl() {
    activeStrengthAction.value = null
    draggingAction = null
    suppressClickAction.value = null
    clearLongPressTimer()
    removeGlobalListeners()
  }

  function onActionMouseDown(action: ImageStrengthAction, e: MouseEvent) {
    clearLongPressTimer()
    suppressClickAction.value = null
    draggingAction = null
    pointerX = e.clientX
    longPressTimer = window.setTimeout(() => {
      suppressClickAction.value = action
      beginDragging(action, pointerX)
    }, 260)
    addGlobalListeners()
  }

  function onMouseMove(e: MouseEvent) {
    pointerX = e.clientX
    if (!draggingAction || activeStrengthAction.value !== draggingAction) return
    updateStrengthFromClientX(draggingAction, e.clientX)
  }

  function onMouseUp() {
    clearLongPressTimer()
    removeGlobalListeners()
    if (!draggingAction) return
    const action = draggingAction
    draggingAction = null
    activeStrengthAction.value = null
    onCommit(action, strengths[action])
  }

  function onStrengthTrackMouseDown(e: MouseEvent) {
    const action = activeStrengthAction.value
    if (!action) return
    beginDragging(action, e.clientX)
  }

  function getActionStrength(action: ImageStrengthAction): number {
    return strengths[action]
  }

  onBeforeUnmount(() => {
    hideStrengthControl()
  })

  return {
    activeStrengthAction,
    activeStrengthValue,
    suppressClickAction,
    setStrengthTrackRef,
    hideStrengthControl,
    getActionStrength,
    onActionMouseDown,
    onStrengthTrackMouseDown,
  }
}
