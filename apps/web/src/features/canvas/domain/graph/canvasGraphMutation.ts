import type { CanvasConnection, CanvasNodeItem } from '../../../board/ports/canvas'
import { cloneJsonValue } from './canvasGraphClone'
import { hasSameGroupRecord, isSameGroupGeometry } from './canvasGraphRecordEquality'
import {
  addEdgeToIndexes,
  addGroupToIndexes,
  getEdgeEndpointIds,
  hasSameEdgeTopology,
  hasSameGroupMembership,
  removeEdgeFromIndexes,
  removeGroupFromIndexes,
} from './canvasGraphStoreIndexes'
import { getNodeKind } from './canvasGraphStoreState'
import {
  toCanvasGraphEdgeId,
  toCanvasGraphGroupId,
  toCanvasGraphNodeId,
} from './canvasGraphStoreIds'
import type {
  CanvasGraphEdgeId,
  CanvasGraphEdgeRecord,
  CanvasGraphGroupGeometry,
  CanvasGraphGroupId,
  CanvasGraphGroupRecord,
  CanvasGraphNodeId,
  CanvasGraphNodeRecord,
  CanvasGraphNodeUpdate,
  CanvasGraphOrderPatch,
  CanvasGraphStoreState,
  CanvasGraphTransactionDraft,
  TransactionRecordWrites,
} from './canvasGraphStoreTypes'

/**
 * Graph mutation runtime for the canvas graph transaction subsystem.
 *
 * Owns record semantics and draft staging: applying writes to canonical
 * state, validating/normalizing node updates and group geometry, and reading
 * or staging draft records and order journals. Pure with respect to the
 * draft it is given — no patch math (see `canvasGraphPatchAlgebra`) and no
 * summary building (see `canvasGraphHistoryProjection`).
 */

export function commitNodeWrites(
  state: CanvasGraphStoreState,
  nodeWrites: TransactionRecordWrites<CanvasGraphNodeId, CanvasGraphNodeRecord>,
): void {
  for (const [nodeId, nextRecord] of nodeWrites) {
    const previousRecord = state.nodesById.get(nodeId)
    if (!nextRecord) {
      if (previousRecord) {
        state.nodesById.delete(nodeId)
        state.indexes.nodeIdsByKind.get(previousRecord.kind)?.delete(nodeId)
        state.indexes.groupIdByNodeId.delete(nodeId)
      }
      continue
    }
    state.nodesById.set(nodeId, nextRecord)
    if (!previousRecord) state.indexes.nodeIdsByKind.get(nextRecord.kind)?.add(nodeId)
  }
}

export function commitEdgeWrites(
  state: CanvasGraphStoreState,
  edgeWrites: TransactionRecordWrites<CanvasGraphEdgeId, CanvasGraphEdgeRecord>,
): void {
  for (const [edgeId, nextRecord] of edgeWrites) {
    const previousRecord = state.edgesById.get(edgeId)
    const didChangeTopology =
      !previousRecord || !nextRecord || !hasSameEdgeTopology(previousRecord, nextRecord)
    if (previousRecord && didChangeTopology) {
      removeEdgeFromIndexes(state.indexes, previousRecord)
    }
    if (!nextRecord) {
      state.edgesById.delete(edgeId)
      continue
    }
    state.edgesById.set(edgeId, nextRecord)
    if (didChangeTopology) addEdgeToIndexes(state.indexes, nextRecord)
  }
}

export function commitGroupWrites(
  state: CanvasGraphStoreState,
  groupWrites: TransactionRecordWrites<CanvasGraphGroupId, CanvasGraphGroupRecord>,
): void {
  for (const [groupId, nextRecord] of groupWrites) {
    const previousRecord = state.groupsById.get(groupId)
    const didChangeMembership =
      !previousRecord || !nextRecord || !hasSameGroupMembership(previousRecord, nextRecord)
    if (previousRecord && didChangeMembership) {
      removeGroupFromIndexes(state.indexes, previousRecord)
    }
    if (!nextRecord) {
      state.groupsById.delete(groupId)
      continue
    }
    state.groupsById.set(groupId, nextRecord)
    if (didChangeMembership) {
      addGroupToIndexes(state.indexes, nextRecord, state.nodesById)
    }
  }
}

