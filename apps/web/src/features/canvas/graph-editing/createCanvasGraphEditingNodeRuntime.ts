import type { CanvasNodeItem } from '../../canvas-board/domain'
import type { FlowSnapshotParamPatch } from '../utils/flowSnapshotParamMutations'
import { useFlowCanvasNodeHandlers } from './useFlowCanvasNodeHandlers'
import { useFlowNodeMediaInputs } from './useFlowNodeMediaInputs'
import type { CanvasGraphEditingBrowserAdapter } from './browserAdapter'

export function createCanvasGraphEditingNodeRuntime(options: {
  browser: CanvasGraphEditingBrowserAdapter
  applyImageFileToImageNode: (nodeId: string, file: File) => Promise<boolean>
  updateSnapshotNode: (
    nodeId: string,
    mutate: (snapshotNode: CanvasNodeItem) => CanvasNodeItem,
    options?: { history?: boolean },
  ) => void
  markFlowExecutionNodeDirty: (nodeId: string, options?: { includeSelf?: boolean }) => void
  deleteNode: (nodeId: string) => boolean
  updatePromptText: (nodeId: string, value: string) => void
  updateNodeParams: (nodeId: string, payload: FlowSnapshotParamPatch) => void
  dismissTransientCanvasUi: (options?: { closeContextMenu?: boolean }) => void
  discardNodeEdits: (nodeId: string) => void
}) {
  const applyImageFile = async (nodeId: string, file: File) => {
    const updated = await options.applyImageFileToImageNode(nodeId, file)
    if (updated) options.discardNodeEdits(nodeId)
    return updated
  }
  const media = useFlowNodeMediaInputs({
    pickImageFile: options.browser.pickImageFile,
    readDroppedFile: options.browser.readDroppedFile,
    applyImageFileToImageNode: applyImageFile,
    updateSnapshotNode: options.updateSnapshotNode,
    markFlowExecutionNodeDirty: options.markFlowExecutionNodeDirty,
  })
  const handlers = useFlowCanvasNodeHandlers({
    dismissTransientCanvasUi: options.dismissTransientCanvasUi,
    discardFlowImageToolNodeEdits: options.discardNodeEdits,
    deleteNode: options.deleteNode,
    updatePromptText: options.updatePromptText,
    updateNodeParams: options.updateNodeParams,
    resetImage: media.onFlowImageReset,
    markFlowExecutionNodeDirty: options.markFlowExecutionNodeDirty,
  })
  return {
    ...handlers,
    onFlowImageOpenFile: media.onFlowImageOpenFile,
    onFlowImageStageDrop: media.onFlowImageStageDrop,
  }
}
