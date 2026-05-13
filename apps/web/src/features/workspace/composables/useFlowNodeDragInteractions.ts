import type { Ref } from 'vue'

type UseFlowNodeDragInteractionsOptions = {
  flowHistory: { onNodeDragStart: () => void; onNodeDragStop: (onCommit: () => void) => void }
  duplicateModifierPressed: Ref<boolean>
  beginFlowNodeDragDuplicateTracking: (altPressed: boolean) => void
  finishFlowNodeDragDuplicateTracking: (altPressed: boolean) => void
  markDirty: () => void
  activeImageActionNodeId: { value: string | null }
  hideFlowStyleStrengthControl: () => void
}

export function useFlowNodeDragInteractions(options: UseFlowNodeDragInteractionsOptions) {
  function onNodeDragStart() {
    options.activeImageActionNodeId.value = null
    options.hideFlowStyleStrengthControl()
    options.flowHistory.onNodeDragStart()
    options.beginFlowNodeDragDuplicateTracking(options.duplicateModifierPressed.value)
  }

  function onNodeDragStop() {
    options.finishFlowNodeDragDuplicateTracking(options.duplicateModifierPressed.value)
    options.flowHistory.onNodeDragStop(() => {
      options.markDirty()
    })
  }

  return {
    onNodeDragStart,
    onNodeDragStop,
  }
}
