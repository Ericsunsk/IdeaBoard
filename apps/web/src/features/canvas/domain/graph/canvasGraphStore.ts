import { createEmptyBoardSnapshot, type BoardSnapshot } from '../../../board'
import { recordCanvasInteractionCount } from '../../infrastructure/browser/canvasInteractionMetrics'
import { measureCanvasPerf } from '../../infrastructure/browser/canvasPerfProbe'
import {
  cloneEdgeRecord,
  cloneGroupRecord,
  cloneNodeRecord,
  createMeasuredStateFromSnapshot,
  exportSnapshotFromState,
} from './canvasGraphStoreState'
import {
  applyCommitPatchOrder,
  commitEdgeWrites,
  commitGroupWrites,
  commitNodeWrites,
  createChangeSet,
  createCommitPatch,
  createSummary,
  isSameJsonValue,
  normalizeTransactionChanges,
  recordAppliedOrderPatch,
  selectCommitPatchRecords,
} from './canvasGraphStoreTransaction'
import { CanvasGraphTransaction } from './canvasGraphStoreTransaction'
import {
  type CanvasGraphCommitEvent,
  type CanvasGraphCommitPatch,
  type CanvasGraphEdgeId,
  type CanvasGraphGroupId,
  type CanvasGraphNodeId,
  type CanvasGraphNodeKind,
  type CanvasGraphNodeRecord,
  type CanvasGraphOrderPatch,
  type CanvasGraphOrderPlacement,
  type CanvasGraphCollectionPatch,
  type CanvasGraphEdgeRecord,
  type CanvasGraphGroupRecord,
  type CanvasGraphPatchDirection,
  type CanvasGraphRecordPatch,
  CanvasGraphStaleTransactionError,
  type CanvasGraphStoreState,
  type CanvasGraphTransactionDraft,
  type CanvasGraphTransactionOptions,
  type CanvasGraphTransactionReason,
  type CanvasGraphTransactionSummary,
} from './canvasGraphStoreTypes'

// Public surface preserved from the pre-split module. Explicit re-exports (not
// `export *`) keep the internal split modules — their helpers, state builders
// and transaction plumbing — private to this directory.
export {
  CanvasGraphStaleTransactionError,
  type CanvasGraphCollectionPatch,
  type CanvasGraphCommitEvent,
  type CanvasGraphCommitPatch,
  type CanvasGraphEdgeId,
  type CanvasGraphEdgeRecord,
  type CanvasGraphGroupGeometry,
  type CanvasGraphGroupId,
  type CanvasGraphGroupRecord,
  type CanvasGraphNodeId,
  type CanvasGraphNodeIdForKind,
  type CanvasGraphNodeKind,
  type CanvasGraphNodeRecord,
  type CanvasGraphNodeUpdate,
  type CanvasGraphOrderPatch,
  type CanvasGraphOrderPlacement,
  type CanvasGraphPatchDirection,
  type CanvasGraphProjectionPatch,
  type CanvasGraphReadPort,
  type CanvasGraphRecordPatch,
  type CanvasGraphRemoveNodeOptions,
  type CanvasGraphTransactionOptions,
  type CanvasGraphTransactionReason,
  type CanvasGraphTransactionSummary,
} from './canvasGraphStoreTypes'
export { CanvasGraphTransaction } from './canvasGraphStoreTransaction'
export {
  toCanvasGraphEdgeId,
  toCanvasGraphGroupId,
  toCanvasGraphNodeId,
} from './canvasGraphStoreIds'

export class CanvasGraphStore {
  private state: CanvasGraphStoreState
  private revision = 0
  private readonly commitListeners = new Set<(event: CanvasGraphCommitEvent) => void>()

  constructor(snapshot: BoardSnapshot = createEmptyBoardSnapshot()) {
    this.state = createMeasuredStateFromSnapshot(snapshot)
  }

  static fromSnapshot(snapshot: BoardSnapshot): CanvasGraphStore {
    return new CanvasGraphStore(snapshot)
  }

  hydrate(snapshot: BoardSnapshot) {
    this.state = createMeasuredStateFromSnapshot(snapshot)
    this.revision += 1
  }

  beginTransaction(
    reason: CanvasGraphTransactionReason,
    options: CanvasGraphTransactionOptions = {},
  ): CanvasGraphTransaction {
    return new CanvasGraphTransaction(this, reason, {
      recordHistory: options.recordHistory ?? true,
      markDirty: options.markDirty ?? true,
    })
  }

