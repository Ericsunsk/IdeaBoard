import type {
  CanvasGraphEdgeId,
  CanvasGraphGroupId,
  CanvasGraphNodeId,
  CanvasGraphNodeKind,
} from './graphStoreTypes'

export function toCanvasGraphNodeId(kind: CanvasGraphNodeKind, id: number): CanvasGraphNodeId {
  return `${kind}-${id}` as CanvasGraphNodeId
}

export function toCanvasGraphEdgeId(id: number): CanvasGraphEdgeId {
  return `connection-${id}` as CanvasGraphEdgeId
}

export function toCanvasGraphGroupId(id: number): CanvasGraphGroupId {
  return `group-${id}` as CanvasGraphGroupId
}
