export type CanvasInteractionKey = 'mask-painting'

export function useFlowCanvasInteractionState() {
  const isMaskPainting = ref(false)
  const isCanvasInteractionActive = computed(() => isMaskPainting.value)

  function startCanvasInteraction(_key: CanvasInteractionKey) {
    isMaskPainting.value = true
  }

  function stopCanvasInteraction(_key: CanvasInteractionKey) {
    isMaskPainting.value = false
  }

  return {
    isCanvasInteractionActive,
    startCanvasInteraction,
    stopCanvasInteraction,
  }
}
