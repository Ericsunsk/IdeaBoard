import {
  type CanvasConnection,
  type CanvasConnectionNodeKind,
  type CanvasGroup,
  type CanvasNodeItem,
  type FusionNodeItem,
  type ImageNodeItem,
  type PromptNodeItem,
} from '../../../board/ports/canvas'

export type CanvasGraphNodeKind = CanvasConnectionNodeKind
export type CanvasGraphNodeId = `${CanvasGraphNodeKind}-${number}`
export type CanvasGraphNodeIdForKind<T extends CanvasGraphNodeKind> = `${T}-${number}`
export type CanvasGraphEdgeId = `connection-${number}`
export type CanvasGraphGroupId = CanvasGroup['id']

export class CanvasGraphStaleTransactionError extends Error {
  constructor() {
    super('CanvasGraphStore transaction is stale')
    this.name = 'CanvasGraphStaleTransactionError'
  }
}

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
  | 'history-undo'
  | 'history-redo'
  | 'hydrate'

export type CanvasGraphNodeRecord = {
  id: CanvasGraphNodeId
  kind: CanvasGraphNodeKind
  snapshot: CanvasNodeItem
  isSelected: boolean
}

export type CanvasGraphEdgeRecord = {
  id: CanvasGraphEdgeId
  connection: CanvasConnection
  isSelected: boolean
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

/**
 * Read-only canonical graph boundary for business modules.
 *
 * Consumers that only inspect topology or domain snapshots must depend on
 * this port instead of the mutable store implementation or Vue Flow arrays.
 */
export type CanvasGraphReadPort = {
  getNodeIds: () => CanvasGraphNodeId[]
  getEdgeIds: () => CanvasGraphEdgeId[]
  getGroupIds: () => CanvasGraphGroupId[]
  getNode: (id: CanvasGraphNodeId) => CanvasGraphNodeRecord | undefined
  getEdge: (id: CanvasGraphEdgeId) => CanvasGraphEdgeRecord | undefined
  getGroup: (id: CanvasGraphGroupId) => CanvasGraphGroupRecord | undefined
  getGroupIdForNode: (nodeId: CanvasGraphNodeId) => CanvasGraphGroupId | undefined
  getNodeIdsByKind: (kind: CanvasGraphNodeKind) => CanvasGraphNodeId[]
  getOutgoingEdgeIds: (nodeId: CanvasGraphNodeId) => CanvasGraphEdgeId[]
  getIncomingEdgeIds: (nodeId: CanvasGraphNodeId) => CanvasGraphEdgeId[]
}

export type CanvasGraphProjectionPatch = {
  nodes: {
    added: CanvasGraphNodeRecord[]
    updated: CanvasGraphNodeRecord[]
    removedIds: CanvasGraphNodeId[]
    selectionChanged: Array<{ id: CanvasGraphNodeId; isSelected: boolean }>
  }
  edges: {
    added: CanvasGraphEdgeRecord[]
    removedIds: CanvasGraphEdgeId[]
    selectionChanged: Array<{ id: CanvasGraphEdgeId; isSelected: boolean }>
  }
  groups: {
    added: CanvasGraphGroupRecord[]
    updated: CanvasGraphGroupRecord[]
    removedIds: CanvasGraphGroupId[]
  }
}

/**
 * The reversible domain delta produced by one committed graph transaction.
 *
 * This is deliberately expressed in graph records rather than Flow values: it
 * is the shared fact consumed by History, persistence and Flow projection.
 * Selection is part of a record delta so undoing a semantic edit restores the
 * selection that accompanied it, but selection-only commits never request
 * History or persistence side effects.
 */
export type CanvasGraphRecordPatch<Record, Id extends string> = Readonly<{
  id: Id
  before?: Record
  after?: Record
}>

export type CanvasGraphOrderPlacement<Id extends string> = Readonly<{
  previousId?: Id | undefined
  nextId?: Id | undefined
}>

export type CanvasGraphOrderPatch<Id extends string> = Readonly<{
  id: Id
  before?: CanvasGraphOrderPlacement<Id> | undefined
  after?: CanvasGraphOrderPlacement<Id> | undefined
}>

export type CanvasGraphCollectionPatch<Record, Id extends string> = Readonly<{
  records: readonly CanvasGraphRecordPatch<Record, Id>[]
  /** Only changed IDs are retained; never a full Board-sized order snapshot. */
  order: readonly CanvasGraphOrderPatch<Id>[]
}>

export type CanvasGraphCommitPatch = Readonly<{
  nodes: CanvasGraphCollectionPatch<CanvasGraphNodeRecord, CanvasGraphNodeId>
  edges: CanvasGraphCollectionPatch<CanvasGraphEdgeRecord, CanvasGraphEdgeId>
  groups: CanvasGraphCollectionPatch<CanvasGraphGroupRecord, CanvasGraphGroupId>
}>

export type CanvasGraphCommitEvent = Readonly<{
  summary: CanvasGraphTransactionSummary
  patch: CanvasGraphCommitPatch
  sideEffects: Readonly<{
    shouldRecordHistory: boolean
    shouldMarkDirty: boolean
  }>
}>

export type CanvasGraphPatchDirection = 'forward' | 'backward'

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
  commitPatch: CanvasGraphCommitPatch
  projectionPatch: CanvasGraphProjectionPatch
}

