import type {
  DismissTransientCanvasUi,
  DismissTransientCanvasUiOptions,
} from '../../session/flow-adapter'

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
    const clearNodePanel = request.clearNodePanel ?? true
    const hideStrength = request.hideStrength ?? true

    if (request.closeOutpaint) options.cancelFlowOutpaintMode()
    if (request.closeBoardHeader) options.closeBoardHeaderEditing()
    if (request.closeContextMenu) options.closeNodeContextMenu()
    if (clearNodePanel !== false) options.clearActiveNodePanel()
    if (hideStrength) options.hideStrengthControl()
  }

  return {
    dismissTransientCanvasUi,
  }
}
