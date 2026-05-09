import { computed, ref } from 'vue'

export type CanvasInteractionKey =
  | 'move'
  | 'node-drag'
  | 'selection'
  | 'selection-drag'
  | 'connect'
  | 'edge-update'
  | 'mask-painting'
  | 'message-resize'

export function useFlowCanvasInteractionState() {
  const activeCanvasInteractions = ref<Set<CanvasInteractionKey>>(new Set())
  const isCanvasInteractionActive = computed(() => activeCanvasInteractions.value.size > 0)

  function startCanvasInteraction(key: CanvasInteractionKey) {
    const next = new Set(activeCanvasInteractions.value)
    next.add(key)
    activeCanvasInteractions.value = next
  }

  function stopCanvasInteraction(key: CanvasInteractionKey) {
    if (!activeCanvasInteractions.value.has(key)) return
    const next = new Set(activeCanvasInteractions.value)
    next.delete(key)
    activeCanvasInteractions.value = next
  }

  return {
    isCanvasInteractionActive,
    startCanvasInteraction,
    stopCanvasInteraction,
  }
}