  subscribe(listener: (event: CanvasGraphCommitEvent) => void): () => void {
    this.commitListeners.add(listener)
    return () => this.commitListeners.delete(listener)
  }

  applyCommitPatch(
    patch: CanvasGraphCommitPatch,
    direction: CanvasGraphPatchDirection,
    options: CanvasGraphTransactionOptions = {},
  ): CanvasGraphTransactionSummary {
    if (!this._canApplyCommitPatch(patch, direction)) {
      const draft = this._createTransactionDraft()
      return createSummary(
        direction === 'forward' ? 'history-redo' : 'history-undo',
        {
          recordHistory: false,
          markDirty: options.markDirty ?? true,
        },
        this._readState(),
        createChangeSet(),
        draft,
      )
    }
    const draft = this._createTransactionDraft()
    selectCommitPatchRecords(patch.nodes, draft.nodeWrites, cloneNodeRecord, direction)
    selectCommitPatchRecords(patch.edges, draft.edgeWrites, cloneEdgeRecord, direction)
    selectCommitPatchRecords(patch.groups, draft.groupWrites, cloneGroupRecord, direction)
    draft.nodeOrder = applyCommitPatchOrder(this.state.nodeOrder, patch.nodes.order, direction)
    draft.edgeOrder = applyCommitPatchOrder(this.state.edgeOrder, patch.edges.order, direction)
    draft.groupOrder = applyCommitPatchOrder(this.state.groupOrder, patch.groups.order, direction)
    recordAppliedOrderPatch(this.state.nodeOrder, draft.nodeOrder, draft.nodeOrderPatches)
    recordAppliedOrderPatch(this.state.edgeOrder, draft.edgeOrder, draft.edgeOrderPatches)
    recordAppliedOrderPatch(this.state.groupOrder, draft.groupOrder, draft.groupOrderPatches)

    const resolvedOptions: Required<CanvasGraphTransactionOptions> = {
      recordHistory: options.recordHistory ?? false,
      markDirty: options.markDirty ?? true,
    }
    const changes = normalizeTransactionChanges(draft)
    const didChange =
      changes.hasDomainChanges ||
      changes.selectedNodeIds.size > 0 ||
      changes.selectedEdgeIds.size > 0
    const commitPatch = createCommitPatch(draft)
    if (didChange) this._commitTransaction(draft)
    const summary = createSummary(
      direction === 'forward' ? 'history-redo' : 'history-undo',
      resolvedOptions,
      this._readState(),
      changes,
      draft,
      commitPatch,
    )
    if (didChange) this._publishCommit(summary)
    return summary
  }

  exportSnapshot(base?: BoardSnapshot): BoardSnapshot {
    return exportSnapshotFromState(this.state, base)
  }

  getNodeIds(): CanvasGraphNodeId[] {
    return [...this.state.nodeOrder]
  }

  getEdgeIds(): CanvasGraphEdgeId[] {
    return [...this.state.edgeOrder]
  }

  getGroupIds(): CanvasGraphGroupId[] {
    return [...this.state.groupOrder]
  }

  getNode(id: CanvasGraphNodeId): CanvasGraphNodeRecord | undefined {
    const record = this.state.nodesById.get(id)
    return record ? cloneNodeRecord(record) : undefined
  }

  getEdge(id: CanvasGraphEdgeId): CanvasGraphEdgeRecord | undefined {
    const record = this.state.edgesById.get(id)
    return record ? cloneEdgeRecord(record) : undefined
  }

  getGroup(id: CanvasGraphGroupId): CanvasGraphGroupRecord | undefined {
    const record = this.state.groupsById.get(id)
    return record ? cloneGroupRecord(record) : undefined
  }

  getGroupIdForNode(nodeId: CanvasGraphNodeId): CanvasGraphGroupId | undefined {
    return this.state.indexes.groupIdByNodeId.get(nodeId)
  }

  getNodeIdsByKind(kind: CanvasGraphNodeKind): CanvasGraphNodeId[] {
    return Array.from(this.state.indexes.nodeIdsByKind.get(kind) ?? [])
  }

  getOutgoingEdgeIds(nodeId: CanvasGraphNodeId): CanvasGraphEdgeId[] {
    return Array.from(this.state.indexes.outgoingEdgesByNodeId.get(nodeId) ?? [])
  }

  getIncomingEdgeIds(nodeId: CanvasGraphNodeId): CanvasGraphEdgeId[] {
    return Array.from(this.state.indexes.incomingEdgesByNodeId.get(nodeId) ?? [])
  }