export function applyNodeUpdate(
  current: CanvasNodeItem,
  update: CanvasGraphNodeUpdate,
): CanvasNodeItem {
  if (typeof update === 'function') return cloneJsonValue(update(cloneJsonValue(current)))
  return {
    ...cloneJsonValue(current),
    ...update,
    id: current.id,
    type: current.type,
  } as CanvasNodeItem
}

export function normalizeGroupGeometry(
  geometry: CanvasGraphGroupGeometry,
): CanvasGraphGroupGeometry {
  const { x, y, width, height } = geometry
  if (
    !Number.isFinite(x) ||
    !Number.isFinite(y) ||
    !Number.isFinite(width) ||
    !Number.isFinite(height) ||
    width <= 0 ||
    height <= 0
  ) {
    throw new Error('CanvasGraphStore group geometry must be finite and positive')
  }
  return { x, y, width, height }
}

export { hasSameGroupRecord, isSameGroupGeometry }

export function readDraftRecord<Id extends string, Record>(
  writes: TransactionRecordWrites<Id, Record>,
  baseRecords: Map<Id, Record>,
  id: Id,
): Record | undefined {
  if (writes.has(id)) return writes.get(id) ?? undefined
  return baseRecords.get(id)
}

export function readDraftNodeRecord(
  draft: CanvasGraphTransactionDraft,
  nodeId: CanvasGraphNodeId,
): CanvasGraphNodeRecord | undefined {
  return readDraftRecord(draft.nodeWrites, draft.base.nodesById, nodeId)
}

export function readDraftEdgeRecord(
  draft: CanvasGraphTransactionDraft,
  edgeId: CanvasGraphEdgeId,
): CanvasGraphEdgeRecord | undefined {
  return readDraftRecord(draft.edgeWrites, draft.base.edgesById, edgeId)
}

export function readDraftGroupRecord(
  draft: CanvasGraphTransactionDraft,
  groupId: CanvasGraphGroupId,
): CanvasGraphGroupRecord | undefined {
  return readDraftRecord(draft.groupWrites, draft.base.groupsById, groupId)
}

export function getDraftConnectedEdgeIds(
  draft: CanvasGraphTransactionDraft,
  nodeId: CanvasGraphNodeId,
): Set<CanvasGraphEdgeId> {
  const edgeIds = new Set([
    ...(draft.base.indexes.outgoingEdgesByNodeId.get(nodeId) ?? []),
    ...(draft.base.indexes.incomingEdgesByNodeId.get(nodeId) ?? []),
  ])
  for (const [edgeId, nextRecord] of draft.edgeWrites) {
    const previousRecord = draft.base.edgesById.get(edgeId)
    if (previousRecord) {
      const { sourceId, targetId } = getEdgeEndpointIds(previousRecord)
      if (sourceId === nodeId || targetId === nodeId) edgeIds.delete(edgeId)
    }
    if (nextRecord) {
      const { sourceId, targetId } = getEdgeEndpointIds(nextRecord)
      if (sourceId === nodeId || targetId === nodeId) edgeIds.add(edgeId)
    }
  }
  return edgeIds
}

export function getDraftGroupIdForNode(
  draft: CanvasGraphTransactionDraft,
  nodeId: CanvasGraphNodeId,
): CanvasGraphGroupId | undefined {
  for (const [groupId, group] of draft.groupWrites) {
    if (group?.nodeIds.includes(nodeId)) return groupId
  }
  const groupId = draft.base.indexes.groupIdByNodeId.get(nodeId)
  if (!groupId) return undefined
  const group = readDraftGroupRecord(draft, groupId)
  return group?.nodeIds.includes(nodeId) ? groupId : undefined
}

export function readDraftGroupOrder(draft: CanvasGraphTransactionDraft): CanvasGraphGroupId[] {
  return draft.groupOrder ?? draft.base.groupOrder
}

