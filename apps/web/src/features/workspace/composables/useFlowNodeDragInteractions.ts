import { onMounted, onBeforeUnmount, ref } from 'vue'

type UseFlowNodeDragInteractionsOptions = {
  flowHistory: { onNodeDragStart: () => void; onNodeDragStop: (onCommit: () => void) => void }
  beginFlowNodeDragDuplicateTracking: (altPressed: boolean) => void
  finishFlowNodeDragDuplicateTracking: (altPressed: boolean) => void
  markDirty: () => void
  activeMessageActionNodeId: { value: string | null }
  hideFlowStyleStrengthControl: () => void
}

export function useFlowNodeDragInteractions(options: UseFlowNodeDragInteractionsOptions) {
  const isAltPressed = ref(false)

  function onWindowKeyDown(event: KeyboardEvent) {
    if (event.key === 'Alt') {
      isAltPressed.value = true
    }
  }

  function onWindowKeyUp(event: KeyboardEvent) {
    if (event.key === 'Alt') {
      isAltPressed.value = false
    }
  }

  function onNodeDragStart() {
    options.activeMessageActionNodeId.value = null
    options.hideFlowStyleStrengthControl()
    options.flowHistory.onNodeDragStart()
    options.beginFlowNodeDragDuplicateTracking(isAltPressed.value)
  }

  function onNodeDragStop() {
    options.finishFlowNodeDragDuplicateTracking(isAltPressed.value)
    options.flowHistory.onNodeDragStop(() => {
      options.markDirty()
    })
  }

  function onWindowBlur() {
    isAltPressed.value = false
  }

  onMounted(() => {
    window.addEventListener('keydown', onWindowKeyDown)
    window.addEventListener('keyup', onWindowKeyUp)
    window.addEventListener('blur', onWindowBlur)
  })

  onBeforeUnmount(() => {
    window.removeEventListener('keydown', onWindowKeyDown)
    window.removeEventListener('keyup', onWindowKeyUp)
    window.removeEventListener('blur', onWindowBlur)
  })

  return {
    isAltPressed,
    onNodeDragStart,
    onNodeDragStop,
  }
}
