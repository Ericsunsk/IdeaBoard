import { IMAGE_UPLOAD_ACCEPT } from '../../../shared/constants/media'
import type { CanvasNodeItem } from '../../canvas-board/domain'
import { IMAGE_NODE_EMPTY_SIZE } from '../../canvas-board/domain'

type UseFlowNodeMediaInputsOptions = {
  applyImageFileToImageNode: (nodeId: string, file: File) => Promise<boolean>
  updateSnapshotNode: (nodeId: string, patcher: (prev: CanvasNodeItem) => CanvasNodeItem) => void
  markFlowExecutionNodeDirty: (nodeId: string) => void
}

export function useFlowNodeMediaInputs(options: UseFlowNodeMediaInputsOptions) {
  // --- Image node handlers ---
  function onFlowImageOpenFile(nodeId: string) {
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = IMAGE_UPLOAD_ACCEPT
    input.onchange = () => {
      const file = input.files?.[0]
      if (!file) return
      void options.applyImageFileToImageNode(nodeId, file)
    }
    input.click()
  }

  function onFlowImageReset(nodeId: string) {
    options.updateSnapshotNode(nodeId, (prev) => {
      if (prev.type !== 'image-node') return prev
      return {
        ...prev,
        width: IMAGE_NODE_EMPTY_SIZE,
        sourceImageUrl: '',
        sourceMediaAssetId: undefined,
        sourceImageRatio: 1,
        pendingTaskId: undefined,
        outputStatus: 'idle',
        outputImages: [],
      }
    })
    options.markFlowExecutionNodeDirty(nodeId)
  }

  function onFlowImageStageDrop(nodeId: string, event: DragEvent) {
    event.preventDefault()
    const file = event.dataTransfer?.files?.[0]
    if (!file) return
    void options.applyImageFileToImageNode(nodeId, file)
  }

  return {
    onFlowImageOpenFile,
    onFlowImageReset,
    onFlowImageStageDrop,
  }
}
