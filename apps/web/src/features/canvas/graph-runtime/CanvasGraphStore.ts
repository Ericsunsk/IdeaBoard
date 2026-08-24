import {
  createEmptyCanvasBoardSnapshot,
  type CanvasBoardSnapshot,
  type CanvasConnection,
  type CanvasConnectionNodeType,
  type CanvasGroup,
  type CanvasNodeItem,
  type FusionNodeItem,
  type ImageNodeItem,
  type PromptNodeItem,
} from '../../canvas-board/domain'
import { measureCanvasPerf } from '../utils/canvasPerfProbe'

export type CanvasGraphNodeKind = CanvasConnectionNodeType
export type CanvasGraphNodeId = `${CanvasGraphNodeKind}-${number}`
export type CanvasGraphNodeIdForKind<T extends CanvasGraphNodeKind> = `${T}-${number}`
export type CanvasGraphEdgeId = `connection-${number}`
export type CanvasGraphGroupId = CanvasGroup['id']

export type CanvasGraphTransactionReason =
  | 'node-drag'
  | 'connect'
  | 'disconnect'
  | 'delete'
  | 'batch-delete'
  | 'node-update'
  | 'result-materialize'
  | 'group'
  | 'ungroup'
  | 'group-delete'
  | 'group-move'
  | 'group-resize'
  | 'group-membership'
  | 'hydrate'

export type CanvasGraphNodeRecord = {
  id: CanvasGraphNodeId
  kind: CanvasGraphNodeKind
  snapshot: CanvasNodeItem
  selected: boolean
}

export type CanvasGraphEdgeRecord = {
  id: CanvasGraphEdgeId
  connection: CanvasConnection
  selected: boolean
}

export type CanvasGraphGroupRecord = {
  id: CanvasGraphGroupId
  nodeIds: CanvasGraphNodeId[]
  x: number
  y: number
  width: number
  height: number
}

export type CanvasGraphGroupGeometry = Pick<CanvasGraphGroupRecord, 'x' | 'y' | 'width' | 'height'>

export type CanvasGraphProjectionPatch = {
  nodes: {
    added: CanvasGraphNodeRecord[]
    updated: CanvasGraphNodeRecord[]
    removedIds: CanvasGraphNodeId[]
    selectionChanged: Array<{ id: CanvasGraphNodeId; selected: boolean }>
  }
  edges: {
    added: CanvasGraphEdgeRecord[]
    removedIds: CanvasGraphEdgeId[]
    selectionChanged: Array<{ id: CanvasGraphEdgeId; selected: boolean }>
  }
  groups: {
    added: CanvasGraphGroupRecord[]
    updated: CanvasGraphGroupRecord[]
    removedIds: CanvasGraphGroupId[]
  }
}

export type CanvasGraphTransactionSummary = {
  id: string
  reason: CanvasGraphTransactionReason
  didChange: boolean
  hasDomainChanges: boolean
  changedNodeIds: CanvasGraphNodeId[]
  changedEdgeIds: CanvasGraphEdgeId[]
  changedGroupIds: CanvasGraphGroupId[]
  removedNodeIds: CanvasGraphNodeId[]
  removedEdgeIds: CanvasGraphEdgeId[]
  removedGroupIds: CanvasGraphGroupId[]
  selectedNodeIds: CanvasGraphNodeId[]
  selectedEdgeIds: CanvasGraphEdgeId[]
  didChangeOrder: boolean
  shouldRecordHistory: boolean
  shouldMarkDirty: boolean
  projectionPatch: CanvasGraphProjectionPatch
}

export type CanvasGraphTransactionOptions = {
  recordHistory?: boolean
  markDirty?: boolean
}

export type CanvasGraphNodeUpdate =
  | Partial<Omit<ImageNodeItem, 'id' | 'type'>>
  | Partial<Omit<PromptNodeItem, 'id' | 'type'>>
  | Partial<Omit<FusionNodeItem, 'id' | 'type'>>
  | ((node: CanvasNodeItem) => CanvasNodeItem)

export type CanvasGraphRemoveNodeOptions = {
  removeConnectedEdges?: boolean
}

type CanvasGraphStoreState = {
  nodesById: Map<CanvasGraphNodeId, CanvasGraphNodeRecord>
  edgesById: Map<CanvasGraphEdgeId, CanvasGraphEdgeRecord>
  groupsById: Map<CanvasGraphGroupId, CanvasGraphGroupRecord>
  nodeOrder: CanvasGraphNodeId[]
  edgeOrder: CanvasGraphEdgeId[]
  groupOrder: CanvasGraphGroupId[]
  indexes: CanvasGraphIndexes
}

