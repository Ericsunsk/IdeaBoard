import {
  createEmptyCanvasBoardSnapshot,
  type CanvasBoardSnapshot,
  type CanvasConnection,
  type CanvasConnectionNodeType,
  type CanvasNodeItem,
  type FusionNodeItem,
  type ImageNodeItem,
  type PromptNodeItem,
} from '../../canvas-board/domain'

export type CanvasGraphNodeKind = CanvasConnectionNodeType
export type CanvasGraphNodeId = `${CanvasGraphNodeKind}-${number}`
export type CanvasGraphEdgeId = `connection-${number}`

export type CanvasGraphTransactionReason =
  | 'node-drag'
  | 'connect'
  | 'disconnect'
  | 'delete'
  | 'batch-delete'
  | 'node-update'
  | 'result-materialize'
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
}

export type CanvasGraphTransactionSummary = {
  id: string
  reason: CanvasGraphTransactionReason
  didChange: boolean
  hasDomainChanges: boolean
  changedNodeIds: CanvasGraphNodeId[]
  changedEdgeIds: CanvasGraphEdgeId[]
  removedNodeIds: CanvasGraphNodeId[]
  removedEdgeIds: CanvasGraphEdgeId[]
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
  | Partial<Omit<CanvasConnection, 'id'>>
  | ((connection: CanvasConnection) => CanvasConnection)

export type CanvasGraphRemoveNodeOptions = {
  removeConnectedEdges?: boolean
}

type CanvasGraphStoreState = {
  nodesById: Map<CanvasGraphNodeId, CanvasGraphNodeRecord>
  edgesById: Map<CanvasGraphEdgeId, CanvasGraphEdgeRecord>
  nodeOrder: CanvasGraphNodeId[]
  edgeOrder: CanvasGraphEdgeId[]
  indexes: CanvasGraphIndexes
}

type CanvasGraphIndexes = {
  outgoingEdgesByNodeId: Map<CanvasGraphNodeId, Set<CanvasGraphEdgeId>>
  incomingEdgesByNodeId: Map<CanvasGraphNodeId, Set<CanvasGraphEdgeId>>
  nodeIdsByKind: Map<CanvasGraphNodeKind, Set<CanvasGraphNodeId>>
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
  didChangeNodeOrder: boolean
  didChangeEdgeOrder: boolean
  hasDomainChanges: boolean
}

let transactionSequence = 0

function cloneJson<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

export function toCanvasGraphNodeId(
  kind: CanvasGraphNodeKind,
  id: number,
): CanvasGraphNodeId {
  return `${kind}-${id}` as CanvasGraphNodeId
}

export function toCanvasGraphEdgeId(id: number): CanvasGraphEdgeId {
  return `connection-${id}` as CanvasGraphEdgeId
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
    const sourceId = toCanvasGraphNodeId(
      edge.connection.fromNodeType,
      edge.connection.fromNodeId,
    )
    const targetId = toCanvasGraphNodeId(edge.connection.toNodeType, edge.connection.toNodeId)
    addToSetMap(indexes.outgoingEdgesByNodeId, sourceId, edgeId)
    addToSetMap(indexes.incomingEdgesByNodeId, targetId, edgeId)
  }
  return indexes
}

function createState(
  nodesById: Map<CanvasGraphNodeId, CanvasGraphNodeRecord>,
  edgesById: Map<CanvasGraphEdgeId, CanvasGraphEdgeRecord>,
  nodeOrder: CanvasGraphNodeId[],
  edgeOrder: CanvasGraphEdgeId[],
): CanvasGraphStoreState {
  return {
    nodesById,
    edgesById,
    nodeOrder,
    edgeOrder,
    indexes: rebuildIndexes({ nodesById, edgesById, nodeOrder, edgeOrder }),
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

function cloneState(state: CanvasGraphStoreState): CanvasGraphStoreState {
  return createState(
    new Map(
      Array.from(state.nodesById.entries(), ([id, record]) => [id, cloneNodeRecord(record)]),
    ),
    new Map(
      Array.from(state.edgesById.entries(), ([id, record]) => [id, cloneEdgeRecord(record)]),
    ),
    [...state.nodeOrder],
    [...state.edgeOrder],
  )
}

function createStateFromSnapshot(snapshot: CanvasBoardSnapshot): CanvasGraphStoreState {
  const nodesById = new Map<CanvasGraphNodeId, CanvasGraphNodeRecord>()
  const edgesById = new Map<CanvasGraphEdgeId, CanvasGraphEdgeRecord>()
  const nodeOrder: CanvasGraphNodeId[] = []
  const edgeOrder: CanvasGraphEdgeId[] = []

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

  return createState(nodesById, edgesById, nodeOrder, edgeOrder)
}

function exportSnapshotFromState(
  state: CanvasGraphStoreState,
  base?: CanvasBoardSnapshot,
): CanvasBoardSnapshot {
  const snapshot = createEmptyCanvasBoardSnapshot()
  if (base) snapshot.viewport = { ...base.viewport }

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
    const targetId = toCanvasGraphNodeId(
      record.connection.toNodeType,
      record.connection.toNodeId,
    )
    if (!state.nodesById.has(sourceId) || !state.nodesById.has(targetId)) continue
    snapshot.connections.push(cloneJson(record.connection))
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
    didChangeNodeOrder: false,
    didChangeEdgeOrder: false,
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

function applyNodeUpdate(
  current: CanvasNodeItem,
  update: CanvasGraphNodeUpdate,
): CanvasNodeItem {
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
  const selectedNodeIds = Array.from(changes.selectedNodeIds)
  const selectedEdgeIds = Array.from(changes.selectedEdgeIds)
  const removedNodeIds = Array.from(changes.removedNodeIds)
  const removedEdgeIds = Array.from(changes.removedEdgeIds)
  const changedNodeIds = [...addedNodeIds, ...updatedNodeIds]
  const changedEdgeIds = [...addedEdgeIds, ...updatedEdgeIds]
  const didChange =
    changes.hasDomainChanges || selectedNodeIds.length > 0 || selectedEdgeIds.length > 0

  return {
    id: `canvas-graph-tx-${++transactionSequence}`,
    reason,
    didChange,
    hasDomainChanges: changes.hasDomainChanges,
    changedNodeIds,
    changedEdgeIds,
    removedNodeIds,
    removedEdgeIds,
    selectedNodeIds,
    selectedEdgeIds,
    didChangeOrder: changes.didChangeNodeOrder || changes.didChangeEdgeOrder,
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

  getNode(id: CanvasGraphNodeId): CanvasGraphNodeRecord | undefined {
    const record = this.state.nodesById.get(id)
    return record ? cloneNodeRecord(record) : undefined
  }

  getEdge(id: CanvasGraphEdgeId): CanvasGraphEdgeRecord | undefined {
    const record = this.state.edgesById.get(id)
    return record ? cloneEdgeRecord(record) : undefined
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
    this.state = createState(state.nodesById, state.edgesById, state.nodeOrder, state.edgeOrder)
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
    this.status = 'committed'
    this.store._commitDraftState(this.draft)
    return createSummary(this.reason, this.options, this.draft, this.changes)
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

  private rebuildDraftIndexes() {
    this.draft.indexes = rebuildIndexes(this.draft)
  }
}

export function createCanvasGraphStore(snapshot?: CanvasBoardSnapshot): CanvasGraphStore {
  return new CanvasGraphStore(snapshot)
}

export type CanvasGraphSnapshotNode = ImageNodeItem | PromptNodeItem | FusionNodeItem
