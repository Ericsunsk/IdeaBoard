import type { ImageMaskEditorCommittedData } from './types'

type ValueRef<T> = {
  readonly value: T
}

type MaskEditorExecutionMaskData = {
  executionMaskDataUrl?: ImageMaskEditorCommittedData['executionMaskDataUrl'] | null
}

type UseMaskEditorChromeActionsOptions = {
  maskEditorNodeId: ValueRef<string | null>
  commitMaskEditorMode: () => boolean
  getCommittedMaskData: (nodeId: string) => MaskEditorExecutionMaskData | null | undefined
  clearActiveNodePanel: () => void
  hideStrengthControl: () => void
  runInpaintAction: (
    nodeId: string,
    options: {
      prompt: string
      maskDataUrl: string
    },
  ) => Promise<void> | void
  onWarning: (message: string) => void
}

export function useMaskEditorChromeActions(options: UseMaskEditorChromeActionsOptions) {
  function onMaskEditorChromeSubmit(prompt: string) {
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
      maskDataUrl: committed.executionMaskDataUrl,
    })
  }

  return {
    onMaskEditorChromeSubmit,
  }
}
