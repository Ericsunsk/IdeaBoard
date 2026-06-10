import type { ClearActiveNodePanelsOptions } from './useFlowActiveNodePanels'

export type DismissTransientWorkspaceUiOptions = {
  closeExpand?: boolean
  closeBoardHeader?: boolean
  closeContextMenu?: boolean
  clearNodePanels?: boolean | ClearActiveNodePanelsOptions
  hideStrength?: boolean
}

export type DismissTransientWorkspaceUi = (options?: DismissTransientWorkspaceUiOptions) => void

type UseFlowTransientUiDismissalOptions = {
  cancelFlowExpandMode: () => void
  closeBoardHeaderEditing: () => void
  closeNodeContextMenu: () => void
  clearActiveNodePanels: (options?: ClearActiveNodePanelsOptions) => void
  hideStrengthControl: () => void
}

export function useFlowTransientUiDismissal(options: UseFlowTransientUiDismissalOptions) {
  function dismissTransientWorkspaceUi(request: DismissTransientWorkspaceUiOptions = {}) {
    const clearNodePanels = request.clearNodePanels ?? true
    const hideStrength = request.hideStrength ?? true

    if (request.closeExpand) options.cancelFlowExpandMode()
    if (request.closeBoardHeader) options.closeBoardHeaderEditing()
    if (request.closeContextMenu) options.closeNodeContextMenu()
    if (clearNodePanels !== false) {
      options.clearActiveNodePanels(clearNodePanels === true ? undefined : clearNodePanels)
    }
    if (hideStrength) options.hideStrengthControl()
  }

  return {
    dismissTransientWorkspaceUi,
  }
}
