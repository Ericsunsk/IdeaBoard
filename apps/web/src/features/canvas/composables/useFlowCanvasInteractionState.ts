import { computed, ref } from 'vue'

export type CanvasInteractionKey = 'image-mask-editor' | 'flow-viewport' | 'node-drag'

const CANVAS_LOCKING_INTERACTIONS = new Set<CanvasInteractionKey>(['image-mask-editor'])

export function useFlowCanvasInteractionState() {
  const activeInteractions = ref(new Set<CanvasInteractionKey>())
  const isCanvasInteractionActive = computed(() => activeInteractions.value.size > 0)
  const isCanvasInteractionLocked = computed(() => {
    for (const key of activeInteractions.value) {
      if (CANVAS_LOCKING_INTERACTIONS.has(key)) return true
    }
    return false
  })

  function startCanvasInteraction(key: CanvasInteractionKey) {
    const nextInteractions = new Set(activeInteractions.value)
    nextInteractions.add(key)
    activeInteractions.value = nextInteractions
  }

  function stopCanvasInteraction(key: CanvasInteractionKey) {
    const nextInteractions = new Set(activeInteractions.value)
    nextInteractions.delete(key)
    activeInteractions.value = nextInteractions
  }

  return {
    isCanvasInteractionActive,
    isCanvasInteractionLocked,
    startCanvasInteraction,
    stopCanvasInteraction,
  }
}
