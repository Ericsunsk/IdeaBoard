import type {
  DismissTransientCanvasUi,
  DismissTransientCanvasUiOptions,
} from '../../../session/flow-adapter'

export type { DismissTransientCanvasUi, DismissTransientCanvasUiOptions }

type UseFlowTransientUiDismissalOptions = {
  cancelFlowOutpaintMode: () => void
  closeBoardHeaderEditing: () => void
  closeNodeContextMenu: () => void
  clearActiveNodePanel: () => void
  hideStrengthControl: () => void
}

export function useFlowTransientUiDismissal(options: UseFlowTransientUiDismissalOptions) {
  function dismissTransientCanvasUi(request: DismissTransientCanvasUiOptions = {}) {
    const shouldClearNodePanel = request.shouldClearNodePanel ?? true
    const shouldHideStrength = request.shouldHideStrength ?? true

    if (request.shouldCloseOutpaint) options.cancelFlowOutpaintMode()
    if (request.shouldCloseBoardHeader) options.closeBoardHeaderEditing()
    if (request.shouldCloseContextMenu) options.closeNodeContextMenu()
    if (shouldClearNodePanel !== false) options.clearActiveNodePanel()
    if (shouldHideStrength) options.hideStrengthControl()
  }

  return {
    dismissTransientCanvasUi,
  }
}
