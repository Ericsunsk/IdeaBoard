import { toCanvasGraphNodeId } from './graphStoreIds'
import type {
  CanvasGraphEdgeRecord,
  CanvasGraphGroupRecord,
  CanvasGraphIndexes,
  CanvasGraphNodeId,
  CanvasGraphNodeRecord,
  CanvasGraphStoreState,
} from './graphStoreTypes'

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

export function getEdgeEndpointIds(record: CanvasGraphEdgeRecord): {
  sourceId: CanvasGraphNodeId
  targetId: CanvasGraphNodeId
} {
  return {
    sourceId: toCanvasGraphNodeId(record.connection.fromNodeType, record.connection.fromNodeId),
    targetId: toCanvasGraphNodeId(record.connection.toNodeType, record.connection.toNodeId),
  }
}

export function addEdgeToIndexes(indexes: CanvasGraphIndexes, record: CanvasGraphEdgeRecord) {
  const { sourceId, targetId } = getEdgeEndpointIds(record)
  addToSetMap(indexes.outgoingEdgesByNodeId, sourceId, record.id)
  addToSetMap(indexes.incomingEdgesByNodeId, targetId, record.id)
}

export function removeEdgeFromIndexes(indexes: CanvasGraphIndexes, record: CanvasGraphEdgeRecord) {
  const { sourceId, targetId } = getEdgeEndpointIds(record)
  removeFromSetMap(indexes.outgoingEdgesByNodeId, sourceId, record.id)
  removeFromSetMap(indexes.incomingEdgesByNodeId, targetId, record.id)
}

export function hasSameEdgeTopology(
  left: CanvasGraphEdgeRecord,
  right: CanvasGraphEdgeRecord,
): boolean {
  const leftEndpoints = getEdgeEndpointIds(left)
  const rightEndpoints = getEdgeEndpointIds(right)
  return (
    leftEndpoints.sourceId === rightEndpoints.sourceId &&
    leftEndpoints.targetId === rightEndpoints.targetId
  )
}

export function addGroupToIndexes(
  indexes: CanvasGraphIndexes,
  record: CanvasGraphGroupRecord,
  nodesById: Map<CanvasGraphNodeId, CanvasGraphNodeRecord>,
) {
  for (const nodeId of record.nodeIds) {
    if (nodesById.has(nodeId)) indexes.groupIdByNodeId.set(nodeId, record.id)
  }
}

export function removeGroupFromIndexes(
  indexes: CanvasGraphIndexes,
  record: CanvasGraphGroupRecord,
) {
  for (const nodeId of record.nodeIds) {
    if (indexes.groupIdByNodeId.get(nodeId) === record.id) {
      indexes.groupIdByNodeId.delete(nodeId)
    }
  }
}

export { hasSameGroupMembership } from './graphRecordEquality'

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

export function rebuildIndexes(state: Omit<CanvasGraphStoreState, 'indexes'>): CanvasGraphIndexes {
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
