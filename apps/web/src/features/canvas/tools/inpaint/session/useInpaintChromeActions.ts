import type { ValueRef } from '@/core/vue/refs'
import { i18n } from '../../../../../core/i18n'
import type { ImageInpaintCommittedData } from '../core/model.ts'

type InpaintExecutionMaskData = {
  executionMaskDataUrl?: ImageInpaintCommittedData['executionMaskDataUrl'] | null
}

type UseInpaintChromeActionsOptions = {
  inpaintNodeId: ValueRef<string | null>
  commitInpaintMode: () => boolean
  getCommittedMaskData: (nodeId: string) => InpaintExecutionMaskData | null | undefined
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

export function useInpaintChromeActions(options: UseInpaintChromeActionsOptions) {
  function onInpaintChromeSubmit(prompt: string) {
    const nodeId = options.inpaintNodeId.value
    if (!nodeId) return
    const didCommitMask = options.commitInpaintMode()
    const committed = options.getCommittedMaskData(nodeId)
    if (!didCommitMask || !committed?.executionMaskDataUrl) {
      options.onWarning(i18n.global.t('canvas.execution.markInpaintAreaFirst'))
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
    onInpaintChromeSubmit,
  }
}
