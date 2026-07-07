import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useFlowNodeDragInteractions } from './useFlowNodeDragInteractions'

function createHarness(
  options: { duplicateModifierPressed?: boolean; nodeDragLocked?: boolean } = {},
) {
  const beginNodeDragTransaction = vi.fn()
  const commitNodeDragTransaction = vi.fn()
  const beginFlowNodeDragDuplicateTracking = vi.fn()
  const finishFlowNodeDragDuplicateTracking = vi.fn()
  const dismissTransientCanvasUi = vi.fn()
  const isNodeDragLocked = vi.fn(() => options.nodeDragLocked ?? false)

  const interactions = useFlowNodeDragInteractions({
    beginNodeDragTransaction,
    commitNodeDragTransaction,
    duplicateModifierPressed: ref(options.duplicateModifierPressed ?? false),
    beginFlowNodeDragDuplicateTracking,
    finishFlowNodeDragDuplicateTracking,
    dismissTransientCanvasUi,
    isNodeDragLocked,
  })

  return {
    interactions,
    beginNodeDragTransaction,
    commitNodeDragTransaction,
    beginFlowNodeDragDuplicateTracking,
    finishFlowNodeDragDuplicateTracking,
    dismissTransientCanvasUi,
    isNodeDragLocked,
  }
}

describe('useFlowNodeDragInteractions', () => {
  it('dismisses transient UI before starting a node drag transaction', () => {
    const harness = createHarness({ duplicateModifierPressed: true })

    harness.interactions.onNodeDragStart()

    expect(harness.dismissTransientCanvasUi).toHaveBeenCalledWith({ closeExpand: true })
    expect(harness.beginNodeDragTransaction).toHaveBeenCalledTimes(1)
    expect(harness.beginFlowNodeDragDuplicateTracking).toHaveBeenCalledWith(true)
    expect(harness.dismissTransientCanvasUi.mock.invocationCallOrder[0]).toBeLessThan(
      harness.beginNodeDragTransaction.mock.invocationCallOrder[0] ?? 0,
    )
  })

  it('only closes expand mode when a node drag stops', async () => {
    const harness = createHarness()

    harness.interactions.onNodeDragStop()

    expect(harness.dismissTransientCanvasUi).toHaveBeenCalledWith({
      closeExpand: true,
      clearNodePanel: false,
      hideStrength: false,
    })
    expect(harness.finishFlowNodeDragDuplicateTracking).toHaveBeenCalledWith(false)
    expect(harness.commitNodeDragTransaction).not.toHaveBeenCalled()

    await Promise.resolve()

    expect(harness.commitNodeDragTransaction).toHaveBeenCalledTimes(1)
  })

  it('only commits the latest queued drag stop', async () => {
    const harness = createHarness()

    harness.interactions.onNodeDragStop()
    harness.interactions.onNodeDragStop()

    await Promise.resolve()

    expect(harness.commitNodeDragTransaction).toHaveBeenCalledTimes(1)
  })

  it('ignores node drag callbacks while node drag is locked', async () => {
    const harness = createHarness({ nodeDragLocked: true })

    harness.interactions.onNodeDragStart()
    harness.interactions.onNodeDragStop()

    expect(harness.dismissTransientCanvasUi).not.toHaveBeenCalled()
    expect(harness.beginNodeDragTransaction).not.toHaveBeenCalled()
    expect(harness.beginFlowNodeDragDuplicateTracking).not.toHaveBeenCalled()
    expect(harness.finishFlowNodeDragDuplicateTracking).not.toHaveBeenCalled()

    await Promise.resolve()

    expect(harness.commitNodeDragTransaction).not.toHaveBeenCalled()
  })
})
