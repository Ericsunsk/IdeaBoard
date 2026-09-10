import type { CanvasNodeItem } from '../../../board/domain'
import { IMAGE_NODE_EMPTY_SIZE } from '../../../board/domain'

type UseFlowNodeMediaInputsOptions = {
  pickImageFile: () => Promise<File | null>
  readDroppedFile: (event: DragEvent) => File | null
  applyImageFileToImageNode: (nodeId: string, file: File) => Promise<boolean>
  updateSnapshotNode: (nodeId: string, patcher: (prev: CanvasNodeItem) => CanvasNodeItem) => void
  markFlowExecutionNodeDirty: (nodeId: string) => void
}

export function useFlowNodeMediaInputs(options: UseFlowNodeMediaInputsOptions) {
  // --- Image node handlers ---
  async function onFlowImageOpenFile(nodeId: string) {
    const file = await options.pickImageFile()
    if (!file) return
    await options.applyImageFileToImageNode(nodeId, file)
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
        pendingResultIndex: undefined,
        outputStatus: 'idle',
        outputImages: [],
      }
    })
    options.markFlowExecutionNodeDirty(nodeId)
  }

  function onFlowImageStageDrop(nodeId: string, event: DragEvent) {
    event.preventDefault()
    const file = options.readDroppedFile(event)
    if (!file) return
    void options.applyImageFileToImageNode(nodeId, file)
  }

  return {
    onFlowImageOpenFile,
    onFlowImageReset,
    onFlowImageStageDrop,
  }
}
