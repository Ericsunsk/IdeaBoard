import {
  createEmptyBoardSnapshot,
  type BoardSnapshot,
  type CanvasGroup,
  type CanvasNodeItem,
} from '../../../board'
import { cloneJsonValue } from './canvasGraphClone'
import { toCanvasGraphEdgeId, toCanvasGraphNodeId } from './canvasGraphStoreIds'
import { rebuildIndexes } from './canvasGraphStoreIndexes'
import type {
  CanvasGraphEdgeId,
  CanvasGraphEdgeRecord,
  CanvasGraphGroupId,
  CanvasGraphGroupRecord,
  CanvasGraphNodeId,
  CanvasGraphNodeKind,
  CanvasGraphNodeRecord,
  CanvasGraphStoreState,
} from './canvasGraphStoreTypes'

export function getNodeKind(node: CanvasNodeItem): CanvasGraphNodeKind {
  return node.type === 'image-node' ? 'image' : node.type
}

function getNodeCollection(snapshot: BoardSnapshot, kind: CanvasGraphNodeKind): CanvasNodeItem[] {
  if (kind === 'image') return snapshot.imageNodes
  if (kind === 'prompt') return snapshot.promptNodes
  return snapshot.fusionNodes
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

export function cloneNodeRecord(record: CanvasGraphNodeRecord): CanvasGraphNodeRecord {
  return {
    id: record.id,
    kind: record.kind,
    snapshot: cloneJsonValue(record.snapshot),
    selected: record.selected,
  }
}

export function cloneEdgeRecord(record: CanvasGraphEdgeRecord): CanvasGraphEdgeRecord {
  return {
    id: record.id,
    connection: cloneJsonValue(record.connection),
    selected: record.selected,
  }
}

export function cloneGroupRecord(record: CanvasGraphGroupRecord): CanvasGraphGroupRecord {
  return {
    id: record.id,
    nodeIds: [...record.nodeIds],
    x: record.x,
    y: record.y,
    width: record.width,
    height: record.height,
  }
}

export function createStateFromSnapshot(snapshot: BoardSnapshot): CanvasGraphStoreState {
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
      snapshot: cloneJsonValue(node),
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
      connection: cloneJsonValue(connection),
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

export function exportSnapshotFromState(
  state: CanvasGraphStoreState,
  base?: BoardSnapshot,
): BoardSnapshot {
  const snapshot = createEmptyBoardSnapshot()
  if (base?.mediaRefs) {
    snapshot.mediaRefs = base.mediaRefs.map((ref) => ({ ...ref }))
  }

  for (const nodeId of state.nodeOrder) {
    const record = state.nodesById.get(nodeId)
    if (!record) continue
    getNodeCollection(snapshot, record.kind).push(cloneJsonValue(record.snapshot))
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
    snapshot.connections.push(cloneJsonValue(record.connection))
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