export function ensureNodeOrder(draft: CanvasGraphTransactionDraft): CanvasGraphNodeId[] {
  return (draft.nodeOrder ??= [...draft.base.nodeOrder])
}

export function ensureEdgeOrder(draft: CanvasGraphTransactionDraft): CanvasGraphEdgeId[] {
  return (draft.edgeOrder ??= [...draft.base.edgeOrder])
}

export function ensureGroupOrder(draft: CanvasGraphTransactionDraft): CanvasGraphGroupId[] {
  return (draft.groupOrder ??= [...draft.base.groupOrder])
}

export function recordOrderAdded<Id extends string>(
  patches: Map<Id, CanvasGraphOrderPatch<Id>>,
  id: Id,
  order: readonly Id[],
) {
  const previous = patches.get(id)
  const placement = Object.freeze({
    ...(order.at(-1) === undefined ? {} : { previousId: order.at(-1) }),
  })
  patches.set(
    id,
    Object.freeze({
      id,
      ...(previous?.before === undefined ? {} : { before: previous.before }),
      after: placement,
    }),
  )
}

export function removeFromOrder<Id extends string>(
  order: Id[],
  id: Id,
  patches: Map<Id, CanvasGraphOrderPatch<Id>>,
) {
  const index = order.indexOf(id)
  if (index < 0) return
  const previous = patches.get(id)
  // An item added and removed in one draft has no net order delta.
  if (previous?.before === undefined && previous?.after !== undefined) {
    patches.delete(id)
    order.splice(index, 1)
    return
  }
  patches.set(
    id,
    Object.freeze({
      id,
      before: Object.freeze({
        ...(order[index - 1] === undefined ? {} : { previousId: order[index - 1] }),
        ...(order[index + 1] === undefined ? {} : { nextId: order[index + 1] }),
      }),
    }),
  )
  order.splice(index, 1)
}

export function assertDraftConnectionEndpointsExist(
  draft: CanvasGraphTransactionDraft,
  connection: CanvasConnection,
) {
  const sourceId = toCanvasGraphNodeId(connection.fromNodeType, connection.fromNodeId)
  const targetId = toCanvasGraphNodeId(connection.toNodeType, connection.toNodeId)
  if (!readDraftNodeRecord(draft, sourceId) || !readDraftNodeRecord(draft, targetId)) {
    throw new Error(`CanvasGraphStore connection has missing endpoint: ${connection.id}`)
  }
}

export function createNextGroupId(draft: CanvasGraphTransactionDraft): CanvasGraphGroupId {
  let maxId = 0
  for (const groupId of readDraftGroupOrder(draft)) {
    const match = /^group-(\d+)$/.exec(groupId)
    if (!match) continue
    maxId = Math.max(maxId, Number(match[1]))
  }
  return toCanvasGraphGroupId(maxId + 1)
}

export function normalizeNewGroupNodeIds(
  draft: CanvasGraphTransactionDraft,
  nodeIds: CanvasGraphNodeId[],
): CanvasGraphNodeId[] {
  const normalizedNodeIds: CanvasGraphNodeId[] = []
  const seenNodeIds = new Set<CanvasGraphNodeId>()
  for (const nodeId of nodeIds) {
    if (seenNodeIds.has(nodeId)) continue
    if (!readDraftNodeRecord(draft, nodeId)) {
      throw new Error(`CanvasGraphStore group node does not exist: ${nodeId}`)
    }
    const existingGroupId = getDraftGroupIdForNode(draft, nodeId)
    if (existingGroupId) {
      throw new Error(`CanvasGraphStore node already belongs to group: ${nodeId}`)
    }
    seenNodeIds.add(nodeId)
    normalizedNodeIds.push(nodeId)
  }
  if (normalizedNodeIds.length < 2) {
    throw new Error('CanvasGraphStore group requires at least two nodes')
  }
  return normalizedNodeIds
}

