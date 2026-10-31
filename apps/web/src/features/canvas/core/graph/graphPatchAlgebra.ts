import { cloneEdgeRecord, cloneGroupRecord, cloneNodeRecord } from './graphStoreState'
import { hasSameGroupRecord } from './graphRecordEquality'
import { isSameJsonValue } from './graphValueEquality'
import type {
  CanvasGraphCollectionPatch,
  CanvasGraphCommitPatch,
  CanvasGraphOrderPatch,
  CanvasGraphOrderPlacement,
  CanvasGraphPatchDirection,
  CanvasGraphRecordPatch,
  CanvasGraphStoreState,
  CanvasGraphTransactionDraft,
  TransactionChangeSet,
  TransactionRecordWrites,
} from './graphStoreTypes'

/**
 * Patch algebra for the canvas graph transaction subsystem.
 *
 * Pure value math only: change-set collection/normalization, commit-patch
 * build/select/apply, order-patch bookkeeping, record equality, and patch
 * applicability checks. No store access, no draft staging rules, no summary
 * building — those live in the mutation and history/projection modules.
 */

export function createChangeSet(): TransactionChangeSet {
  return {
    addedNodeIds: new Set(),
    updatedNodeIds: new Set(),
    removedNodeIds: new Set(),
    selectedNodeIds: new Set(),
    addedEdgeIds: new Set(),
    removedEdgeIds: new Set(),
    selectedEdgeIds: new Set(),
    addedGroupIds: new Set(),
    updatedGroupIds: new Set(),
    removedGroupIds: new Set(),
    didChangeNodeOrder: false,
    didChangeEdgeOrder: false,
    didChangeGroupOrder: false,
    hasDomainChanges: false,
  }
}

export { isSameJsonValue }

/** A commit is visible when it carries domain changes or selection changes. */
export function hasVisibleChanges(changes: TransactionChangeSet): boolean {
  return (
    changes.hasDomainChanges || changes.selectedNodeIds.size > 0 || changes.selectedEdgeIds.size > 0
  )
}

function collectNodeWriteChanges(
  draft: CanvasGraphTransactionDraft,
  changes: TransactionChangeSet,
): void {
  for (const [nodeId, nextRecord] of draft.nodeWrites) {
    const baseRecord = draft.base.nodesById.get(nodeId)
    if (!baseRecord && nextRecord) {
      changes.addedNodeIds.add(nodeId)
      continue
    }
    if (baseRecord && !nextRecord) {
      changes.removedNodeIds.add(nodeId)
      continue
    }
    if (!baseRecord || !nextRecord) continue
    if (!isSameJsonValue(baseRecord.snapshot, nextRecord.snapshot)) {
      changes.updatedNodeIds.add(nodeId)
    }
    if (baseRecord.isSelected !== nextRecord.isSelected) changes.selectedNodeIds.add(nodeId)
  }
}

function collectEdgeWriteChanges(
  draft: CanvasGraphTransactionDraft,
  changes: TransactionChangeSet,
): void {
  for (const [edgeId, nextRecord] of draft.edgeWrites) {
    const baseRecord = draft.base.edgesById.get(edgeId)
    if (!baseRecord && nextRecord) {
      changes.addedEdgeIds.add(edgeId)
      continue
    }
    if (baseRecord && !nextRecord) {
      changes.removedEdgeIds.add(edgeId)
      continue
    }
    if (!baseRecord || !nextRecord) continue
    if (!isSameJsonValue(baseRecord.connection, nextRecord.connection)) {
      changes.removedEdgeIds.add(edgeId)
      changes.addedEdgeIds.add(edgeId)
    }
    if (baseRecord.isSelected !== nextRecord.isSelected) changes.selectedEdgeIds.add(edgeId)
  }
}

function collectGroupWriteChanges(
  draft: CanvasGraphTransactionDraft,
  changes: TransactionChangeSet,
): void {
  for (const [groupId, nextRecord] of draft.groupWrites) {
    const baseRecord = draft.base.groupsById.get(groupId)
    if (!baseRecord && nextRecord) {
      changes.addedGroupIds.add(groupId)
      continue
    }
    if (baseRecord && !nextRecord) {
      changes.removedGroupIds.add(groupId)
      continue
    }
    if (baseRecord && nextRecord && !hasSameGroupRecord(baseRecord, nextRecord)) {
      changes.updatedGroupIds.add(groupId)
    }
  }
}

function summarizeChangeSetFlags(changes: TransactionChangeSet): void {
  // Transactions only mutate order through add/remove.  Derive this from the
  // changed records so a one-node edit never scans or copies Board-sized order
  // arrays merely to decide whether a History patch is needed.
  changes.didChangeNodeOrder = changes.addedNodeIds.size > 0 || changes.removedNodeIds.size > 0
  changes.didChangeEdgeOrder = changes.addedEdgeIds.size > 0 || changes.removedEdgeIds.size > 0
  changes.didChangeGroupOrder = changes.addedGroupIds.size > 0 || changes.removedGroupIds.size > 0
  changes.hasDomainChanges =
    changes.addedNodeIds.size > 0 ||
    changes.updatedNodeIds.size > 0 ||
    changes.removedNodeIds.size > 0 ||
    changes.addedEdgeIds.size > 0 ||
    changes.removedEdgeIds.size > 0 ||
    changes.addedGroupIds.size > 0 ||
    changes.updatedGroupIds.size > 0 ||
    changes.removedGroupIds.size > 0 ||
    changes.didChangeNodeOrder ||
    changes.didChangeEdgeOrder ||
    changes.didChangeGroupOrder
}

