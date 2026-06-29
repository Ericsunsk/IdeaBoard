import type { DismissTransientCanvasUi } from './useFlowTransientUiDismissal'
import type { FlowSnapshotParamPatch } from '../utils/flowSnapshotParamMutations'

type UseFlowCanvasNodeHandlersOptions = {
  dismissTransientCanvasUi: DismissTransientCanvasUi
  discardFlowImageToolNodeEdits: (nodeId: string) => void
  _onFlowDeleteNode: (nodeId: string) => void
  _onFlowPromptTextUpdate: (nodeId: string, value: string) => void
  _onFlowNodeParamUpdate: (nodeId: string, payload: FlowSnapshotParamPatch) => void
  _onFlowImageReset: (nodeId: string) => void
  markFlowExecutionNodeDirty: (nodeId: string, options?: { includeSelf?: boolean }) => void
}

export function useFlowCanvasNodeHandlers(options: UseFlowCanvasNodeHandlersOptions) {
  function onFlowDeleteNode(nodeId: string) {
    options.dismissTransientCanvasUi({ closeContextMenu: true })
    options.discardFlowImageToolNodeEdits(nodeId)
    options._onFlowDeleteNode(nodeId)
  }

  function onFlowPromptTextUpdate(nodeId: string, value: string) {
    options._onFlowPromptTextUpdate(nodeId, value)
    options.markFlowExecutionNodeDirty(nodeId)
  }

  function onFlowNodeParamUpdate(nodeId: string, payload: FlowSnapshotParamPatch) {
    options._onFlowNodeParamUpdate(nodeId, payload)
    options.markFlowExecutionNodeDirty(nodeId, { includeSelf: true })
  }

  function onFlowImageReset(nodeId: string) {
    options.discardFlowImageToolNodeEdits(nodeId)
    options._onFlowImageReset(nodeId)
  }

  return {
    onFlowDeleteNode,
    onFlowPromptTextUpdate,
    onFlowNodeParamUpdate,
    onFlowImageReset,
  }
}
