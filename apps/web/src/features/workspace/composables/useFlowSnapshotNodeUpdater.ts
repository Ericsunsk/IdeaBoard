import type { CanvasNodeItem } from '../types/canvasNodes'

type SnapshotMutator = (snapshotNode: CanvasNodeItem) => CanvasNodeItem
type SnapshotOptions = { history?: boolean }

type UseFlowSnapshotNodeUpdaterOptions = {
  updateSnapshotNodeBase: (
    nodeId: string,
    mutate: SnapshotMutator,
    options?: SnapshotOptions,
  ) => void
  scheduleFlowNodeInternalsRefresh: (nodeIds: string[]) => void
}

export function useFlowSnapshotNodeUpdater(options: UseFlowSnapshotNodeUpdaterOptions) {
  function updateSnapshotNode(
    nodeId: string,
    mutate: SnapshotMutator,
    optionsArg?: SnapshotOptions,
  ) {
    options.updateSnapshotNodeBase(nodeId, mutate, optionsArg)
    options.scheduleFlowNodeInternalsRefresh([nodeId])
  }

  return {
    updateSnapshotNode,
  }
}
