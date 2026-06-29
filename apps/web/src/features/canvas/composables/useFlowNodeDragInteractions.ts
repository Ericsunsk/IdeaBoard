import type { Ref } from 'vue'
import type { DismissTransientCanvasUi } from './useFlowTransientUiDismissal'

type UseFlowNodeDragInteractionsOptions = {
  beginNodeDragTransaction: () => void
  commitNodeDragTransaction: () => void
  duplicateModifierPressed: Ref<boolean>
  beginFlowNodeDragDuplicateTracking: (altPressed: boolean) => void
  finishFlowNodeDragDuplicateTracking: (altPressed: boolean) => void
  dismissTransientCanvasUi: DismissTransientCanvasUi
}

export function useFlowNodeDragInteractions(options: UseFlowNodeDragInteractionsOptions) {
  function onNodeDragStart() {
    options.dismissTransientCanvasUi({ closeExpand: true })
    options.beginNodeDragTransaction()
    options.beginFlowNodeDragDuplicateTracking(options.duplicateModifierPressed.value)
  }

  function onNodeDragStop() {
    options.dismissTransientCanvasUi({
      closeExpand: true,
      clearNodePanel: false,
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
