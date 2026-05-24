import type { Ref } from 'vue'
import { clampImageNodeOutputCount } from '../types/canvasNodes'

type MaskEditorReferenceImage = {
  url: string
}

type UseFlowMaskEditorChromeActionsOptions = {
  flowMaskEditorNodeId: Ref<string | null>
  maskEditorReferenceImages: Ref<MaskEditorReferenceImage[]>
  commitFlowMaskEditorMode: () => boolean
  getFlowMaskEditorCommittedData: (
    nodeId: string,
  ) => { executionMaskDataUrl?: string | null } | null | undefined
  clearActiveNodePanels: () => void
  hideStrengthControl: () => void
  runImageInpaintAction: (
    nodeId: string,
    options: {
      prompt: string
      outputCount: number
      maskDataUrl: string
      referenceImageUrls?: string[]
    },
  ) => Promise<void> | void
  onFlowNodeParamUpdate: (nodeId: string, payload: { key: 'outputCount'; value: number }) => void
  onWarning: (message: string) => void
}

export function useFlowMaskEditorChromeActions(options: UseFlowMaskEditorChromeActionsOptions) {
  function onMaskEditorChromeSubmit(prompt: string, outputCount: number) {
    const nodeId = options.flowMaskEditorNodeId.value
    if (!nodeId) return
    const didCommitMask = options.commitFlowMaskEditorMode()
    const committed = options.getFlowMaskEditorCommittedData(nodeId)
    if (!didCommitMask || !committed?.executionMaskDataUrl) {
      options.onWarning('请先标记局部重绘区域')
      return
    }
    options.clearActiveNodePanels()
    options.hideStrengthControl()
    void options.runImageInpaintAction(nodeId, {
      prompt,
      outputCount,
      maskDataUrl: committed.executionMaskDataUrl,
      referenceImageUrls: options.maskEditorReferenceImages.value.map((image) => image.url),
    })
  }

  function onMaskEditorChromeOutputCountUpdate(value: number) {
    const nodeId = options.flowMaskEditorNodeId.value
    if (!nodeId) return
    options.onFlowNodeParamUpdate(nodeId, {
      key: 'outputCount',
      value: clampImageNodeOutputCount(value),
    })
  }

  return {
    onMaskEditorChromeSubmit,
    onMaskEditorChromeOutputCountUpdate,
  }
}
