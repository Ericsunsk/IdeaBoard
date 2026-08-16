import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useMaskEditorChromeActions } from './useMaskEditorChromeActions'

function createHarness(options: { didCommitMask?: boolean; maskDataUrl?: string | null } = {}) {
  const maskEditorNodeId = ref<string | null>('node-1')
  const commitMaskEditorMode = vi.fn(() => options.didCommitMask ?? true)
  const getCommittedMaskData = vi.fn(() =>
    options.maskDataUrl === undefined
      ? { executionMaskDataUrl: 'data:image/png;base64,mask' }
      : { executionMaskDataUrl: options.maskDataUrl },
  )
  const clearActiveNodePanel = vi.fn()
  const hideStrengthControl = vi.fn()
  const runInpaintAction = vi.fn()
  const onWarning = vi.fn()

  const actions = useMaskEditorChromeActions({
    maskEditorNodeId,
    commitMaskEditorMode,
    getCommittedMaskData,
    clearActiveNodePanel,
    hideStrengthControl,
    runInpaintAction,
    onWarning,
  })

  return {
    actions,
    commitMaskEditorMode,
    getCommittedMaskData,
    clearActiveNodePanel,
    hideStrengthControl,
    runInpaintAction,
    onWarning,
  }
}

describe('useMaskEditorChromeActions', () => {
  it('warns and keeps transient UI when mask commit fails', () => {
    const harness = createHarness({ didCommitMask: false, maskDataUrl: null })

    harness.actions.onMaskEditorChromeSubmit('paint this area')

    expect(harness.commitMaskEditorMode).toHaveBeenCalledTimes(1)
    expect(harness.getCommittedMaskData).toHaveBeenCalledWith('node-1')
    expect(harness.onWarning).toHaveBeenCalledWith('请先标记局部重绘区域')
    expect(harness.clearActiveNodePanel).not.toHaveBeenCalled()
    expect(harness.hideStrengthControl).not.toHaveBeenCalled()
    expect(harness.runInpaintAction).not.toHaveBeenCalled()
  })

  it('runs inpaint with only the prompt and committed execution mask', () => {
    const harness = createHarness()

    harness.actions.onMaskEditorChromeSubmit('paint this area')

    expect(harness.clearActiveNodePanel).toHaveBeenCalledTimes(1)
    expect(harness.hideStrengthControl).toHaveBeenCalledTimes(1)
    expect(harness.runInpaintAction).toHaveBeenCalledWith('node-1', {
      prompt: 'paint this area',
      maskDataUrl: 'data:image/png;base64,mask',
    })
  })
})
