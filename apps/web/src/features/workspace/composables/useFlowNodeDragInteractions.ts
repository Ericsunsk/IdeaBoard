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
  cancelFlowExpandMode?: () => void
  hideStrengthControl: () => void
}

export function useFlowNodeDragInteractions(options: UseFlowNodeDragInteractionsOptions) {
  function onNodeDragStart() {
    options.cancelFlowExpandMode?.()
    options.activeImageActionNodeId.value = null
    if (options.activeImagePromptNodeId) options.activeImagePromptNodeId.value = null
    if (options.activeFusionPromptNodeId) options.activeFusionPromptNodeId.value = null
    options.hideStrengthControl()
    options.beginNodeDragTransaction()
    options.beginFlowNodeDragDuplicateTracking(options.duplicateModifierPressed.value)
  }

  function onNodeDragStop() {
    options.cancelFlowExpandMode?.()
    options.finishFlowNodeDragDuplicateTracking(options.duplicateModifierPressed.value)
    options.commitNodeDragTransaction()
  }

  return {
    onNodeDragStart,
    onNodeDragStop,
  }
}
