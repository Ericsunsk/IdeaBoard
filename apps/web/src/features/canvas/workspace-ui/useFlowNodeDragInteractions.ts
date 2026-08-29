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
    options.dismissTransientCanvasUi({ closeOutpaint: true })
    options.beginNodeDragTransaction(nodeIds)
    options.beginFlowNodeDragDuplicateTracking(options.duplicateModifierPressed.value)
  }

  function onNodeDragStop() {
    if (options.isNodeDragLocked?.()) return
    options.dismissTransientCanvasUi({
      closeOutpaint: true,
      clearNodePanel: false,
      hideStrength: false,
    })
    const isDuplicateMode = options.duplicateModifierPressed.value
    const commitToken = {}
    queueMicrotask(() => {
      if (pendingDragStopCommit !== commitToken) return
      pendingDragStopCommit = null
      try {
        options.commitNodeDragTransaction()
      } finally {
        // Always clear duplicate tracking, even when a graph commit fails.
        // Keeping this cleanup after commit preserves the duplicate position
        // calculation while preventing stale Alt-drag state from leaking.
        options.finishFlowNodeDragDuplicateTracking(isDuplicateMode)
      }
    })
    pendingDragStopCommit = commitToken
  }

  return {
    onNodeDragStart,
    onNodeDragStop,
  }
}