type CanvasGraphIndexes = {
  outgoingEdgesByNodeId: Map<CanvasGraphNodeId, Set<CanvasGraphEdgeId>>
  incomingEdgesByNodeId: Map<CanvasGraphNodeId, Set<CanvasGraphEdgeId>>
  nodeIdsByKind: Map<CanvasGraphNodeKind, Set<CanvasGraphNodeId>>
  groupIdByNodeId: Map<CanvasGraphNodeId, CanvasGraphGroupId>
}

type TransactionRecordWrites<Id extends string, Record> = Map<Id, Record | null>

type CanvasGraphTransactionDraft = {
  base: CanvasGraphStoreState
  baseRevision: number
  nodeWrites: TransactionRecordWrites<CanvasGraphNodeId, CanvasGraphNodeRecord>
  edgeWrites: TransactionRecordWrites<CanvasGraphEdgeId, CanvasGraphEdgeRecord>
  groupWrites: TransactionRecordWrites<CanvasGraphGroupId, CanvasGraphGroupRecord>
  nodeOrder?: CanvasGraphNodeId[]
  edgeOrder?: CanvasGraphEdgeId[]
  groupOrder?: CanvasGraphGroupId[]
}

type TransactionChangeSet = {
  addedNodeIds: Set<CanvasGraphNodeId>
  updatedNodeIds: Set<CanvasGraphNodeId>
  removedNodeIds: Set<CanvasGraphNodeId>
  selectedNodeIds: Set<CanvasGraphNodeId>
  addedEdgeIds: Set<CanvasGraphEdgeId>
  removedEdgeIds: Set<CanvasGraphEdgeId>
  selectedEdgeIds: Set<CanvasGraphEdgeId>
  addedGroupIds: Set<CanvasGraphGroupId>
  updatedGroupIds: Set<CanvasGraphGroupId>
  removedGroupIds: Set<CanvasGraphGroupId>
  didChangeNodeOrder: boolean
  didChangeEdgeOrder: boolean
  didChangeGroupOrder: boolean
  hasDomainChanges: boolean
}

let transactionSequence = 0

function cloneJson<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

export function toCanvasGraphNodeId(kind: CanvasGraphNodeKind, id: number): CanvasGraphNodeId {
  return `${kind}-${id}` as CanvasGraphNodeId
}

export function toCanvasGraphEdgeId(id: number): CanvasGraphEdgeId {
  return `connection-${id}` as CanvasGraphEdgeId
}

export function toCanvasGraphGroupId(id: number): CanvasGraphGroupId {
  return `group-${id}` as CanvasGraphGroupId
}

function getNodeKind(node: CanvasNodeItem): CanvasGraphNodeKind {
  return node.type === 'image-node' ? 'image' : node.type
}

function getNodeCollection(
  snapshot: CanvasBoardSnapshot,
  kind: CanvasGraphNodeKind,
): CanvasNodeItem[] {
  if (kind === 'image') return snapshot.imageNodes
  if (kind === 'prompt') return snapshot.promptNodes
  return snapshot.fusionNodes
}

function addToSetMap<K, V>(map: Map<K, Set<V>>, key: K, value: V) {
  const existing = map.get(key)
  if (existing) {
    existing.add(value)
    return
  }
  map.set(key, new Set([value]))
}

function removeFromSetMap<K, V>(map: Map<K, Set<V>>, key: K, value: V) {
  const existing = map.get(key)
  if (!existing) return
  existing.delete(value)
  if (existing.size === 0) map.delete(key)
}

function getEdgeEndpointIds(record: CanvasGraphEdgeRecord): {
  sourceId: CanvasGraphNodeId
  targetId: CanvasGraphNodeId
} {
  return {
    sourceId: toCanvasGraphNodeId(record.connection.fromNodeType, record.connection.fromNodeId),
    targetId: toCanvasGraphNodeId(record.connection.toNodeType, record.connection.toNodeId),
  }
}

function addEdgeToIndexes(indexes: CanvasGraphIndexes, record: CanvasGraphEdgeRecord) {
  const { sourceId, targetId } = getEdgeEndpointIds(record)
  addToSetMap(indexes.outgoingEdgesByNodeId, sourceId, record.id)
  addToSetMap(indexes.incomingEdgesByNodeId, targetId, record.id)
}

function removeEdgeFromIndexes(indexes: CanvasGraphIndexes, record: CanvasGraphEdgeRecord) {
  const { sourceId, targetId } = getEdgeEndpointIds(record)
  removeFromSetMap(indexes.outgoingEdgesByNodeId, sourceId, record.id)
  removeFromSetMap(indexes.incomingEdgesByNodeId, targetId, record.id)
}

