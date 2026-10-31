import { describe, expect, it, vi } from 'vitest'
import { useFlowTransientUiDismissal } from './useFlowTransientUiDismissal'

function createHarness() {
  const cancelFlowOutpaintMode = vi.fn()
  const closeBoardHeaderEditing = vi.fn()
  const closeNodeContextMenu = vi.fn()
  const clearActiveNodePanel = vi.fn()
  const hideStrengthControl = vi.fn()

  const { dismissTransientCanvasUi } = useFlowTransientUiDismissal({
    cancelFlowOutpaintMode,
    closeBoardHeaderEditing,
    closeNodeContextMenu,
    clearActiveNodePanel,
    hideStrengthControl,
  })

  return {
    dismissTransientCanvasUi,
    cancelFlowOutpaintMode,
    closeBoardHeaderEditing,
    closeNodeContextMenu,
    clearActiveNodePanel,
    hideStrengthControl,
  }
}

describe('useFlowTransientUiDismissal', () => {
  it('clears node panels and strength control by default', () => {
    const harness = createHarness()

    harness.dismissTransientCanvasUi()

    expect(harness.clearActiveNodePanel).toHaveBeenCalledTimes(1)
    expect(harness.hideStrengthControl).toHaveBeenCalledTimes(1)
    expect(harness.cancelFlowOutpaintMode).not.toHaveBeenCalled()
    expect(harness.closeBoardHeaderEditing).not.toHaveBeenCalled()
    expect(harness.closeNodeContextMenu).not.toHaveBeenCalled()
  })

  it('closes explicit surfaces and clears the active node panel', () => {
    const harness = createHarness()

    harness.dismissTransientCanvasUi({
      shouldCloseOutpaint: true,
      shouldCloseBoardHeader: true,
      shouldCloseContextMenu: true,
    })

    expect(harness.cancelFlowOutpaintMode).toHaveBeenCalledTimes(1)
    expect(harness.closeBoardHeaderEditing).toHaveBeenCalledTimes(1)
    expect(harness.closeNodeContextMenu).toHaveBeenCalledTimes(1)
    expect(harness.clearActiveNodePanel).toHaveBeenCalledTimes(1)
    expect(harness.hideStrengthControl).toHaveBeenCalledTimes(1)
  })

  it('can skip panel and strength cleanup for priority-only dismissals', () => {
    const harness = createHarness()

    harness.dismissTransientCanvasUi({
      shouldCloseOutpaint: true,
      shouldClearNodePanel: false,
      shouldHideStrength: false,
    })

    expect(harness.cancelFlowOutpaintMode).toHaveBeenCalledTimes(1)
    expect(harness.clearActiveNodePanel).not.toHaveBeenCalled()
    expect(harness.hideStrengthControl).not.toHaveBeenCalled()
  })
})
