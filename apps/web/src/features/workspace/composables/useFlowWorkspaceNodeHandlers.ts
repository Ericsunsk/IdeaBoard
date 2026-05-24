type NodeParamPayload = { key: string; value: unknown }
type SnapshotMutator = (snapshotNode: Record<string, unknown>) => Record<string, unknown>
type SnapshotOptions = { history?: boolean }

type UseFlowWorkspaceNodeHandlersOptions = {
  clearActiveNodePanels: () => void
  hideStrengthControl: () => void
  closeNodeContextMenu: () => void
  clearFlowMaskEditorCommittedData: (nodeId: string) => void
  _onFlowDeleteNode: (nodeId: string) => void
  _onFlowPromptTextUpdate: (nodeId: string, value: string) => void
  _onFlowNodeParamUpdate: (nodeId: string, payload: NodeParamPayload) => void
  updateSnapshotNodeBase: (
    nodeId: string,
    mutate: SnapshotMutator,
    options?: SnapshotOptions,
  ) => void
  _onFlowImageReset: (nodeId: string) => void
  markFlowExecutionNodeDirty: (nodeId: string, options?: { includeSelf?: boolean }) => void
  scheduleFlowNodeInternalsRefresh: (nodeIds: string[]) => void
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

  function updateSnapshotNode(nodeId: string, mutate: SnapshotMutator, optionsArg?: SnapshotOptions) {
    options.updateSnapshotNodeBase(nodeId, mutate, optionsArg)
    options.scheduleFlowNodeInternalsRefresh([nodeId])
  }

  function onFlowImageReset(nodeId: string) {
    options.clearFlowMaskEditorCommittedData(nodeId)
    options._onFlowImageReset(nodeId)
  }

  return {
    onFlowDeleteNode,
    onFlowPromptTextUpdate,
    onFlowNodeParamUpdate,
    updateSnapshotNode,
    onFlowImageReset,
  }
}