function hasSameEdgeTopology(left: CanvasGraphEdgeRecord, right: CanvasGraphEdgeRecord): boolean {
  const leftEndpoints = getEdgeEndpointIds(left)
  const rightEndpoints = getEdgeEndpointIds(right)
  return (
    leftEndpoints.sourceId === rightEndpoints.sourceId &&
    leftEndpoints.targetId === rightEndpoints.targetId
  )
}

function addGroupToIndexes(
  indexes: CanvasGraphIndexes,
  record: CanvasGraphGroupRecord,
  nodesById: Map<CanvasGraphNodeId, CanvasGraphNodeRecord>,
) {
  for (const nodeId of record.nodeIds) {
    if (nodesById.has(nodeId)) indexes.groupIdByNodeId.set(nodeId, record.id)
  }
}

function removeGroupFromIndexes(indexes: CanvasGraphIndexes, record: CanvasGraphGroupRecord) {
  for (const nodeId of record.nodeIds) {
    if (indexes.groupIdByNodeId.get(nodeId) === record.id) {
      indexes.groupIdByNodeId.delete(nodeId)
    }
  }
}

function hasSameGroupMembership(
  left: CanvasGraphGroupRecord,
  right: CanvasGraphGroupRecord,
): boolean {
  return (
    left.nodeIds.length === right.nodeIds.length &&
    left.nodeIds.every((nodeId, index) => nodeId === right.nodeIds[index])
  )
}

function createEmptyIndexes(): CanvasGraphIndexes {
  return {
    outgoingEdgesByNodeId: new Map(),
    incomingEdgesByNodeId: new Map(),
    nodeIdsByKind: new Map([
      ['image', new Set()],
      ['prompt', new Set()],
      ['fusion', new Set()],
    ]),
    groupIdByNodeId: new Map(),
  }
}

function rebuildIndexes(state: Omit<CanvasGraphStoreState, 'indexes'>): CanvasGraphIndexes {
  const indexes = createEmptyIndexes()
  for (const nodeId of state.nodeOrder) {
    const node = state.nodesById.get(nodeId)
    if (node) indexes.nodeIdsByKind.get(node.kind)?.add(nodeId)
  }
  for (const edgeId of state.edgeOrder) {
    const edge = state.edgesById.get(edgeId)
    if (!edge) continue
    const sourceId = toCanvasGraphNodeId(edge.connection.fromNodeType, edge.connection.fromNodeId)
    const targetId = toCanvasGraphNodeId(edge.connection.toNodeType, edge.connection.toNodeId)
    addToSetMap(indexes.outgoingEdgesByNodeId, sourceId, edgeId)
    addToSetMap(indexes.incomingEdgesByNodeId, targetId, edgeId)
  }
  for (const groupId of state.groupOrder) {
    const group = state.groupsById.get(groupId)
    if (!group) continue
    group.nodeIds.forEach((nodeId) => {
      if (state.nodesById.has(nodeId)) indexes.groupIdByNodeId.set(nodeId, groupId)
    })
  }
  return indexes
}

function createState(
  nodesById: Map<CanvasGraphNodeId, CanvasGraphNodeRecord>,
  edgesById: Map<CanvasGraphEdgeId, CanvasGraphEdgeRecord>,
  groupsById: Map<CanvasGraphGroupId, CanvasGraphGroupRecord>,
  nodeOrder: CanvasGraphNodeId[],
  edgeOrder: CanvasGraphEdgeId[],
  groupOrder: CanvasGraphGroupId[],
): CanvasGraphStoreState {
  return {
    nodesById,
    edgesById,
    groupsById,
    nodeOrder,
    edgeOrder,
    groupOrder,
    indexes: rebuildIndexes({
      nodesById,
      edgesById,
      groupsById,
      nodeOrder,
      edgeOrder,
      groupOrder,
    }),
  }
}

function cloneNodeRecord(record: CanvasGraphNodeRecord): CanvasGraphNodeRecord {
  return {
    id: record.id,
    kind: record.kind,
    snapshot: cloneJson(record.snapshot),
    selected: record.selected,
  }
}

function cloneEdgeRecord(record: CanvasGraphEdgeRecord): CanvasGraphEdgeRecord {
  return {
    id: record.id,
    connection: cloneJson(record.connection),
    selected: record.selected,
  }
}

function cloneGroupRecord(record: CanvasGraphGroupRecord): CanvasGraphGroupRecord {
  return {
    id: record.id,
    nodeIds: [...record.nodeIds],
    x: record.x,
    y: record.y,
    width: record.width,
    height: record.height,
  }
}

