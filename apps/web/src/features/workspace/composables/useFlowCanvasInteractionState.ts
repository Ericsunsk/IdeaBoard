export type CanvasInteractionKey = 'mask-painting' | 'message-resize'

export function useFlowCanvasInteractionState() {
  const isMaskPainting = ref(false)
  const isMessageResizing = ref(false)
  const isCanvasInteractionActive = computed(() => isMaskPainting.value || isMessageResizing.value)

  function startCanvasInteraction(key: CanvasInteractionKey) {
    if (key === 'mask-painting') {
      isMaskPainting.value = true
      return
    }
    isMessageResizing.value = true
  }

  function stopCanvasInteraction(key: CanvasInteractionKey) {
    if (key === 'mask-painting') {
      isMaskPainting.value = false
      return
    }
    isMessageResizing.value = false
  }

  return {
    isCanvasInteractionActive,
    startCanvasInteraction,
    stopCanvasInteraction,
  }
}