export function stageAddNode(
  draft: CanvasGraphTransactionDraft,
  node: CanvasNodeItem,
): CanvasGraphNodeId {
  const kind = getNodeKind(node)
  const id = toCanvasGraphNodeId(kind, node.id)
  if (readDraftNodeRecord(draft, id)) {
    throw new Error(`CanvasGraphStore node already exists: ${id}`)
  }
  draft.nodeWrites.set(id, {
    id,
    kind,
    snapshot: cloneJsonValue(node),
    selected: false,
  })
  const order = ensureNodeOrder(draft)
  recordOrderAdded(draft.nodeOrderPatches, id, order)
  order.push(id)
  return id
}

export function stageUpdateNode(
  draft: CanvasGraphTransactionDraft,
  nodeId: CanvasGraphNodeId,
  update: CanvasGraphNodeUpdate,
): void {
  const current = readDraftNodeRecord(draft, nodeId)
  if (!current) return
  const nextSnapshot = applyNodeUpdate(current.snapshot, update)
  const nextKind = getNodeKind(nextSnapshot)
  if (nextKind !== current.kind || nextSnapshot.id !== current.snapshot.id) {
    throw new Error(`CanvasGraphStore cannot change node identity: ${nodeId}`)
  }
  draft.nodeWrites.set(nodeId, {
    ...current,
    snapshot: nextSnapshot,
  })
}

export function stageRemoveNode(
  draft: CanvasGraphTransactionDraft,
  nodeId: CanvasGraphNodeId,
  options: { removeConnectedEdges?: boolean } = {},
): void {
  if (!readDraftNodeRecord(draft, nodeId)) return
  if (options.removeConnectedEdges) {
    getDraftConnectedEdgeIds(draft, nodeId).forEach((edgeId) => stageRemoveEdge(draft, edgeId))
  }
  removeNodeFromGroupsWrite(draft, nodeId)
  draft.nodeWrites.set(nodeId, null)
  removeFromOrder(ensureNodeOrder(draft), nodeId, draft.nodeOrderPatches)
}

export function stageSelectNode(
  draft: CanvasGraphTransactionDraft,
  nodeId: CanvasGraphNodeId,
  selected: boolean,
): void {
  const current = readDraftNodeRecord(draft, nodeId)
  if (!current || current.selected === selected) return
  draft.nodeWrites.set(nodeId, {
    ...current,
    selected,
  })
}

export function stageCreateGroup(
  draft: CanvasGraphTransactionDraft,
  nodeIds: CanvasGraphNodeId[],
  geometry: CanvasGraphGroupGeometry,
  groupId?: CanvasGraphGroupId,
): CanvasGraphGroupId {
  const resolvedGroupId = groupId ?? createNextGroupId(draft)
  if (readDraftGroupRecord(draft, resolvedGroupId)) {
    throw new Error(`CanvasGraphStore group already exists: ${resolvedGroupId}`)
  }
  const normalizedNodeIds = normalizeNewGroupNodeIds(draft, nodeIds)
  const normalizedGeometry = normalizeGroupGeometry(geometry)
  draft.groupWrites.set(resolvedGroupId, {
    id: resolvedGroupId,
    nodeIds: normalizedNodeIds,
    ...normalizedGeometry,
  })
  const order = ensureGroupOrder(draft)
  recordOrderAdded(draft.groupOrderPatches, resolvedGroupId, order)
  order.push(resolvedGroupId)
  return resolvedGroupId
}

export function stageUpdateGroupGeometry(
  draft: CanvasGraphTransactionDraft,
  groupId: CanvasGraphGroupId,
  geometry: CanvasGraphGroupGeometry,
): void {
  const current = readDraftGroupRecord(draft, groupId)
  if (!current) return
  const normalizedGeometry = normalizeGroupGeometry(geometry)
  if (isSameGroupGeometry(current, normalizedGeometry)) return
  draft.groupWrites.set(groupId, {
    ...current,
    ...normalizedGeometry,
  })
}

export function removeGroupWrite(
  draft: CanvasGraphTransactionDraft,
  groupId: CanvasGraphGroupId,
): void {
  draft.groupWrites.set(groupId, null)
  removeFromOrder(ensureGroupOrder(draft), groupId, draft.groupOrderPatches)
}

