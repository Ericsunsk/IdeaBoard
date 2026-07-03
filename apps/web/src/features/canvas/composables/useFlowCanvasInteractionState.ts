import { computed, ref } from 'vue'

export type CanvasInteractionKey =
  | 'image-editor-raster'
  | 'image-mask-editor'
  | 'flow-viewport'
  | 'viewport-pan'
  | 'node-drag'

const CANVAS_LOCKING_INTERACTIONS = new Set<CanvasInteractionKey>([
  'image-editor-raster',
  'image-mask-editor',
])

export function useFlowCanvasInteractionState() {
  const activeInteractions = ref(new Set<CanvasInteractionKey>())
  const isCanvasInteractionActive = computed(() => activeInteractions.value.size > 0)
  const isViewportPanInteractionActive = computed(() =>
    activeInteractions.value.has('viewport-pan'),
  )
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
    isViewportPanInteractionActive,
    isCanvasInteractionLocked,
    startCanvasInteraction,
    stopCanvasInteraction,
  }
}
