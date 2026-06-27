import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useMaskEditorChromeActions } from './useMaskEditorChromeActions'

function createHarness(options: { didCommitMask?: boolean; maskDataUrl?: string | null } = {}) {
  const maskEditorNodeId = ref<string | null>('node-1')
  const referenceImages = ref([{ url: 'https://example.com/ref-a.png' }])
  const commitMaskEditorMode = vi.fn(() => options.didCommitMask ?? true)
  const getCommittedMaskData = vi.fn(() =>
    options.maskDataUrl === undefined
      ? { executionMaskDataUrl: 'data:image/png;base64,mask' }
      : { executionMaskDataUrl: options.maskDataUrl },
  )
  const clearActiveNodePanel = vi.fn()
  const hideStrengthControl = vi.fn()
  const runInpaintAction = vi.fn()
  const updateOutputCount = vi.fn()
  const onWarning = vi.fn()

  const actions = useMaskEditorChromeActions({
    maskEditorNodeId,
    referenceImages,
    commitMaskEditorMode,
    getCommittedMaskData,
    clearActiveNodePanel,
    hideStrengthControl,
    runInpaintAction,
    updateOutputCount,
    onWarning,
  })

  return {
    actions,
    commitMaskEditorMode,
    getCommittedMaskData,
    clearActiveNodePanel,
    hideStrengthControl,
    runInpaintAction,
    updateOutputCount,
    onWarning,
  }
}

describe('useMaskEditorChromeActions', () => {
  it('warns and keeps transient UI when mask commit fails', () => {
    const harness = createHarness({ didCommitMask: false, maskDataUrl: null })

    harness.actions.onMaskEditorChromeSubmit('paint this area', 2)

    expect(harness.commitMaskEditorMode).toHaveBeenCalledTimes(1)
    expect(harness.getCommittedMaskData).toHaveBeenCalledWith('node-1')
    expect(harness.onWarning).toHaveBeenCalledWith('请先标记局部重绘区域')
    expect(harness.clearActiveNodePanel).not.toHaveBeenCalled()
    expect(harness.hideStrengthControl).not.toHaveBeenCalled()
    expect(harness.runInpaintAction).not.toHaveBeenCalled()
  })

  it('runs inpaint with reference images after a successful mask commit', () => {
    const harness = createHarness()

    harness.actions.onMaskEditorChromeSubmit('paint this area', 2)

    expect(harness.clearActiveNodePanel).toHaveBeenCalledTimes(1)
    expect(harness.hideStrengthControl).toHaveBeenCalledTimes(1)
    expect(harness.runInpaintAction).toHaveBeenCalledWith('node-1', {
      prompt: 'paint this area',
      outputCount: 2,
      maskDataUrl: 'data:image/png;base64,mask',
      referenceImageUrls: ['https://example.com/ref-a.png'],
    })
  })

  it('normalizes output count before delegating updates', () => {
    const harness = createHarness()

    harness.actions.onMaskEditorChromeOutputCountUpdate(99)

    expect(harness.updateOutputCount).toHaveBeenCalledWith('node-1', 8)
  })
})