export function stageUngroup(
  draft: CanvasGraphTransactionDraft,
  groupId: CanvasGraphGroupId,
): void {
  if (!readDraftGroupRecord(draft, groupId)) return
  removeGroupWrite(draft, groupId)
}

export function addNodeToGroupWrite(
  draft: CanvasGraphTransactionDraft,
  nodeId: CanvasGraphNodeId,
  groupId: CanvasGraphGroupId,
): void {
  const group = readDraftGroupRecord(draft, groupId)
  if (!group) {
    throw new Error(`CanvasGraphStore group does not exist: ${groupId}`)
  }
  if (group.nodeIds.includes(nodeId)) return
  draft.groupWrites.set(groupId, {
    ...group,
    nodeIds: [...group.nodeIds, nodeId],
  })
}

export function removeNodeFromGroupWrite(
  draft: CanvasGraphTransactionDraft,
  nodeId: CanvasGraphNodeId,
  groupId: CanvasGraphGroupId,
): void {
  const group = readDraftGroupRecord(draft, groupId)
  if (!group?.nodeIds.includes(nodeId)) return
  const nextNodeIds = group.nodeIds.filter((id) => id !== nodeId)
  draft.groupWrites.set(groupId, {
    ...group,
    nodeIds: nextNodeIds,
  })
}

export function removeNodeFromGroupsWrite(
  draft: CanvasGraphTransactionDraft,
  nodeId: CanvasGraphNodeId,
): void {
  const groupId = getDraftGroupIdForNode(draft, nodeId)
  if (groupId) removeNodeFromGroupWrite(draft, nodeId, groupId)
}

export function stageMoveNodeToGroup(
  draft: CanvasGraphTransactionDraft,
  nodeId: CanvasGraphNodeId,
  groupId: CanvasGraphGroupId | null,
): void {
  if (!readDraftNodeRecord(draft, nodeId)) return

  const currentGroupId = getDraftGroupIdForNode(draft, nodeId)
  if (currentGroupId === groupId) return

  if (currentGroupId) {
    removeNodeFromGroupWrite(draft, nodeId, currentGroupId)
  }
  if (groupId) {
    addNodeToGroupWrite(draft, nodeId, groupId)
  }
}

export function stageAddEdge(
  draft: CanvasGraphTransactionDraft,
  connection: CanvasConnection,
): CanvasGraphEdgeId {
  const id = toCanvasGraphEdgeId(connection.id)
  if (readDraftEdgeRecord(draft, id)) {
    throw new Error(`CanvasGraphStore edge already exists: ${id}`)
  }
  assertDraftConnectionEndpointsExist(draft, connection)
  draft.edgeWrites.set(id, {
    id,
    connection: cloneJsonValue(connection),
    selected: false,
  })
  const order = ensureEdgeOrder(draft)
  recordOrderAdded(draft.edgeOrderPatches, id, order)
  order.push(id)
  return id
}

export function stageRemoveEdge(
  draft: CanvasGraphTransactionDraft,
  edgeId: CanvasGraphEdgeId,
): void {
  if (!readDraftEdgeRecord(draft, edgeId)) return
  draft.edgeWrites.set(edgeId, null)
  removeFromOrder(ensureEdgeOrder(draft), edgeId, draft.edgeOrderPatches)
}

export function stageSelectEdge(
  draft: CanvasGraphTransactionDraft,
  edgeId: CanvasGraphEdgeId,
  selected: boolean,
): void {
  const current = readDraftEdgeRecord(draft, edgeId)
  if (!current || current.selected === selected) return
  draft.edgeWrites.set(edgeId, {
    ...current,
    selected,
  })
}

export function stageBatchDelete(
  draft: CanvasGraphTransactionDraft,
  options: {
    nodeIds?: CanvasGraphNodeId[]
    edgeIds?: CanvasGraphEdgeId[]
    removeConnectedEdges?: boolean
  },
): void {
  options.edgeIds?.forEach((edgeId) => {
    stageRemoveEdge(draft, edgeId)
  })
  options.nodeIds?.forEach((nodeId) => {
    stageRemoveNode(draft, nodeId, {
      removeConnectedEdges: options.removeConnectedEdges ?? true,
    })
  })
}
