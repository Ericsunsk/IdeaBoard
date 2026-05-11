import type { ComponentPublicInstance } from 'vue'

export function useFlowStyleInnovateControl(options: { onCommit: (strength: number) => void }) {
  const { onCommit } = options
  const flowStyleInnovateStrength = ref(50)
  const flowShowStyleStrengthControl = ref(false)
  const flowStyleStrengthTrackRef = ref<HTMLElement | HTMLElement[] | null>(null)
  const flowSuppressStyleInnovateClick = ref(false)

  let flowStyleStrengthLongPressTimer: number | null = null
  let flowStyleStrengthDragging = false
  let flowStyleStrengthPointerY = 0

  function clearFlowStyleStrengthLongPressTimer() {
    if (!flowStyleStrengthLongPressTimer) return
    clearTimeout(flowStyleStrengthLongPressTimer)
    flowStyleStrengthLongPressTimer = null
  }

  function getFlowStyleStrengthTrackEl(): HTMLElement | null {
    if (Array.isArray(flowStyleStrengthTrackRef.value)) {
      return flowStyleStrengthTrackRef.value[0] ?? null
    }
    return flowStyleStrengthTrackRef.value
  }

  function setFlowStyleStrengthTrackRef(el: Element | ComponentPublicInstance | null) {
    const targetEl =
      el instanceof HTMLElement
        ? el
        : el && '$el' in el && (el as ComponentPublicInstance).$el instanceof HTMLElement
          ? (el as ComponentPublicInstance).$el
          : null
    flowStyleStrengthTrackRef.value = targetEl
  }

  function updateFlowStyleStrengthFromClientY(clientY: number) {
    const track = getFlowStyleStrengthTrackEl()
    if (!track) return
    const rect = track.getBoundingClientRect()
    if (rect.height <= 0) return
    const ratio = 1 - (clientY - rect.top) / rect.height
    const clamped = Math.max(0, Math.min(1, ratio))
    flowStyleInnovateStrength.value = Math.max(0, Math.min(100, Math.round(clamped * 100)))
  }

  function removeFlowStyleStrengthGlobalListeners() {
    window.removeEventListener('mousemove', onFlowStyleInnovateMouseMove, true)
    window.removeEventListener('mouseup', onFlowStyleInnovateMouseUp, true)
  }

  function addFlowStyleStrengthGlobalListeners() {
    window.addEventListener('mousemove', onFlowStyleInnovateMouseMove, true)
    window.addEventListener('mouseup', onFlowStyleInnovateMouseUp, true)
  }

  function beginFlowStyleStrengthDragging(clientY: number) {
    flowShowStyleStrengthControl.value = true
    flowStyleStrengthDragging = true
    flowStyleStrengthPointerY = clientY
    addFlowStyleStrengthGlobalListeners()
    void nextTick(() => updateFlowStyleStrengthFromClientY(flowStyleStrengthPointerY))
  }

  function hideFlowStyleStrengthControl() {
    flowShowStyleStrengthControl.value = false
    flowStyleStrengthDragging = false
    flowSuppressStyleInnovateClick.value = false
    clearFlowStyleStrengthLongPressTimer()
    removeFlowStyleStrengthGlobalListeners()
  }

  function onFlowStyleInnovateMouseDown(e: MouseEvent) {
    clearFlowStyleStrengthLongPressTimer()
    flowSuppressStyleInnovateClick.value = false
    flowStyleStrengthDragging = false
    flowStyleStrengthPointerY = e.clientY
    flowStyleStrengthLongPressTimer = window.setTimeout(() => {
      flowSuppressStyleInnovateClick.value = true
      beginFlowStyleStrengthDragging(flowStyleStrengthPointerY)
    }, 260)
    addFlowStyleStrengthGlobalListeners()
  }

  function onFlowStyleInnovateMouseMove(e: MouseEvent) {
    flowStyleStrengthPointerY = e.clientY
    if (!flowStyleStrengthDragging || !flowShowStyleStrengthControl.value) return
    updateFlowStyleStrengthFromClientY(e.clientY)
  }

  function onFlowStyleInnovateMouseUp() {
    clearFlowStyleStrengthLongPressTimer()
    removeFlowStyleStrengthGlobalListeners()
    if (!flowStyleStrengthDragging) return
    flowStyleStrengthDragging = false
    flowShowStyleStrengthControl.value = false
    onCommit(flowStyleInnovateStrength.value)
  }

  function onFlowStyleStrengthTrackMouseDown(e: MouseEvent) {
    beginFlowStyleStrengthDragging(e.clientY)
  }

  onBeforeUnmount(() => {
    hideFlowStyleStrengthControl()
  })

  return {
    flowStyleInnovateStrength,
    flowShowStyleStrengthControl,
    flowStyleStrengthTrackRef,
    flowSuppressStyleInnovateClick,
    setFlowStyleStrengthTrackRef,
    hideFlowStyleStrengthControl,
    onFlowStyleInnovateMouseDown,
    onFlowStyleInnovateMouseMove,
    onFlowStyleInnovateMouseUp,
    onFlowStyleStrengthTrackMouseDown,
  }
}
