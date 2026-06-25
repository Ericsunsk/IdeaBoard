import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useFlowNodeDragInteractions } from './useFlowNodeDragInteractions'

function createHarness(duplicateModifierPressed = false) {
  const beginNodeDragTransaction = vi.fn()
  const commitNodeDragTransaction = vi.fn()
  const beginFlowNodeDragDuplicateTracking = vi.fn()
  const finishFlowNodeDragDuplicateTracking = vi.fn()
  const dismissTransientWorkspaceUi = vi.fn()

  const interactions = useFlowNodeDragInteractions({
    beginNodeDragTransaction,
    commitNodeDragTransaction,
    duplicateModifierPressed: ref(duplicateModifierPressed),
    beginFlowNodeDragDuplicateTracking,
    finishFlowNodeDragDuplicateTracking,
    dismissTransientWorkspaceUi,
  })

  return {
    interactions,
    beginNodeDragTransaction,
    commitNodeDragTransaction,
    beginFlowNodeDragDuplicateTracking,
    finishFlowNodeDragDuplicateTracking,
    dismissTransientWorkspaceUi,
  }
}

describe('useFlowNodeDragInteractions', () => {
  it('dismisses transient UI before starting a node drag transaction', () => {
    const harness = createHarness(true)

    harness.interactions.onNodeDragStart()

    expect(harness.dismissTransientWorkspaceUi).toHaveBeenCalledWith({ closeExpand: true })
    expect(harness.beginNodeDragTransaction).toHaveBeenCalledTimes(1)
    expect(harness.beginFlowNodeDragDuplicateTracking).toHaveBeenCalledWith(true)
    expect(harness.dismissTransientWorkspaceUi.mock.invocationCallOrder[0]).toBeLessThan(
      harness.beginNodeDragTransaction.mock.invocationCallOrder[0] ?? 0,
    )
  })

  it('only closes expand mode when a node drag stops', () => {
    const harness = createHarness(false)

    harness.interactions.onNodeDragStop()

    expect(harness.dismissTransientWorkspaceUi).toHaveBeenCalledWith({
      closeExpand: true,
      clearNodePanel: false,
      hideStrength: false,
    })
    expect(harness.finishFlowNodeDragDuplicateTracking).toHaveBeenCalledWith(false)
    expect(harness.commitNodeDragTransaction).toHaveBeenCalledTimes(1)
  })
})
