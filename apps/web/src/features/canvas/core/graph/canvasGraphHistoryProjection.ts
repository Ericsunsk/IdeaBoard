import { cloneEdgeRecord, cloneGroupRecord, cloneNodeRecord } from './canvasGraphStoreState'
import { createCommitPatch, hasVisibleChanges } from './canvasGraphPatchAlgebra'
import type {
  CanvasGraphEdgeRecord,
  CanvasGraphGroupRecord,
  CanvasGraphNodeRecord,
  CanvasGraphCommitPatch,
  CanvasGraphStoreState,
  CanvasGraphTransactionDraft,
  CanvasGraphTransactionOptions,
  CanvasGraphTransactionReason,
  CanvasGraphTransactionSummary,
  TransactionChangeSet,
} from './canvasGraphStoreTypes'

/**
 * History and projection summary for the canvas graph transaction subsystem.
 *
 * Builds the single `CanvasGraphTransactionSummary` consumed by History,
 * persistence/dirty tracking, and Flow projection from a normalized
 * change set. Callers never construct summaries by hand — both the live
 * transaction port and the history undo/redo path funnel through
 * `createSummary` so dirty/history/projection semantics stay identical.
 */

let transactionSequence = 0

export function createSummary(
  reason: CanvasGraphTransactionReason,
  options: Required<CanvasGraphTransactionOptions>,
  state: CanvasGraphStoreState,
  changes: TransactionChangeSet,
  draft: CanvasGraphTransactionDraft,
  commitPatch: CanvasGraphCommitPatch = createCommitPatch(draft),
): CanvasGraphTransactionSummary {
  const addedNodeIds = Array.from(changes.addedNodeIds)
  const updatedNodeIds = Array.from(changes.updatedNodeIds)
  const addedEdgeIds = Array.from(changes.addedEdgeIds)
  const addedGroupIds = Array.from(changes.addedGroupIds)
  const updatedGroupIds = Array.from(changes.updatedGroupIds)
  const selectedNodeIds = Array.from(changes.selectedNodeIds)
  const selectedEdgeIds = Array.from(changes.selectedEdgeIds)
  const removedNodeIds = Array.from(changes.removedNodeIds)
  const removedEdgeIds = Array.from(changes.removedEdgeIds)
  const removedGroupIds = Array.from(changes.removedGroupIds)
  const changedNodeIds = [...addedNodeIds, ...updatedNodeIds]
  const changedEdgeIds = addedEdgeIds
  const changedGroupIds = [...addedGroupIds, ...updatedGroupIds]
  const didChange = hasVisibleChanges(changes)

  return {
    id: `canvas-graph-tx-${++transactionSequence}`,
    reason,
    origin: options.origin,
    didChange,
    hasDomainChanges: changes.hasDomainChanges,
    changedNodeIds,
    changedEdgeIds,
    changedGroupIds,
    removedNodeIds,
    removedEdgeIds,
    removedGroupIds,
    selectedNodeIds,
    selectedEdgeIds,
    didChangeOrder:
      changes.didChangeNodeOrder || changes.didChangeEdgeOrder || changes.didChangeGroupOrder,
    shouldRecordHistory: changes.hasDomainChanges && options.shouldRecordHistory,
    shouldMarkDirty: changes.hasDomainChanges && options.shouldMarkDirty,
    commitPatch,
    projectionPatch: {
      nodes: {
        added: addedNodeIds
          .map((id) => state.nodesById.get(id))
          .filter((record): record is CanvasGraphNodeRecord => !!record)
          .map(cloneNodeRecord),
        updated: updatedNodeIds
          .map((id) => state.nodesById.get(id))
          .filter((record): record is CanvasGraphNodeRecord => !!record)
          .map(cloneNodeRecord),
        removedIds: removedNodeIds,
        selectionChanged: selectedNodeIds
          .map((id) => state.nodesById.get(id))
          .filter((record): record is CanvasGraphNodeRecord => !!record)
          .map((record) => ({ id: record.id, isSelected: record.isSelected })),
      },
      edges: {
        added: addedEdgeIds
          .map((id) => state.edgesById.get(id))
          .filter((record): record is CanvasGraphEdgeRecord => !!record)
          .map(cloneEdgeRecord),
        removedIds: removedEdgeIds,
        selectionChanged: selectedEdgeIds
          .map((id) => state.edgesById.get(id))
          .filter((record): record is CanvasGraphEdgeRecord => !!record)
          .map((record) => ({ id: record.id, isSelected: record.isSelected })),
      },
      groups: {
        added: addedGroupIds
          .map((id) => state.groupsById.get(id))
          .filter((record): record is CanvasGraphGroupRecord => !!record)
          .map(cloneGroupRecord),
        updated: updatedGroupIds
          .map((id) => state.groupsById.get(id))
          .filter((record): record is CanvasGraphGroupRecord => !!record)
          .map(cloneGroupRecord),
        removedIds: removedGroupIds,
      },
    },
  }
}
