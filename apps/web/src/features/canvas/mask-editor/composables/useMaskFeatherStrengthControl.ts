import { getCurrentInstance, nextTick, onBeforeUnmount, ref } from 'vue'
import {
  clampMaskFeatherStrength,
  MASK_EDITOR_DEFAULT_FEATHER_STRENGTH,
} from '../config/maskEditorConstants'

const DEFAULT_LONG_PRESS_MS = 260

type UseMaskFeatherStrengthControlOptions = {
  onApply: (strength: number) => void
  longPressMs?: number
}

export function useMaskFeatherStrengthControl(options: UseMaskFeatherStrengthControlOptions) {
  const strength = ref(MASK_EDITOR_DEFAULT_FEATHER_STRENGTH)
  const visible = ref(false)
  const trackRef = ref<HTMLElement | null>(null)
  const longPressMs = options.longPressMs ?? DEFAULT_LONG_PRESS_MS

  let longPressTimer: number | null = null
  let isDragging = false
  let pointerX = 0
  let shouldSuppressNextClick = false

  function clearLongPressTimer() {
    if (!longPressTimer) return
    clearTimeout(longPressTimer)
    longPressTimer = null
  }

  function updateStrengthFromClientX(clientX: number) {
    const track = trackRef.value
    if (!track) return
    const rect = track.getBoundingClientRect()
    if (rect.width <= 0) return
    strength.value = clampMaskFeatherStrength(((clientX - rect.left) / rect.width) * 100)
  }

  function addGlobalListeners() {
    window.addEventListener('mousemove', onGlobalMouseMove, true)
    window.addEventListener('mouseup', onGlobalMouseUp, true)
  }

  function removeGlobalListeners() {
    window.removeEventListener('mousemove', onGlobalMouseMove, true)
    window.removeEventListener('mouseup', onGlobalMouseUp, true)
  }

  function beginDragging(clientX: number) {
    visible.value = true
    isDragging = true
    pointerX = clientX
    addGlobalListeners()
    void nextTick(() => updateStrengthFromClientX(pointerX))
  }

  function onButtonMouseDown(event: MouseEvent) {
    if (event.button !== 0) return
    clearLongPressTimer()
    removeGlobalListeners()
    isDragging = false
    shouldSuppressNextClick = false
    pointerX = event.clientX
    longPressTimer = window.setTimeout(() => {
      shouldSuppressNextClick = true
      beginDragging(pointerX)
    }, longPressMs)
    addGlobalListeners()
  }

  function onButtonClick() {
    if (shouldSuppressNextClick) {
      shouldSuppressNextClick = false
      return
    }
    options.onApply(strength.value)
  }

  function onTrackMouseDown(event: MouseEvent) {
    clearLongPressTimer()
    shouldSuppressNextClick = true
    beginDragging(event.clientX)
  }

  function onGlobalMouseMove(event: MouseEvent) {
    pointerX = event.clientX
    if (!isDragging) return
    updateStrengthFromClientX(event.clientX)
  }

  function onGlobalMouseUp() {
    clearLongPressTimer()
    removeGlobalListeners()
    if (!isDragging) return
    isDragging = false
    visible.value = false
    options.onApply(strength.value)
  }

  function dispose() {
    clearLongPressTimer()
    removeGlobalListeners()
    isDragging = false
    visible.value = false
  }

  if (getCurrentInstance()) {
    onBeforeUnmount(dispose)
  }

  return {
    strength,
    visible,
    trackRef,
    onButtonMouseDown,
    onButtonClick,
    onTrackMouseDown,
    dispose,
  }
}