  readCounts(): { nodes: number; edges: number } {
    return {
      nodes: this.state.nodeOrder.length,
      edges: this.state.edgeOrder.length,
    }
  }

  _createTransactionDraft(): CanvasGraphTransactionDraft {
    return measureCanvasPerf(
      'graph-transaction.draft',
      () => this.readCounts(),
      () => ({
        base: this.state,
        baseRevision: this.revision,
        nodeWrites: new Map(),
        edgeWrites: new Map(),
        groupWrites: new Map(),
        nodeOrderPatches: new Map(),
        edgeOrderPatches: new Map(),
        groupOrderPatches: new Map(),
      }),
    )
  }

  _commitTransaction(draft: CanvasGraphTransactionDraft) {
    this._assertTransactionCurrent(draft)

    commitNodeWrites(this.state, draft.nodeWrites)
    commitEdgeWrites(this.state, draft.edgeWrites)
    commitGroupWrites(this.state, draft.groupWrites)

    if (draft.nodeOrder) this.state.nodeOrder = draft.nodeOrder
    if (draft.edgeOrder) this.state.edgeOrder = draft.edgeOrder
    if (draft.groupOrder) this.state.groupOrder = draft.groupOrder
    this.revision += 1
    this.state = { ...this.state }
  }

  _publishCommit(summary: CanvasGraphTransactionSummary) {
    const event: CanvasGraphCommitEvent = Object.freeze({
      summary,
      patch: summary.commitPatch,
      sideEffects: Object.freeze({
        recordHistory: summary.shouldRecordHistory,
        markDirty: summary.shouldMarkDirty,
      }),
    })
    recordCanvasInteractionCount('graphCommitCount')
    if (event.sideEffects.recordHistory) recordCanvasInteractionCount('graphHistoryCommitCount')
    if (event.sideEffects.markDirty) recordCanvasInteractionCount('graphDirtyCommitCount')
    this.commitListeners.forEach((listener) => {
      try {
        listener(event)
      } catch (error) {
        // A projection/observer failure must never invalidate a canonical
        // graph commit or prevent remaining observers from seeing it.
        console.error('[Canvas] Graph commit listener failed', error)
      }
    })
  }

  _canApplyCommitPatch(
    patch: CanvasGraphCommitPatch,
    direction: CanvasGraphPatchDirection,
  ): boolean {
    const source = <Record, Id extends string>(recordPatch: CanvasGraphRecordPatch<Record, Id>) =>
      direction === 'forward' ? recordPatch.before : recordPatch.after
    const matchesRecords = <Record, Id extends string>(
      records: ReadonlyMap<Id, Record>,
      collection: CanvasGraphCollectionPatch<Record, Id>,
    ) =>
      collection.records.every((recordPatch) => {
        const expected = source(recordPatch)
        const current = records.get(recordPatch.id)
        return expected === undefined
          ? current === undefined
          : !!current && isSameJsonValue(current, expected)
      })
    const matchesPlacement = <Id extends string>(
      order: readonly Id[],
      placement: CanvasGraphOrderPlacement<Id> | undefined,
      id: Id,
    ) => {
      const index = order.indexOf(id)
      if (!placement) return index < 0
      if (index < 0) return false
      const previousId = order[index - 1]
      const nextId = order[index + 1]
      return (
        (placement.previousId === undefined || placement.previousId === previousId) &&
        (placement.nextId === undefined || placement.nextId === nextId)
      )
    }
    const matchesOrder = <Id extends string>(
      order: readonly Id[],
      changes: readonly CanvasGraphOrderPatch<Id>[],
    ) =>
      changes.every((change) =>
        matchesPlacement(order, direction === 'forward' ? change.before : change.after, change.id),
      )
    return (
      matchesRecords(this.state.nodesById, patch.nodes) &&
      matchesRecords(this.state.edgesById, patch.edges) &&
      matchesRecords(this.state.groupsById, patch.groups) &&
      matchesOrder(this.state.nodeOrder, patch.nodes.order) &&
      matchesOrder(this.state.edgeOrder, patch.edges.order) &&
      matchesOrder(this.state.groupOrder, patch.groups.order)
    )
  }

  _assertTransactionCurrent(draft: CanvasGraphTransactionDraft) {
    if (this.revision !== draft.baseRevision) {
      throw new CanvasGraphStaleTransactionError()
    }
  }

  _readState(): CanvasGraphStoreState {
    return this.state
  }
}

export function createCanvasGraphStore(snapshot?: BoardSnapshot): CanvasGraphStore {
  return new CanvasGraphStore(snapshot)
}