export function normalizeTransactionChanges(
  draft: CanvasGraphTransactionDraft,
): TransactionChangeSet {
  const changes = createChangeSet()
  collectNodeWriteChanges(draft, changes)
  collectEdgeWriteChanges(draft, changes)
  collectGroupWriteChanges(draft, changes)
  summarizeChangeSetFlags(changes)
  return changes
}

function cloneRecordPatch<Record, Id extends string>(
  id: Id,
  before: Record | undefined,
  after: Record | undefined,
  clone: (record: Record) => Record,
): CanvasGraphRecordPatch<Record, Id> | null {
  if (before === undefined && after === undefined) return null
  if (before !== undefined && after !== undefined && isSameJsonValue(before, after)) return null
  return Object.freeze({
    id,
    ...(before === undefined ? {} : { before: clone(before) }),
    ...(after === undefined ? {} : { after: clone(after) }),
  })
}

function createCollectionCommitPatch<Record, Id extends string>(params: {
  writes: TransactionRecordWrites<Id, Record>
  beforeRecords: Map<Id, Record>
  orderPatches: ReadonlyMap<Id, CanvasGraphOrderPatch<Id>>
  clone: (record: Record) => Record
}): CanvasGraphCollectionPatch<Record, Id> {
  const records = Array.from(params.writes, ([id, after]) =>
    cloneRecordPatch(id, params.beforeRecords.get(id), after ?? undefined, params.clone),
  ).filter((patch): patch is CanvasGraphRecordPatch<Record, Id> => patch !== null)
  const order = Array.from(params.orderPatches.values(), (patch) => Object.freeze({ ...patch }))
  return Object.freeze({
    records: Object.freeze(records),
    order: Object.freeze(order),
  })
}

export function createCommitPatch(draft: CanvasGraphTransactionDraft): CanvasGraphCommitPatch {
  return Object.freeze({
    nodes: createCollectionCommitPatch({
      writes: draft.nodeWrites,
      beforeRecords: draft.base.nodesById,
      orderPatches: draft.nodeOrderPatches,
      clone: cloneNodeRecord,
    }),
    edges: createCollectionCommitPatch({
      writes: draft.edgeWrites,
      beforeRecords: draft.base.edgesById,
      orderPatches: draft.edgeOrderPatches,
      clone: cloneEdgeRecord,
    }),
    groups: createCollectionCommitPatch({
      writes: draft.groupWrites,
      beforeRecords: draft.base.groupsById,
      orderPatches: draft.groupOrderPatches,
      clone: cloneGroupRecord,
    }),
  })
}

export function selectCommitPatchRecords<Record, Id extends string>(
  collection: CanvasGraphCollectionPatch<Record, Id>,
  writes: TransactionRecordWrites<Id, Record>,
  clone: (record: Record) => Record,
  direction: CanvasGraphPatchDirection,
): void {
  for (const recordPatch of collection.records) {
    const next = direction === 'forward' ? recordPatch.after : recordPatch.before
    writes.set(recordPatch.id, next === undefined ? null : clone(next))
  }
}

export function applyCommitPatchOrder<Id extends string>(
  current: readonly Id[],
  orderPatch: readonly CanvasGraphOrderPatch<Id>[],
  direction: CanvasGraphPatchDirection,
): Id[] | undefined {
  if (orderPatch.length === 0) return undefined
  const targetPlacement = (patchItem: CanvasGraphOrderPatch<Id>) =>
    direction === 'forward' ? patchItem.after : patchItem.before
  const next = [...current]
  const orderedPatches = direction === 'forward' ? orderPatch : [...orderPatch].reverse()
  for (const patchItem of orderedPatches) {
    const placement = targetPlacement(patchItem)
    const currentIndex = next.indexOf(patchItem.id)
    if (!placement) {
      if (currentIndex >= 0) next.splice(currentIndex, 1)
      continue
    }
    if (currentIndex >= 0) next.splice(currentIndex, 1)
    const previousIndex = placement.previousId ? next.indexOf(placement.previousId) : -1
    const nextIndex = placement.nextId ? next.indexOf(placement.nextId) : -1
    const insertIndex =
      previousIndex >= 0 ? previousIndex + 1 : nextIndex >= 0 ? nextIndex : next.length
    next.splice(insertIndex, 0, patchItem.id)
  }
  return next
}

export function recordAppliedOrderPatch<Id extends string>(
  before: readonly Id[],
  after: readonly Id[] | undefined,
  target: Map<Id, CanvasGraphOrderPatch<Id>>,
): void {
  if (!after) return
  const ids = new Set([...before, ...after])
  const placement = (order: readonly Id[], id: Id) => {
    const index = order.indexOf(id)
    if (index < 0) return undefined
    return {
      ...(order[index - 1] === undefined ? {} : { previousId: order[index - 1] }),
      ...(order[index + 1] === undefined ? {} : { nextId: order[index + 1] }),
    }
  }
  ids.forEach((id) => {
    const beforePlacement = placement(before, id)
    const afterPlacement = placement(after, id)
    if (isSameJsonValue(beforePlacement, afterPlacement)) return
    target.set(id, {
      id,
      ...(beforePlacement ? { before: beforePlacement } : {}),
      ...(afterPlacement ? { after: afterPlacement } : {}),
    })
  })
}

/**
 * Whether a stored commit patch still applies cleanly to the given state in
 * the given direction. Moved out of the store so history/undo-redo
 * precondition checks share the single patch-algebra implementation.
 */
export function canApplyCommitPatch(
  state: CanvasGraphStoreState,
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
    matchesRecords(state.nodesById, patch.nodes) &&
    matchesRecords(state.edgesById, patch.edges) &&
    matchesRecords(state.groupsById, patch.groups) &&
    matchesOrder(state.nodeOrder, patch.nodes.order) &&
    matchesOrder(state.edgeOrder, patch.edges.order) &&
    matchesOrder(state.groupOrder, patch.groups.order)
  )
}
