import { computed, ref } from 'vue'

export type FlowCanvasTool = 'select' | 'hand' | 'text'

export type CanvasInteractionKey =
  | 'move'
  | 'node-drag'
  | 'selection'
  | 'selection-drag'
  | 'connect'
  | 'edge-update'
  | 'message-resize'

export function useFlowCanvasInteractionState() {
  const activeTool = ref<FlowCanvasTool>('select')
  const isSpacePressed = ref(false)
  const isDuplicateModifierPressed = ref(false)
  const activeCanvasInteractions = ref<Set<CanvasInteractionKey>>(new Set())

  const isCanvasInteractionActive = computed(() => activeCanvasInteractions.value.size > 0)
  const effectiveTool = computed<FlowCanvasTool>(() => (isSpacePressed.value ? 'hand' : activeTool.value))

  function setActiveTool(tool: FlowCanvasTool) {
    activeTool.value = tool
  }

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

  function clearCanvasInteractions() {
    if (activeCanvasInteractions.value.size === 0) return
    activeCanvasInteractions.value = new Set()
  }

  return {
    activeTool,
    isSpacePressed,
    isDuplicateModifierPressed,
    activeCanvasInteractions,
    isCanvasInteractionActive,
    effectiveTool,
    setActiveTool,
    startCanvasInteraction,
    stopCanvasInteraction,
    clearCanvasInteractions,
  }
}
