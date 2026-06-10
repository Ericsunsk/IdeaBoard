import type { Ref } from 'vue'
import type { DismissTransientWorkspaceUi } from './useFlowTransientUiDismissal'

type UseFlowNodeDragInteractionsOptions = {
  beginNodeDragTransaction: () => void
  commitNodeDragTransaction: () => void
  duplicateModifierPressed: Ref<boolean>
  beginFlowNodeDragDuplicateTracking: (altPressed: boolean) => void
  finishFlowNodeDragDuplicateTracking: (altPressed: boolean) => void
  dismissTransientWorkspaceUi: DismissTransientWorkspaceUi
}

export function useFlowNodeDragInteractions(options: UseFlowNodeDragInteractionsOptions) {
  function onNodeDragStart() {
    options.dismissTransientWorkspaceUi({ closeExpand: true })
    options.beginNodeDragTransaction()
    options.beginFlowNodeDragDuplicateTracking(options.duplicateModifierPressed.value)
  }

  function onNodeDragStop() {
    options.dismissTransientWorkspaceUi({
      closeExpand: true,
      clearNodePanels: false,
      hideStrength: false,
    })
    options.finishFlowNodeDragDuplicateTracking(options.duplicateModifierPressed.value)
    options.commitNodeDragTransaction()
  }

  return {
    onNodeDragStart,
    onNodeDragStop,
  }
}
