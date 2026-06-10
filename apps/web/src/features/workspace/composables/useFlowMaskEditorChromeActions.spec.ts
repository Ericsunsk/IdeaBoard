import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useFlowMaskEditorChromeActions } from './useFlowMaskEditorChromeActions'

function createHarness(options: { didCommitMask?: boolean; maskDataUrl?: string | null } = {}) {
  const flowMaskEditorNodeId = ref<string | null>('node-1')
  const maskEditorReferenceImages = ref([{ url: 'https://example.com/ref-a.png' }])
  const commitFlowMaskEditorMode = vi.fn(() => options.didCommitMask ?? true)
  const getFlowMaskEditorCommittedData = vi.fn(() =>
    options.maskDataUrl === undefined
      ? { executionMaskDataUrl: 'data:image/png;base64,mask' }
      : { executionMaskDataUrl: options.maskDataUrl },
  )
  const clearActiveNodePanels = vi.fn()
  const hideStrengthControl = vi.fn()
  const runImageInpaintAction = vi.fn()
  const onFlowNodeParamUpdate = vi.fn()
  const onWarning = vi.fn()

  const actions = useFlowMaskEditorChromeActions({
    flowMaskEditorNodeId,
    maskEditorReferenceImages,
    commitFlowMaskEditorMode,
    getFlowMaskEditorCommittedData,
    clearActiveNodePanels,
    hideStrengthControl,
    runImageInpaintAction,
    onFlowNodeParamUpdate,
    onWarning,
  })

  return {
    actions,
    commitFlowMaskEditorMode,
    getFlowMaskEditorCommittedData,
    clearActiveNodePanels,
    hideStrengthControl,
    runImageInpaintAction,
    onWarning,
  }
}

describe('useFlowMaskEditorChromeActions', () => {
  it('warns and keeps transient UI when mask commit fails', () => {
    const harness = createHarness({ didCommitMask: false, maskDataUrl: null })

    harness.actions.onMaskEditorChromeSubmit('paint this area', 2)

    expect(harness.commitFlowMaskEditorMode).toHaveBeenCalledTimes(1)
    expect(harness.getFlowMaskEditorCommittedData).toHaveBeenCalledWith('node-1')
    expect(harness.onWarning).toHaveBeenCalledWith('请先标记局部重绘区域')
    expect(harness.clearActiveNodePanels).not.toHaveBeenCalled()
    expect(harness.hideStrengthControl).not.toHaveBeenCalled()
    expect(harness.runImageInpaintAction).not.toHaveBeenCalled()
  })

  it('runs inpaint with reference images after a successful mask commit', () => {
    const harness = createHarness()

    harness.actions.onMaskEditorChromeSubmit('paint this area', 2)

    expect(harness.clearActiveNodePanels).toHaveBeenCalledTimes(1)
    expect(harness.hideStrengthControl).toHaveBeenCalledTimes(1)
    expect(harness.runImageInpaintAction).toHaveBeenCalledWith('node-1', {
      prompt: 'paint this area',
      outputCount: 2,
      maskDataUrl: 'data:image/png;base64,mask',
      referenceImageUrls: ['https://example.com/ref-a.png'],
    })
  })
})
