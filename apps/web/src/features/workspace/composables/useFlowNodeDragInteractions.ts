import { onMounted, onBeforeUnmount, ref } from 'vue'
import type { CanvasInteractionKey } from './useFlowCanvasInteractionState'

type UseFlowNodeDragInteractionsOptions = {
  flowHistory: { onNodeDragStart: () => void; onNodeDragStop: (onCommit: () => void) => void }
  startFlowInteraction: (key: CanvasInteractionKey) => void
  stopFlowInteraction: (key: CanvasInteractionKey) => void
  beginFlowNodeDragDuplicateTracking: (altPressed: boolean) => void
  finishFlowNodeDragDuplicateTracking: (altPressed: boolean) => void
  ensureFlowEdgeVisuals: () => void
  markDirty: () => void
  activeMessageActionNodeId: { value: string | null }
  hideFlowStyleStrengthControl: () => void
}

export function useFlowNodeDragInteractions(options: UseFlowNodeDragInteractionsOptions) {
  const isAltPressed = ref(false)
  const isDraggingNode = ref(false)

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
    isDraggingNode.value = true
    options.startFlowInteraction('node-drag')
    options.activeMessageActionNodeId.value = null
    options.hideFlowStyleStrengthControl()
    options.flowHistory.onNodeDragStart()
    options.beginFlowNodeDragDuplicateTracking(isAltPressed.value)
  }

  function onNodeDragStop() {
    isDraggingNode.value = false
    options.stopFlowInteraction('node-drag')
    options.finishFlowNodeDragDuplicateTracking(isAltPressed.value)
    options.flowHistory.onNodeDragStop(() => {
      options.markDirty()
    })
    options.ensureFlowEdgeVisuals()
  }

  onMounted(() => {
    window.addEventListener('keydown', onWindowKeyDown)
    window.addEventListener('keyup', onWindowKeyUp)
  })

  onBeforeUnmount(() => {
    window.removeEventListener('keydown', onWindowKeyDown)
    window.removeEventListener('keyup', onWindowKeyUp)
  })

  return {
    isAltPressed,
    isDraggingNode,
    onNodeDragStart,
    onNodeDragStop,
  }
}
