import type { DismissTransientWorkspaceUi } from './useFlowTransientUiDismissal'

type NodeParamPayload = { key: string; value: unknown }

type UseFlowWorkspaceNodeHandlersOptions = {
  dismissTransientWorkspaceUi: DismissTransientWorkspaceUi
  clearFlowMaskEditorCommittedData: (nodeId: string) => void
  _onFlowDeleteNode: (nodeId: string) => void
  _onFlowPromptTextUpdate: (nodeId: string, value: string) => void
  _onFlowNodeParamUpdate: (nodeId: string, payload: NodeParamPayload) => void
  _onFlowImageReset: (nodeId: string) => void
  markFlowExecutionNodeDirty: (nodeId: string, options?: { includeSelf?: boolean }) => void
}

export function useFlowWorkspaceNodeHandlers(options: UseFlowWorkspaceNodeHandlersOptions) {
  function onFlowDeleteNode(nodeId: string) {
    options.dismissTransientWorkspaceUi({ closeContextMenu: true })
    options.clearFlowMaskEditorCommittedData(nodeId)
    options._onFlowDeleteNode(nodeId)
  }

  function onFlowPromptTextUpdate(nodeId: string, value: string) {
    options._onFlowPromptTextUpdate(nodeId, value)
    options.markFlowExecutionNodeDirty(nodeId)
  }

  function onFlowNodeParamUpdate(nodeId: string, payload: NodeParamPayload) {
    options._onFlowNodeParamUpdate(nodeId, payload)
    options.markFlowExecutionNodeDirty(nodeId, { includeSelf: true })
  }

  function onFlowImageReset(nodeId: string) {
    options.clearFlowMaskEditorCommittedData(nodeId)
    options._onFlowImageReset(nodeId)
  }

  return {
    onFlowDeleteNode,
    onFlowPromptTextUpdate,
    onFlowNodeParamUpdate,
    onFlowImageReset,
  }
}
