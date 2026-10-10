import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useInpaintChromeActions } from './useInpaintChromeActions'

function createHarness(options: { didCommitMask?: boolean; maskDataUrl?: string | null } = {}) {
  const inpaintNodeId = ref<string | null>('node-1')
  const commitInpaintMode = vi.fn(() => options.didCommitMask ?? true)
  const getCommittedMaskData = vi.fn(() =>
    options.maskDataUrl === undefined
      ? { executionMaskDataUrl: 'data:image/png;base64,mask' }
      : { executionMaskDataUrl: options.maskDataUrl },
  )
  const clearActiveNodePanel = vi.fn()
  const hideStrengthControl = vi.fn()
  const runInpaintAction = vi.fn()
  const onWarning = vi.fn()

  const actions = useInpaintChromeActions({
    inpaintNodeId,
    commitInpaintMode,
    getCommittedMaskData,
    clearActiveNodePanel,
    hideStrengthControl,
    runInpaintAction,
    onWarning,
  })

  return {
    actions,
    commitInpaintMode,
    getCommittedMaskData,
    clearActiveNodePanel,
    hideStrengthControl,
    runInpaintAction,
    onWarning,
  }
}

describe('useInpaintChromeActions', () => {
  it('warns and keeps transient UI when mask commit fails', () => {
    const harness = createHarness({ didCommitMask: false, maskDataUrl: null })

    harness.actions.onInpaintChromeSubmit('paint this area')

    expect(harness.commitInpaintMode).toHaveBeenCalledTimes(1)
    expect(harness.getCommittedMaskData).toHaveBeenCalledWith('node-1')
    expect(harness.onWarning).toHaveBeenCalledWith('Mark the area to inpaint first.')
    expect(harness.clearActiveNodePanel).not.toHaveBeenCalled()
    expect(harness.hideStrengthControl).not.toHaveBeenCalled()
    expect(harness.runInpaintAction).not.toHaveBeenCalled()
  })

  it('runs inpaint with only the prompt and committed execution mask', () => {
    const harness = createHarness()

    harness.actions.onInpaintChromeSubmit('paint this area')

    expect(harness.clearActiveNodePanel).toHaveBeenCalledTimes(1)
    expect(harness.hideStrengthControl).toHaveBeenCalledTimes(1)
    expect(harness.runInpaintAction).toHaveBeenCalledWith('node-1', {
      prompt: 'paint this area',
      maskDataUrl: 'data:image/png;base64,mask',
    })
  })
})
