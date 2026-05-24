type NodeParamPayload = { key: string; value: unknown }

type UseFlowWorkspaceNodeHandlersOptions = {
  clearActiveNodePanels: () => void
  hideStrengthControl: () => void
  closeNodeContextMenu: () => void
  clearFlowMaskEditorCommittedData: (nodeId: string) => void
  _onFlowDeleteNode: (nodeId: string) => void
  _onFlowPromptTextUpdate: (nodeId: string, value: string) => void
  _onFlowNodeParamUpdate: (nodeId: string, payload: NodeParamPayload) => void
  _onFlowImageReset: (nodeId: string) => void
  markFlowExecutionNodeDirty: (nodeId: string, options?: { includeSelf?: boolean }) => void
}

export function useFlowWorkspaceNodeHandlers(options: UseFlowWorkspaceNodeHandlersOptions) {
  function onFlowDeleteNode(nodeId: string) {
    options.clearActiveNodePanels()
    options.clearFlowMaskEditorCommittedData(nodeId)
    options.hideStrengthControl()
    options.closeNodeContextMenu()
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
