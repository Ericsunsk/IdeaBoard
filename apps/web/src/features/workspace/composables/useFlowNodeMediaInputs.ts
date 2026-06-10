import { IMAGE_UPLOAD_ACCEPT } from '../../../shared/constants/media'
import { IMAGE_NODE_EMPTY_SIZE } from '../utils/imageNodeSizing'

type UseFlowNodeMediaInputsOptions = {
  applyImageFileToImageNode: (nodeId: string, file: File) => Promise<boolean>
  updateSnapshotNode: (
    nodeId: string,
    patcher: (prev: Record<string, unknown>) => Record<string, unknown>,
  ) => void
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
    options.updateSnapshotNode(nodeId, (prev) => ({
      ...prev,
      width: IMAGE_NODE_EMPTY_SIZE,
      sourceImageUrl: '',
      sourceImageRatio: 1,
      pendingTaskId: undefined,
      outputStatus: 'idle',
      outputImages: [],
    }))
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
