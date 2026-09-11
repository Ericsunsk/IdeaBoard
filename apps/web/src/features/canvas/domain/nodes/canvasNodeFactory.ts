import {
  createTypedCanvasNodeSnapshot,
  type CanvasCreatableNodeKind,
  type CanvasNodeItemByKind,
  type CanvasPoint,
} from '../../../board'

let lastCanvasEntityId = 0

function createCanvasEntityId(): number {
  const timestampId = Date.now() * 1000
  const nextId = Math.max(timestampId, lastCanvasEntityId + 1)
  lastCanvasEntityId = nextId
  return nextId
}

export function createCanvasNodeAt<T extends CanvasCreatableNodeKind>(
  kind: T,
  point: CanvasPoint,
): CanvasNodeItemByKind[T] {
  return createTypedCanvasNodeSnapshot(kind, point, createCanvasEntityId())
}
