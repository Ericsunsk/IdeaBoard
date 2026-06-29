import { computed, ref } from 'vue'

export type CanvasInteractionKey = 'image-mask-editor'

export function useFlowCanvasInteractionState() {
  const isImageMaskEditor = ref(false)
  const isCanvasInteractionActive = computed(() => isImageMaskEditor.value)

  function startCanvasInteraction(_key: CanvasInteractionKey) {
    isImageMaskEditor.value = true
  }

  function stopCanvasInteraction(_key: CanvasInteractionKey) {
    isImageMaskEditor.value = false
  }

  return {
    isCanvasInteractionActive,
    startCanvasInteraction,
    stopCanvasInteraction,
  }
}
