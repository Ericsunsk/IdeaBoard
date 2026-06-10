import { describe, expect, it, vi } from 'vitest'
import { useFlowTransientUiDismissal } from './useFlowTransientUiDismissal'

function createHarness() {
  const cancelFlowExpandMode = vi.fn()
  const closeBoardHeaderEditing = vi.fn()
  const closeNodeContextMenu = vi.fn()
  const clearActiveNodePanels = vi.fn()
  const hideStrengthControl = vi.fn()

  const { dismissTransientWorkspaceUi } = useFlowTransientUiDismissal({
    cancelFlowExpandMode,
    closeBoardHeaderEditing,
    closeNodeContextMenu,
    clearActiveNodePanels,
    hideStrengthControl,
  })

  return {
    dismissTransientWorkspaceUi,
    cancelFlowExpandMode,
    closeBoardHeaderEditing,
    closeNodeContextMenu,
    clearActiveNodePanels,
    hideStrengthControl,
  }
}

describe('useFlowTransientUiDismissal', () => {
  it('clears node panels and strength control by default', () => {
    const harness = createHarness()

    harness.dismissTransientWorkspaceUi()

    expect(harness.clearActiveNodePanels).toHaveBeenCalledWith(undefined)
    expect(harness.hideStrengthControl).toHaveBeenCalledTimes(1)
    expect(harness.cancelFlowExpandMode).not.toHaveBeenCalled()
    expect(harness.closeBoardHeaderEditing).not.toHaveBeenCalled()
    expect(harness.closeNodeContextMenu).not.toHaveBeenCalled()
  })

  it('closes explicit surfaces and forwards panel keep options', () => {
    const harness = createHarness()

    harness.dismissTransientWorkspaceUi({
      closeExpand: true,
      closeBoardHeader: true,
      closeContextMenu: true,
      clearNodePanels: { keepFusionPrompt: true },
    })

    expect(harness.cancelFlowExpandMode).toHaveBeenCalledTimes(1)
    expect(harness.closeBoardHeaderEditing).toHaveBeenCalledTimes(1)
    expect(harness.closeNodeContextMenu).toHaveBeenCalledTimes(1)
    expect(harness.clearActiveNodePanels).toHaveBeenCalledWith({ keepFusionPrompt: true })
    expect(harness.hideStrengthControl).toHaveBeenCalledTimes(1)
  })

  it('can skip panel and strength cleanup for priority-only dismissals', () => {
    const harness = createHarness()

    harness.dismissTransientWorkspaceUi({
      closeExpand: true,
      clearNodePanels: false,
      hideStrength: false,
    })

    expect(harness.cancelFlowExpandMode).toHaveBeenCalledTimes(1)
    expect(harness.clearActiveNodePanels).not.toHaveBeenCalled()
    expect(harness.hideStrengthControl).not.toHaveBeenCalled()
  })
})