function createStateFromSnapshot(snapshot: CanvasBoardSnapshot): CanvasGraphStoreState {
  const nodesById = new Map<CanvasGraphNodeId, CanvasGraphNodeRecord>()
  const edgesById = new Map<CanvasGraphEdgeId, CanvasGraphEdgeRecord>()
  const groupsById = new Map<CanvasGraphGroupId, CanvasGraphGroupRecord>()
  const nodeOrder: CanvasGraphNodeId[] = []
  const edgeOrder: CanvasGraphEdgeId[] = []
  const groupOrder: CanvasGraphGroupId[] = []

  const appendNode = (node: CanvasNodeItem) => {
    const kind = getNodeKind(node)
    const id = toCanvasGraphNodeId(kind, node.id)
    if (nodesById.has(id)) return
    nodesById.set(id, {
      id,
      kind,
      snapshot: cloneJson(node),
      selected: false,
    })
    nodeOrder.push(id)
  }

  snapshot.imageNodes.forEach(appendNode)
  snapshot.promptNodes.forEach(appendNode)
  snapshot.fusionNodes.forEach(appendNode)

  for (const connection of snapshot.connections) {
    const sourceId = toCanvasGraphNodeId(connection.fromNodeType, connection.fromNodeId)
    const targetId = toCanvasGraphNodeId(connection.toNodeType, connection.toNodeId)
    if (!nodesById.has(sourceId) || !nodesById.has(targetId)) continue
    const id = toCanvasGraphEdgeId(connection.id)
    if (edgesById.has(id)) continue
    edgesById.set(id, {
      id,
      connection: cloneJson(connection),
      selected: false,
    })
    edgeOrder.push(id)
  }

  const groupedNodeIds = new Set<CanvasGraphNodeId>()
  for (const group of snapshot.groups) {
    if (groupsById.has(group.id)) continue
    const nodeIds: CanvasGraphNodeId[] = []
    const seenNodeIds = new Set<CanvasGraphNodeId>()
    for (const nodeId of group.nodeIds as CanvasGraphNodeId[]) {
      if (!nodesById.has(nodeId) || groupedNodeIds.has(nodeId) || seenNodeIds.has(nodeId)) {
        continue
      }
      seenNodeIds.add(nodeId)
      nodeIds.push(nodeId)
    }
    nodeIds.forEach((nodeId) => groupedNodeIds.add(nodeId))
    groupsById.set(group.id, {
      id: group.id,
      nodeIds,
      x: group.x,
      y: group.y,
      width: group.width,
      height: group.height,
    })
    groupOrder.push(group.id)
  }

  return createState(nodesById, edgesById, groupsById, nodeOrder, edgeOrder, groupOrder)
}

function createMeasuredStateFromSnapshot(snapshot: CanvasBoardSnapshot): CanvasGraphStoreState {
  return measureCanvasPerf(
    'graph-store.hydrate',
    () => ({
      nodes: snapshot.imageNodes.length + snapshot.promptNodes.length + snapshot.fusionNodes.length,
      edges: snapshot.connections.length,
      groups: snapshot.groups.length,
    }),
    () => createStateFromSnapshot(snapshot),
  )
}

function exportSnapshotFromState(
  state: CanvasGraphStoreState,
  base?: CanvasBoardSnapshot,
): CanvasBoardSnapshot {
  const snapshot = createEmptyCanvasBoardSnapshot()
  if (base?.mediaRefs) {
    snapshot.mediaRefs = base.mediaRefs.map((ref) => ({ ...ref }))
  }

  for (const nodeId of state.nodeOrder) {
    const record = state.nodesById.get(nodeId)
    if (!record) continue
    getNodeCollection(snapshot, record.kind).push(cloneJson(record.snapshot))
  }

  for (const edgeId of state.edgeOrder) {
    const record = state.edgesById.get(edgeId)
    if (!record) continue
    const sourceId = toCanvasGraphNodeId(
      record.connection.fromNodeType,
      record.connection.fromNodeId,
    )
    const targetId = toCanvasGraphNodeId(record.connection.toNodeType, record.connection.toNodeId)
    if (!state.nodesById.has(sourceId) || !state.nodesById.has(targetId)) continue
    snapshot.connections.push(cloneJson(record.connection))
  }

  for (const groupId of state.groupOrder) {
    const record = state.groupsById.get(groupId)
    if (!record) continue
    const nodeIds = record.nodeIds.filter((nodeId) => state.nodesById.has(nodeId))
    snapshot.groups.push({
      id: record.id,
      nodeIds: nodeIds as CanvasGroup['nodeIds'],
      x: record.x,
      y: record.y,
      width: record.width,
      height: record.height,
    })
  }

  return snapshot
}

