import { clampImageNodeOutputCount } from '../../../canvas-board/domain'
import type { ImageMaskEditorCommittedData, MaskEditorReferenceImage } from '../types'

type ValueRef<T> = {
  readonly value: T
}

type MaskEditorExecutionMaskData = {
  executionMaskDataUrl?: ImageMaskEditorCommittedData['executionMaskDataUrl'] | null
}

type UseMaskEditorChromeActionsOptions = {
  maskEditorNodeId: ValueRef<string | null>
  referenceImages: ValueRef<readonly Pick<MaskEditorReferenceImage, 'url'>[]>
  commitMaskEditorMode: () => boolean
  getCommittedMaskData: (nodeId: string) => MaskEditorExecutionMaskData | null | undefined
  clearActiveNodePanel: () => void
  hideStrengthControl: () => void
  runInpaintAction: (
    nodeId: string,
    options: {
      prompt: string
      outputCount: number
      maskDataUrl: string
      referenceImageUrls?: string[]
    },
  ) => Promise<void> | void
  updateOutputCount: (nodeId: string, outputCount: number) => void
  onWarning: (message: string) => void
}

export function useMaskEditorChromeActions(options: UseMaskEditorChromeActionsOptions) {
  function onMaskEditorChromeSubmit(prompt: string, outputCount: number) {
    const nodeId = options.maskEditorNodeId.value
    if (!nodeId) return
    const didCommitMask = options.commitMaskEditorMode()
    const committed = options.getCommittedMaskData(nodeId)
    if (!didCommitMask || !committed?.executionMaskDataUrl) {
      options.onWarning('请先标记局部重绘区域')
      return
    }
    options.clearActiveNodePanel()
    options.hideStrengthControl()
    void options.runInpaintAction(nodeId, {
      prompt,
      outputCount,
      maskDataUrl: committed.executionMaskDataUrl,
      referenceImageUrls: options.referenceImages.value.map((image) => image.url),
    })
  }

  function onMaskEditorChromeOutputCountUpdate(value: number) {
    const nodeId = options.maskEditorNodeId.value
    if (!nodeId) return
    options.updateOutputCount(nodeId, clampImageNodeOutputCount(value))
  }

  return {
    onMaskEditorChromeSubmit,
    onMaskEditorChromeOutputCountUpdate,
  }
}
