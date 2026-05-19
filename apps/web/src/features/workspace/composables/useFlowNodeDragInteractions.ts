import type { Ref } from 'vue'

type UseFlowNodeDragInteractionsOptions = {
  beginNodeDragTransaction: () => void
  commitNodeDragTransaction: () => void
  duplicateModifierPressed: Ref<boolean>
  beginFlowNodeDragDuplicateTracking: (altPressed: boolean) => void
  finishFlowNodeDragDuplicateTracking: (altPressed: boolean) => void
  activeImageActionNodeId: { value: string | null }
  activeImagePromptNodeId?: { value: string | null }
  activeFusionPromptNodeId?: { value: string | null }
  hideFlowStyleStrengthControl: () => void
}

export function useFlowNodeDragInteractions(options: UseFlowNodeDragInteractionsOptions) {
  function onNodeDragStart() {
    options.activeImageActionNodeId.value = null
    if (options.activeImagePromptNodeId) options.activeImagePromptNodeId.value = null
    if (options.activeFusionPromptNodeId) options.activeFusionPromptNodeId.value = null
    options.hideFlowStyleStrengthControl()
    options.beginNodeDragTransaction()
    options.beginFlowNodeDragDuplicateTracking(options.duplicateModifierPressed.value)
  }

  function onNodeDragStop() {
    options.finishFlowNodeDragDuplicateTracking(options.duplicateModifierPressed.value)
    options.commitNodeDragTransaction()
  }

  return {
    onNodeDragStart,
    onNodeDragStop,
  }
}