function createChangeSet(): TransactionChangeSet {
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

function isSameOrder<Id>(left: readonly Id[], right: readonly Id[]): boolean {
  return left.length === right.length && left.every((id, index) => id === right[index])
}

function isSameJsonValue(left: unknown, right: unknown): boolean {
  return JSON.stringify(left) === JSON.stringify(right)
}

function normalizeTransactionChanges(draft: CanvasGraphTransactionDraft): TransactionChangeSet {
  const changes = createChangeSet()

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
    if (baseRecord.selected !== nextRecord.selected) changes.selectedNodeIds.add(nodeId)
  }

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
    if (baseRecord.selected !== nextRecord.selected) changes.selectedEdgeIds.add(edgeId)
  }

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
    if (
      baseRecord &&
      nextRecord &&
      (!hasSameGroupMembership(baseRecord, nextRecord) ||
        !isSameGroupGeometry(baseRecord, nextRecord))
    ) {
      changes.updatedGroupIds.add(groupId)
    }
  }

  changes.didChangeNodeOrder =
    !!draft.nodeOrder && !isSameOrder(draft.base.nodeOrder, draft.nodeOrder)
  changes.didChangeEdgeOrder =
    !!draft.edgeOrder && !isSameOrder(draft.base.edgeOrder, draft.edgeOrder)
  changes.didChangeGroupOrder =
    !!draft.groupOrder && !isSameOrder(draft.base.groupOrder, draft.groupOrder)
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
  return changes
}

function applyNodeUpdate(current: CanvasNodeItem, update: CanvasGraphNodeUpdate): CanvasNodeItem {
  if (typeof update === 'function') return cloneJson(update(cloneJson(current)))
  return {
    ...cloneJson(current),
    ...update,
    id: current.id,
    type: current.type,
  } as CanvasNodeItem
}

