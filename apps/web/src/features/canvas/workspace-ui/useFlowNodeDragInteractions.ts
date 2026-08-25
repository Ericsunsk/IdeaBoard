import type { Ref } from 'vue'
import type { DismissTransientCanvasUi } from './useFlowTransientUiDismissal'

type UseFlowNodeDragInteractionsOptions = {
  beginNodeDragTransaction: (nodeIds: readonly string[]) => void
  commitNodeDragTransaction: () => void
  duplicateModifierPressed: Ref<boolean>
  beginFlowNodeDragDuplicateTracking: (altPressed: boolean) => void
  finishFlowNodeDragDuplicateTracking: (altPressed: boolean) => void
  dismissTransientCanvasUi: DismissTransientCanvasUi
  isNodeDragLocked?: () => boolean
}

export function useFlowNodeDragInteractions(options: UseFlowNodeDragInteractionsOptions) {
  let pendingDragStopCommit: object | null = null

  function onNodeDragStart(nodeIds: readonly string[]) {
    if (options.isNodeDragLocked?.()) return
    pendingDragStopCommit = null
    options.dismissTransientCanvasUi({ closeExpand: true })
    options.beginNodeDragTransaction(nodeIds)
    options.beginFlowNodeDragDuplicateTracking(options.duplicateModifierPressed.value)
  }

  function onNodeDragStop() {
    if (options.isNodeDragLocked?.()) return
    options.dismissTransientCanvasUi({
      closeExpand: true,
      clearNodePanel: false,
      hideStrength: false,
    })
    options.finishFlowNodeDragDuplicateTracking(options.duplicateModifierPressed.value)
    const commitToken = {}
    queueMicrotask(() => {
      if (pendingDragStopCommit !== commitToken) return
      pendingDragStopCommit = null
      options.commitNodeDragTransaction()
    })
    pendingDragStopCommit = commitToken
  }

  return {
    onNodeDragStart,
    onNodeDragStop,
  }
}