export type CanvasGraphTransactionOptions = {
  shouldRecordHistory?: boolean
  shouldMarkDirty?: boolean
}

export type CanvasGraphNodeUpdate =
  | Partial<Omit<ImageNodeItem, 'id' | 'type'>>
  | Partial<Omit<PromptNodeItem, 'id' | 'type'>>
  | Partial<Omit<FusionNodeItem, 'id' | 'type'>>
  | ((node: CanvasNodeItem) => CanvasNodeItem)

export type CanvasGraphRemoveNodeOptions = {
  shouldRemoveConnectedEdges?: boolean
}

export type CanvasGraphStoreState = {
  nodesById: Map<CanvasGraphNodeId, CanvasGraphNodeRecord>
  edgesById: Map<CanvasGraphEdgeId, CanvasGraphEdgeRecord>
  groupsById: Map<CanvasGraphGroupId, CanvasGraphGroupRecord>
  nodeOrder: CanvasGraphNodeId[]
  edgeOrder: CanvasGraphEdgeId[]
  groupOrder: CanvasGraphGroupId[]
  indexes: CanvasGraphIndexes
}

export type CanvasGraphIndexes = {
  outgoingEdgesByNodeId: Map<CanvasGraphNodeId, Set<CanvasGraphEdgeId>>
  incomingEdgesByNodeId: Map<CanvasGraphNodeId, Set<CanvasGraphEdgeId>>
  nodeIdsByKind: Map<CanvasGraphNodeKind, Set<CanvasGraphNodeId>>
  groupIdByNodeId: Map<CanvasGraphNodeId, CanvasGraphGroupId>
}

export type TransactionRecordWrites<Id extends string, Record> = Map<Id, Record | null>

export type CanvasGraphTransactionDraft = {
  base: CanvasGraphStoreState
  baseRevision: number
  nodeWrites: TransactionRecordWrites<CanvasGraphNodeId, CanvasGraphNodeRecord>
  edgeWrites: TransactionRecordWrites<CanvasGraphEdgeId, CanvasGraphEdgeRecord>
  groupWrites: TransactionRecordWrites<CanvasGraphGroupId, CanvasGraphGroupRecord>
  nodeOrder?: CanvasGraphNodeId[] | undefined
  edgeOrder?: CanvasGraphEdgeId[] | undefined
  groupOrder?: CanvasGraphGroupId[] | undefined
  nodeOrderPatches: Map<CanvasGraphNodeId, CanvasGraphOrderPatch<CanvasGraphNodeId>>
  edgeOrderPatches: Map<CanvasGraphEdgeId, CanvasGraphOrderPatch<CanvasGraphEdgeId>>
  groupOrderPatches: Map<CanvasGraphGroupId, CanvasGraphOrderPatch<CanvasGraphGroupId>>
}

export type TransactionChangeSet = {
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

/**
 * Narrow view of the store that {@link import('./canvasGraphTransactionPort').CanvasGraphTransaction}
 * needs. Declared here (rather than importing `CanvasGraphStore`) so the store
 * and its transaction subsystem do not form an import cycle.
 */
export type CanvasGraphTransactionHost = {
  _readState(): CanvasGraphStoreState
  _publishCommit(summary: CanvasGraphTransactionSummary): void
  _createTransactionDraft(): CanvasGraphTransactionDraft
  _commitTransaction(draft: CanvasGraphTransactionDraft): void
  _assertTransactionCurrent(draft: CanvasGraphTransactionDraft): void
}
