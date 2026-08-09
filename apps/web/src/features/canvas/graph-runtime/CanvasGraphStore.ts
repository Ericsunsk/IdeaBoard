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
    updated: CanvasGraphEdgeRecord[]
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

export type CanvasGraphEdgeUpdate =
  Partial<Omit<CanvasConnection, 'id'>> | ((connection: CanvasConnection) => CanvasConnection)

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

type MutableTransactionState = CanvasGraphStoreState

type TransactionChangeSet = {
  addedNodeIds: Set<CanvasGraphNodeId>
  updatedNodeIds: Set<CanvasGraphNodeId>
  removedNodeIds: Set<CanvasGraphNodeId>
  selectedNodeIds: Set<CanvasGraphNodeId>
  addedEdgeIds: Set<CanvasGraphEdgeId>
  updatedEdgeIds: Set<CanvasGraphEdgeId>
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

function cloneState(state: CanvasGraphStoreState): CanvasGraphStoreState {
  return createState(
    new Map(Array.from(state.nodesById.entries(), ([id, record]) => [id, cloneNodeRecord(record)])),
    new Map(Array.from(state.edgesById.entries(), ([id, record]) => [id, cloneEdgeRecord(record)])),
    new Map(
      Array.from(state.groupsById.entries(), ([id, record]) => [id, cloneGroupRecord(record)]),
    ),
    [...state.nodeOrder],
    [...state.edgeOrder],
    [...state.groupOrder],
  )
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
    updatedEdgeIds: new Set(),
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

function setDomainChanged(changes: TransactionChangeSet) {
  changes.hasDomainChanges = true
}

function markNodeChanged(
  changes: TransactionChangeSet,
  nodeId: CanvasGraphNodeId,
  type: 'added' | 'updated' | 'removed' | 'selected',
) {
  if (type === 'selected') {
    changes.selectedNodeIds.add(nodeId)
    return
  }
  setDomainChanged(changes)
  if (type === 'removed') {
    changes.addedNodeIds.delete(nodeId)
    changes.updatedNodeIds.delete(nodeId)
    changes.removedNodeIds.add(nodeId)
    changes.didChangeNodeOrder = true
    return
  }
  if (type === 'added') {
    changes.removedNodeIds.delete(nodeId)
    changes.addedNodeIds.add(nodeId)
    changes.didChangeNodeOrder = true
    return
  }
  if (!changes.addedNodeIds.has(nodeId)) changes.updatedNodeIds.add(nodeId)
}

function markEdgeChanged(
  changes: TransactionChangeSet,
  edgeId: CanvasGraphEdgeId,
  type: 'added' | 'updated' | 'removed' | 'selected',
) {
  if (type === 'selected') {
    changes.selectedEdgeIds.add(edgeId)
    return
  }
  setDomainChanged(changes)
  if (type === 'removed') {
    changes.addedEdgeIds.delete(edgeId)
    changes.updatedEdgeIds.delete(edgeId)
    changes.removedEdgeIds.add(edgeId)
    changes.didChangeEdgeOrder = true
    return
  }
  if (type === 'added') {
    changes.removedEdgeIds.delete(edgeId)
    changes.addedEdgeIds.add(edgeId)
    changes.didChangeEdgeOrder = true
    return
  }
  if (!changes.addedEdgeIds.has(edgeId)) changes.updatedEdgeIds.add(edgeId)
}

function markGroupChanged(
  changes: TransactionChangeSet,
  groupId: CanvasGraphGroupId,
  type: 'added' | 'updated' | 'removed',
) {
  setDomainChanged(changes)
  if (type === 'removed') {
    changes.addedGroupIds.delete(groupId)
    changes.updatedGroupIds.delete(groupId)
    changes.removedGroupIds.add(groupId)
    changes.didChangeGroupOrder = true
    return
  }
  if (type === 'added') {
    changes.removedGroupIds.delete(groupId)
    changes.addedGroupIds.add(groupId)
    changes.didChangeGroupOrder = true
    return
  }
  if (!changes.addedGroupIds.has(groupId)) changes.updatedGroupIds.add(groupId)
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

function applyEdgeUpdate(
  current: CanvasConnection,
  update: CanvasGraphEdgeUpdate,
): CanvasConnection {
  if (typeof update === 'function') return cloneJson(update(cloneJson(current)))
  return {
    ...cloneJson(current),
    ...update,
    id: current.id,
  }
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
  const updatedEdgeIds = Array.from(changes.updatedEdgeIds)
  const addedGroupIds = Array.from(changes.addedGroupIds)
  const updatedGroupIds = Array.from(changes.updatedGroupIds)
  const selectedNodeIds = Array.from(changes.selectedNodeIds)
  const selectedEdgeIds = Array.from(changes.selectedEdgeIds)
  const removedNodeIds = Array.from(changes.removedNodeIds)
  const removedEdgeIds = Array.from(changes.removedEdgeIds)
  const removedGroupIds = Array.from(changes.removedGroupIds)
  const changedNodeIds = [...addedNodeIds, ...updatedNodeIds]
  const changedEdgeIds = [...addedEdgeIds, ...updatedEdgeIds]
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
        updated: updatedEdgeIds
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

  constructor(snapshot: CanvasBoardSnapshot = createEmptyCanvasBoardSnapshot()) {
    this.state = createStateFromSnapshot(snapshot)
  }

  static fromSnapshot(snapshot: CanvasBoardSnapshot): CanvasGraphStore {
    return new CanvasGraphStore(snapshot)
  }

  hydrate(snapshot: CanvasBoardSnapshot) {
    this.state = createStateFromSnapshot(snapshot)
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

  _createDraftState(): MutableTransactionState {
    return cloneState(this.state)
  }

  _commitDraftState(state: MutableTransactionState) {
    this.state = createState(
      state.nodesById,
      state.edgesById,
      state.groupsById,
      state.nodeOrder,
      state.edgeOrder,
      state.groupOrder,
    )
  }
}

export class CanvasGraphTransaction {
  private readonly store: CanvasGraphStore
  private readonly reason: CanvasGraphTransactionReason
  private readonly options: Required<CanvasGraphTransactionOptions>
  private readonly draft: MutableTransactionState
  private readonly changes = createChangeSet()
  private status: 'open' | 'committed' | 'cancelled' = 'open'

  constructor(
    store: CanvasGraphStore,
    reason: CanvasGraphTransactionReason,
    options: Required<CanvasGraphTransactionOptions>,
  ) {
    this.store = store
    this.reason = reason
    this.options = options
    this.draft = store._createDraftState()
  }

  addNode(node: CanvasNodeItem): this {
    this.assertOpen()
    const kind = getNodeKind(node)
    const id = toCanvasGraphNodeId(kind, node.id)
    if (this.draft.nodesById.has(id)) {
      throw new Error(`CanvasGraphStore node already exists: ${id}`)
    }
    this.draft.nodesById.set(id, {
      id,
      kind,
      snapshot: cloneJson(node),
      selected: false,
    })
    this.draft.nodeOrder.push(id)
    markNodeChanged(this.changes, id, 'added')
    return this
  }

  updateNode(nodeId: CanvasGraphNodeId, update: CanvasGraphNodeUpdate): this {
    this.assertOpen()
    const current = this.draft.nodesById.get(nodeId)
    if (!current) return this
    const nextSnapshot = applyNodeUpdate(current.snapshot, update)
    const nextKind = getNodeKind(nextSnapshot)
    if (nextKind !== current.kind || nextSnapshot.id !== current.snapshot.id) {
      throw new Error(`CanvasGraphStore cannot change node identity: ${nodeId}`)
    }
    this.draft.nodesById.set(nodeId, {
      ...current,
      snapshot: nextSnapshot,
    })
    markNodeChanged(this.changes, nodeId, 'updated')
    return this
  }

  removeNode(nodeId: CanvasGraphNodeId, options: CanvasGraphRemoveNodeOptions = {}): this {
    this.assertOpen()
    if (!this.draft.nodesById.has(nodeId)) return this
    if (options.removeConnectedEdges) {
      const edgeIds = new Set([
        ...Array.from(this.draft.indexes.outgoingEdgesByNodeId.get(nodeId) ?? []),
        ...Array.from(this.draft.indexes.incomingEdgesByNodeId.get(nodeId) ?? []),
      ])
      edgeIds.forEach((edgeId) => {
        this.removeEdge(edgeId)
      })
    }
    this.removeNodeFromGroups(nodeId)
    this.draft.nodesById.delete(nodeId)
    this.draft.nodeOrder = this.draft.nodeOrder.filter((id) => id !== nodeId)
    markNodeChanged(this.changes, nodeId, 'removed')
    this.rebuildDraftIndexes()
    return this
  }

  selectNode(nodeId: CanvasGraphNodeId, selected: boolean): this {
    this.assertOpen()
    const current = this.draft.nodesById.get(nodeId)
    if (!current || current.selected === selected) return this
    this.draft.nodesById.set(nodeId, {
      ...current,
      selected,
    })
    markNodeChanged(this.changes, nodeId, 'selected')
    return this
  }

  createGroup(
    nodeIds: CanvasGraphNodeId[],
    geometry: CanvasGraphGroupGeometry,
    groupId?: CanvasGraphGroupId,
  ): this {
    this.assertOpen()
    const resolvedGroupId = groupId ?? this.createNextGroupId()
    if (this.draft.groupsById.has(resolvedGroupId)) {
      throw new Error(`CanvasGraphStore group already exists: ${resolvedGroupId}`)
    }
    const normalizedNodeIds = this.normalizeNewGroupNodeIds(nodeIds)
    const normalizedGeometry = normalizeGroupGeometry(geometry)
    this.draft.groupsById.set(resolvedGroupId, {
      id: resolvedGroupId,
      nodeIds: normalizedNodeIds,
      ...normalizedGeometry,
    })
    this.draft.groupOrder.push(resolvedGroupId)
    markGroupChanged(this.changes, resolvedGroupId, 'added')
    this.rebuildDraftIndexes()
    return this
  }

  updateGroupGeometry(groupId: CanvasGraphGroupId, geometry: CanvasGraphGroupGeometry): this {
    this.assertOpen()
    const current = this.draft.groupsById.get(groupId)
    if (!current) return this
    const normalizedGeometry = normalizeGroupGeometry(geometry)
    if (isSameGroupGeometry(current, normalizedGeometry)) return this
    this.draft.groupsById.set(groupId, {
      ...current,
      ...normalizedGeometry,
    })
    markGroupChanged(this.changes, groupId, 'updated')
    return this
  }

  ungroup(groupId: CanvasGraphGroupId): this {
    this.assertOpen()
    if (!this.draft.groupsById.has(groupId)) return this
    this.removeGroup(groupId)
    this.rebuildDraftIndexes()
    return this
  }

  moveNodeToGroup(nodeId: CanvasGraphNodeId, groupId: CanvasGraphGroupId | null): this {
    this.assertOpen()
    if (!this.draft.nodesById.has(nodeId)) return this

    const currentGroupId = this.draft.indexes.groupIdByNodeId.get(nodeId)
    if (currentGroupId === groupId) return this

    if (currentGroupId) {
      this.removeNodeFromGroup(nodeId, currentGroupId)
    }
    if (groupId) {
      this.addNodeToGroup(nodeId, groupId)
    }
    this.rebuildDraftIndexes()
    return this
  }

  addEdge(connection: CanvasConnection): this {
    this.assertOpen()
    const id = toCanvasGraphEdgeId(connection.id)
    if (this.draft.edgesById.has(id)) {
      throw new Error(`CanvasGraphStore edge already exists: ${id}`)
    }
    this.assertConnectionEndpointsExist(connection)
    this.draft.edgesById.set(id, {
      id,
      connection: cloneJson(connection),
      selected: false,
    })
    this.draft.edgeOrder.push(id)
    markEdgeChanged(this.changes, id, 'added')
    this.rebuildDraftIndexes()
    return this
  }

  updateEdge(edgeId: CanvasGraphEdgeId, update: CanvasGraphEdgeUpdate): this {
    this.assertOpen()
    const current = this.draft.edgesById.get(edgeId)
    if (!current) return this
    const nextConnection = applyEdgeUpdate(current.connection, update)
    if (nextConnection.id !== current.connection.id) {
      throw new Error(`CanvasGraphStore cannot change edge identity: ${edgeId}`)
    }
    this.assertConnectionEndpointsExist(nextConnection)
    this.draft.edgesById.set(edgeId, {
      ...current,
      connection: nextConnection,
    })
    markEdgeChanged(this.changes, edgeId, 'updated')
    this.rebuildDraftIndexes()
    return this
  }

  removeEdge(edgeId: CanvasGraphEdgeId): this {
    this.assertOpen()
    if (!this.draft.edgesById.has(edgeId)) return this
    this.draft.edgesById.delete(edgeId)
    this.draft.edgeOrder = this.draft.edgeOrder.filter((id) => id !== edgeId)
    markEdgeChanged(this.changes, edgeId, 'removed')
    this.rebuildDraftIndexes()
    return this
  }

  selectEdge(edgeId: CanvasGraphEdgeId, selected: boolean): this {
    this.assertOpen()
    const current = this.draft.edgesById.get(edgeId)
    if (!current || current.selected === selected) return this
    this.draft.edgesById.set(edgeId, {
      ...current,
      selected,
    })
    markEdgeChanged(this.changes, edgeId, 'selected')
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
    const summary = createSummary(this.reason, this.options, this.draft, this.changes)
    this.store._commitDraftState(this.draft)
    this.status = 'committed'
    return summary
  }

  cancel(): CanvasGraphTransactionSummary {
    this.assertOpen()
    this.status = 'cancelled'
    return createSummary(this.reason, this.options, this.draft, createChangeSet())
  }

  private assertOpen() {
    if (this.status !== 'open') {
      throw new Error(`CanvasGraphTransaction is already ${this.status}`)
    }
  }

  private assertConnectionEndpointsExist(connection: CanvasConnection) {
    const sourceId = toCanvasGraphNodeId(connection.fromNodeType, connection.fromNodeId)
    const targetId = toCanvasGraphNodeId(connection.toNodeType, connection.toNodeId)
    if (!this.draft.nodesById.has(sourceId) || !this.draft.nodesById.has(targetId)) {
      throw new Error(`CanvasGraphStore connection has missing endpoint: ${connection.id}`)
    }
  }

  private createNextGroupId(): CanvasGraphGroupId {
    let maxId = 0
    for (const groupId of this.draft.groupOrder) {
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
      if (!this.draft.nodesById.has(nodeId)) {
        throw new Error(`CanvasGraphStore group node does not exist: ${nodeId}`)
      }
      const existingGroupId = this.draft.indexes.groupIdByNodeId.get(nodeId)
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
    this.draft.groupsById.delete(groupId)
    this.draft.groupOrder = this.draft.groupOrder.filter((id) => id !== groupId)
    markGroupChanged(this.changes, groupId, 'removed')
  }

  private addNodeToGroup(nodeId: CanvasGraphNodeId, groupId: CanvasGraphGroupId) {
    const group = this.draft.groupsById.get(groupId)
    if (!group) {
      throw new Error(`CanvasGraphStore group does not exist: ${groupId}`)
    }
    if (group.nodeIds.includes(nodeId)) return
    this.draft.groupsById.set(groupId, {
      ...group,
      nodeIds: [...group.nodeIds, nodeId],
    })
    markGroupChanged(this.changes, groupId, 'updated')
  }

  private removeNodeFromGroup(nodeId: CanvasGraphNodeId, groupId: CanvasGraphGroupId) {
    const group = this.draft.groupsById.get(groupId)
    if (!group?.nodeIds.includes(nodeId)) return
    const nextNodeIds = group.nodeIds.filter((id) => id !== nodeId)
    this.draft.groupsById.set(groupId, {
      ...group,
      nodeIds: nextNodeIds,
    })
    markGroupChanged(this.changes, groupId, 'updated')
  }

  private removeNodeFromGroups(nodeId: CanvasGraphNodeId) {
    for (const groupId of [...this.draft.groupOrder]) {
      this.removeNodeFromGroup(nodeId, groupId)
    }
  }

  private rebuildDraftIndexes() {
    this.draft.indexes = rebuildIndexes(this.draft)
  }
}

export function createCanvasGraphStore(snapshot?: CanvasBoardSnapshot): CanvasGraphStore {
  return new CanvasGraphStore(snapshot)
}
