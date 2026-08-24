import type { DismissTransientCanvasUi } from '../workspace-ui/useFlowTransientUiDismissal'
import type { FlowSnapshotParamPatch } from '../utils/flowSnapshotParamMutations'

type UseFlowCanvasNodeHandlersOptions = {
  dismissTransientCanvasUi: DismissTransientCanvasUi
  discardFlowImageToolNodeEdits: (nodeId: string) => void
  deleteNode: (nodeId: string) => void
  updatePromptText: (nodeId: string, value: string) => void
  updateNodeParams: (nodeId: string, payload: FlowSnapshotParamPatch) => void
  resetImage: (nodeId: string) => void
  markFlowExecutionNodeDirty: (nodeId: string, options?: { includeSelf?: boolean }) => void
}

export function useFlowCanvasNodeHandlers(options: UseFlowCanvasNodeHandlersOptions) {
  function onFlowDeleteNode(nodeId: string) {
    options.dismissTransientCanvasUi({ closeContextMenu: true })
    options.discardFlowImageToolNodeEdits(nodeId)
    options.deleteNode(nodeId)
  }

  function onFlowPromptTextUpdate(nodeId: string, value: string) {
    options.updatePromptText(nodeId, value)
    options.markFlowExecutionNodeDirty(nodeId)
  }

  function onFlowNodeParamUpdate(nodeId: string, payload: FlowSnapshotParamPatch) {
    options.updateNodeParams(nodeId, payload)
    options.markFlowExecutionNodeDirty(nodeId, { includeSelf: true })
  }

  function onFlowImageReset(nodeId: string) {
    options.discardFlowImageToolNodeEdits(nodeId)
    options.resetImage(nodeId)
  }

  return {
    onFlowDeleteNode,
    onFlowPromptTextUpdate,
    onFlowNodeParamUpdate,
    onFlowImageReset,
  }
}