function normalizeGroupGeometry(geometry: CanvasGraphGroupGeometry): CanvasGraphGroupGeometry {
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

function isSameGroupGeometry(
  left: CanvasGraphGroupGeometry,
  right: CanvasGraphGroupGeometry,
): boolean {
  return (
    left.x === right.x &&
    left.y === right.y &&
    left.width === right.width &&
    left.height === right.height
  )
}

function createSummary(
  reason: CanvasGraphTransactionReason,
  options: Required<CanvasGraphTransactionOptions>,
  state: CanvasGraphStoreState,
  changes: TransactionChangeSet,
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
  const didChange =
    changes.hasDomainChanges || selectedNodeIds.length > 0 || selectedEdgeIds.length > 0

  return {
    id: `canvas-graph-tx-${++transactionSequence}`,
    reason,
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
    shouldRecordHistory: changes.hasDomainChanges && options.recordHistory,
    shouldMarkDirty: changes.hasDomainChanges && options.markDirty,
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
          .map((record) => ({ id: record.id, selected: record.selected })),
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
          .map((record) => ({ id: record.id, selected: record.selected })),
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

export class CanvasGraphStore {
  private state: CanvasGraphStoreState
  private revision = 0

  constructor(snapshot: CanvasBoardSnapshot = createEmptyCanvasBoardSnapshot()) {
    this.state = createMeasuredStateFromSnapshot(snapshot)
  }

  static fromSnapshot(snapshot: CanvasBoardSnapshot): CanvasGraphStore {
    return new CanvasGraphStore(snapshot)
  }

  hydrate(snapshot: CanvasBoardSnapshot) {
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

  exportSnapshot(base?: CanvasBoardSnapshot): CanvasBoardSnapshot {
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
      }),
    )
  }

  _commitTransaction(draft: CanvasGraphTransactionDraft) {
    this._assertTransactionCurrent(draft)

    for (const [nodeId, nextRecord] of draft.nodeWrites) {
      const previousRecord = this.state.nodesById.get(nodeId)
      if (!nextRecord) {
        if (previousRecord) {
          this.state.nodesById.delete(nodeId)
          this.state.indexes.nodeIdsByKind.get(previousRecord.kind)?.delete(nodeId)
          this.state.indexes.groupIdByNodeId.delete(nodeId)
        }
        continue
      }
      this.state.nodesById.set(nodeId, nextRecord)
      if (!previousRecord) this.state.indexes.nodeIdsByKind.get(nextRecord.kind)?.add(nodeId)
    }

    for (const [edgeId, nextRecord] of draft.edgeWrites) {
      const previousRecord = this.state.edgesById.get(edgeId)
      const didChangeTopology =
        !previousRecord || !nextRecord || !hasSameEdgeTopology(previousRecord, nextRecord)
      if (previousRecord && didChangeTopology) {
        removeEdgeFromIndexes(this.state.indexes, previousRecord)
      }
      if (!nextRecord) {
        this.state.edgesById.delete(edgeId)
        continue
      }
      this.state.edgesById.set(edgeId, nextRecord)
      if (didChangeTopology) addEdgeToIndexes(this.state.indexes, nextRecord)
    }

    for (const [groupId, nextRecord] of draft.groupWrites) {
      const previousRecord = this.state.groupsById.get(groupId)
      const didChangeMembership =
        !previousRecord || !nextRecord || !hasSameGroupMembership(previousRecord, nextRecord)
      if (previousRecord && didChangeMembership) {
        removeGroupFromIndexes(this.state.indexes, previousRecord)
      }
      if (!nextRecord) {
        this.state.groupsById.delete(groupId)
        continue
      }
      this.state.groupsById.set(groupId, nextRecord)
      if (didChangeMembership) {
        addGroupToIndexes(this.state.indexes, nextRecord, this.state.nodesById)
      }
    }

    if (draft.nodeOrder) this.state.nodeOrder = draft.nodeOrder
    if (draft.edgeOrder) this.state.edgeOrder = draft.edgeOrder
    if (draft.groupOrder) this.state.groupOrder = draft.groupOrder
    this.revision += 1
    this.state = { ...this.state }
  }

  _assertTransactionCurrent(draft: CanvasGraphTransactionDraft) {
    if (this.revision !== draft.baseRevision) {
      throw new Error('CanvasGraphStore transaction is stale')
    }
  }

  _readState(): CanvasGraphStoreState {
    return this.state
  }
}

export class CanvasGraphTransaction {
  private readonly store: CanvasGraphStore
  private readonly reason: CanvasGraphTransactionReason
  private readonly options: Required<CanvasGraphTransactionOptions>
  private readonly draft: CanvasGraphTransactionDraft
  private status: 'open' | 'committed' | 'cancelled' = 'open'

  constructor(
    store: CanvasGraphStore,
    reason: CanvasGraphTransactionReason,
    options: Required<CanvasGraphTransactionOptions>,
  ) {
    this.store = store
    this.reason = reason
    this.options = options
    this.draft = store._createTransactionDraft()
  }

  addNode(node: CanvasNodeItem): this {
    this.assertOpen()
    const kind = getNodeKind(node)
    const id = toCanvasGraphNodeId(kind, node.id)
    if (this.getNodeRecord(id)) {
      throw new Error(`CanvasGraphStore node already exists: ${id}`)
    }
    this.draft.nodeWrites.set(id, {
      id,
      kind,
      snapshot: cloneJson(node),
      selected: false,
    })
    this.ensureNodeOrder().push(id)
    return this
  }

  updateNode(nodeId: CanvasGraphNodeId, update: CanvasGraphNodeUpdate): this {
    this.assertOpen()
    const current = this.getNodeRecord(nodeId)
    if (!current) return this
    const nextSnapshot = applyNodeUpdate(current.snapshot, update)
    const nextKind = getNodeKind(nextSnapshot)
    if (nextKind !== current.kind || nextSnapshot.id !== current.snapshot.id) {
      throw new Error(`CanvasGraphStore cannot change node identity: ${nodeId}`)
    }
    this.draft.nodeWrites.set(nodeId, {
      ...current,
      snapshot: nextSnapshot,
    })
    return this
  }

  removeNode(nodeId: CanvasGraphNodeId, options: CanvasGraphRemoveNodeOptions = {}): this {
    this.assertOpen()
    if (!this.getNodeRecord(nodeId)) return this
    if (options.removeConnectedEdges) {
      this.getConnectedEdgeIds(nodeId).forEach((edgeId) => this.removeEdge(edgeId))
    }
    this.removeNodeFromGroups(nodeId)
    this.draft.nodeWrites.set(nodeId, null)
    this.removeFromOrder(this.ensureNodeOrder(), nodeId)
    return this
  }

  selectNode(nodeId: CanvasGraphNodeId, selected: boolean): this {
    this.assertOpen()
    const current = this.getNodeRecord(nodeId)
    if (!current || current.selected === selected) return this
    this.draft.nodeWrites.set(nodeId, {
      ...current,
      selected,
    })
    return this
  }

  createGroup(
    nodeIds: CanvasGraphNodeId[],
    geometry: CanvasGraphGroupGeometry,
    groupId?: CanvasGraphGroupId,
  ): this {
    this.assertOpen()
    const resolvedGroupId = groupId ?? this.createNextGroupId()
    if (this.getGroupRecord(resolvedGroupId)) {
      throw new Error(`CanvasGraphStore group already exists: ${resolvedGroupId}`)
    }
    const normalizedNodeIds = this.normalizeNewGroupNodeIds(nodeIds)
    const normalizedGeometry = normalizeGroupGeometry(geometry)
    this.draft.groupWrites.set(resolvedGroupId, {
      id: resolvedGroupId,
      nodeIds: normalizedNodeIds,
      ...normalizedGeometry,
    })
    this.ensureGroupOrder().push(resolvedGroupId)
    return this
  }

  updateGroupGeometry(groupId: CanvasGraphGroupId, geometry: CanvasGraphGroupGeometry): this {
    this.assertOpen()
    const current = this.getGroupRecord(groupId)
    if (!current) return this
    const normalizedGeometry = normalizeGroupGeometry(geometry)
    if (isSameGroupGeometry(current, normalizedGeometry)) return this
    this.draft.groupWrites.set(groupId, {
      ...current,
      ...normalizedGeometry,
    })
    return this
  }

  ungroup(groupId: CanvasGraphGroupId): this {
    this.assertOpen()
    if (!this.getGroupRecord(groupId)) return this
    this.removeGroup(groupId)
    return this
  }

  moveNodeToGroup(nodeId: CanvasGraphNodeId, groupId: CanvasGraphGroupId | null): this {
    this.assertOpen()
    if (!this.getNodeRecord(nodeId)) return this

    const currentGroupId = this.getGroupIdForNode(nodeId)
    if (currentGroupId === groupId) return this

    if (currentGroupId) {
      this.removeNodeFromGroup(nodeId, currentGroupId)
    }
    if (groupId) {
      this.addNodeToGroup(nodeId, groupId)
    }
    return this
  }

  addEdge(connection: CanvasConnection): this {
    this.assertOpen()
    const id = toCanvasGraphEdgeId(connection.id)
    if (this.getEdgeRecord(id)) {
      throw new Error(`CanvasGraphStore edge already exists: ${id}`)
    }
    this.assertConnectionEndpointsExist(connection)
    this.draft.edgeWrites.set(id, {
      id,
      connection: cloneJson(connection),
      selected: false,
    })
    this.ensureEdgeOrder().push(id)
    return this
  }

  removeEdge(edgeId: CanvasGraphEdgeId): this {
    this.assertOpen()
    if (!this.getEdgeRecord(edgeId)) return this
    this.draft.edgeWrites.set(edgeId, null)
    this.removeFromOrder(this.ensureEdgeOrder(), edgeId)
    return this
  }

  selectEdge(edgeId: CanvasGraphEdgeId, selected: boolean): this {
    this.assertOpen()
    const current = this.getEdgeRecord(edgeId)
    if (!current || current.selected === selected) return this
    this.draft.edgeWrites.set(edgeId, {
      ...current,
      selected,
    })
    return this
  }

  batchDelete(options: {
    nodeIds?: CanvasGraphNodeId[]
    edgeIds?: CanvasGraphEdgeId[]
    removeConnectedEdges?: boolean
  }): this {
    this.assertOpen()
    options.edgeIds?.forEach((edgeId) => {
      this.removeEdge(edgeId)
    })
    options.nodeIds?.forEach((nodeId) => {
      this.removeNode(nodeId, { removeConnectedEdges: options.removeConnectedEdges ?? true })
    })
    return this
  }

  commit(): CanvasGraphTransactionSummary {
    this.assertOpen()
    const changes = normalizeTransactionChanges(this.draft)
    const didChange =
      changes.hasDomainChanges ||
      changes.selectedNodeIds.size > 0 ||
      changes.selectedEdgeIds.size > 0
    if (didChange) this.store._commitTransaction(this.draft)
    this.status = 'committed'
    return createSummary(this.reason, this.options, this.store._readState(), changes)
  }

  cancel(): CanvasGraphTransactionSummary {
    this.assertOpen(false)
    this.status = 'cancelled'
    return createSummary(this.reason, this.options, this.draft.base, createChangeSet())
  }

  private assertOpen(requireCurrentRevision = true) {
    if (this.status !== 'open') {
      throw new Error(`CanvasGraphTransaction is already ${this.status}`)
    }
    if (requireCurrentRevision) this.store._assertTransactionCurrent(this.draft)
  }

  private assertConnectionEndpointsExist(connection: CanvasConnection) {
    const sourceId = toCanvasGraphNodeId(connection.fromNodeType, connection.fromNodeId)
    const targetId = toCanvasGraphNodeId(connection.toNodeType, connection.toNodeId)
    if (!this.getNodeRecord(sourceId) || !this.getNodeRecord(targetId)) {
      throw new Error(`CanvasGraphStore connection has missing endpoint: ${connection.id}`)
    }
  }

  private createNextGroupId(): CanvasGraphGroupId {
    let maxId = 0
    for (const groupId of this.readGroupOrder()) {
      const match = /^group-(\d+)$/.exec(groupId)
      if (!match) continue
      maxId = Math.max(maxId, Number(match[1]))
    }
    return toCanvasGraphGroupId(maxId + 1)
  }

  private normalizeNewGroupNodeIds(nodeIds: CanvasGraphNodeId[]): CanvasGraphNodeId[] {
    const normalizedNodeIds: CanvasGraphNodeId[] = []
    const seenNodeIds = new Set<CanvasGraphNodeId>()
    for (const nodeId of nodeIds) {
      if (seenNodeIds.has(nodeId)) continue
      if (!this.getNodeRecord(nodeId)) {
        throw new Error(`CanvasGraphStore group node does not exist: ${nodeId}`)
      }
      const existingGroupId = this.getGroupIdForNode(nodeId)
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

  private removeGroup(groupId: CanvasGraphGroupId) {
    this.draft.groupWrites.set(groupId, null)
    this.removeFromOrder(this.ensureGroupOrder(), groupId)
  }

  private addNodeToGroup(nodeId: CanvasGraphNodeId, groupId: CanvasGraphGroupId) {
    const group = this.getGroupRecord(groupId)
    if (!group) {
      throw new Error(`CanvasGraphStore group does not exist: ${groupId}`)
    }
    if (group.nodeIds.includes(nodeId)) return
    this.draft.groupWrites.set(groupId, {
      ...group,
      nodeIds: [...group.nodeIds, nodeId],
    })
  }

  private removeNodeFromGroup(nodeId: CanvasGraphNodeId, groupId: CanvasGraphGroupId) {
    const group = this.getGroupRecord(groupId)
    if (!group?.nodeIds.includes(nodeId)) return
    const nextNodeIds = group.nodeIds.filter((id) => id !== nodeId)
    this.draft.groupWrites.set(groupId, {
      ...group,
      nodeIds: nextNodeIds,
    })
  }

  private removeNodeFromGroups(nodeId: CanvasGraphNodeId) {
    const groupId = this.getGroupIdForNode(nodeId)
    if (groupId) this.removeNodeFromGroup(nodeId, groupId)
  }

  private getNodeRecord(nodeId: CanvasGraphNodeId): CanvasGraphNodeRecord | undefined {
    return this.readRecord(this.draft.nodeWrites, this.draft.base.nodesById, nodeId)
  }

  private getEdgeRecord(edgeId: CanvasGraphEdgeId): CanvasGraphEdgeRecord | undefined {
    return this.readRecord(this.draft.edgeWrites, this.draft.base.edgesById, edgeId)
  }

  private getGroupRecord(groupId: CanvasGraphGroupId): CanvasGraphGroupRecord | undefined {
    return this.readRecord(this.draft.groupWrites, this.draft.base.groupsById, groupId)
  }

  private readRecord<Id extends string, Record>(
    writes: TransactionRecordWrites<Id, Record>,
    baseRecords: Map<Id, Record>,
    id: Id,
  ): Record | undefined {
    if (writes.has(id)) return writes.get(id) ?? undefined
    return baseRecords.get(id)
  }

  private getConnectedEdgeIds(nodeId: CanvasGraphNodeId): Set<CanvasGraphEdgeId> {
    const edgeIds = new Set([
      ...(this.draft.base.indexes.outgoingEdgesByNodeId.get(nodeId) ?? []),
      ...(this.draft.base.indexes.incomingEdgesByNodeId.get(nodeId) ?? []),
    ])
    for (const [edgeId, nextRecord] of this.draft.edgeWrites) {
      const previousRecord = this.draft.base.edgesById.get(edgeId)
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

  private getGroupIdForNode(nodeId: CanvasGraphNodeId): CanvasGraphGroupId | undefined {
    for (const [groupId, group] of this.draft.groupWrites) {
      if (group?.nodeIds.includes(nodeId)) return groupId
    }
    const groupId = this.draft.base.indexes.groupIdByNodeId.get(nodeId)
    if (!groupId) return undefined
    const group = this.getGroupRecord(groupId)
    return group?.nodeIds.includes(nodeId) ? groupId : undefined
  }

  private readGroupOrder(): CanvasGraphGroupId[] {
    return this.draft.groupOrder ?? this.draft.base.groupOrder
  }

  private ensureNodeOrder(): CanvasGraphNodeId[] {
    return (this.draft.nodeOrder ??= [...this.draft.base.nodeOrder])
  }

  private ensureEdgeOrder(): CanvasGraphEdgeId[] {
    return (this.draft.edgeOrder ??= [...this.draft.base.edgeOrder])
  }

  private ensureGroupOrder(): CanvasGraphGroupId[] {
    return (this.draft.groupOrder ??= [...this.draft.base.groupOrder])
  }

  private removeFromOrder<Id>(order: Id[], id: Id) {
    const index = order.indexOf(id)
    if (index >= 0) order.splice(index, 1)
  }
}

export function createCanvasGraphStore(snapshot?: CanvasBoardSnapshot): CanvasGraphStore {
  return new CanvasGraphStore(snapshot)
}
