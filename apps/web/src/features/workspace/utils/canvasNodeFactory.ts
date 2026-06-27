import {
  createTypedCanvasNodeSnapshot,
  type CanvasCreatableNodeKind,
  type CanvasNodeItemByKind,
  type CanvasPoint,
} from '../../canvas-board/domain'

function createCanvasEntityId(): number {
  return Date.now() + Math.random()
}

export function createCanvasNodeAt<T extends CanvasCreatableNodeKind>(
  kind: T,
  point: CanvasPoint,
): CanvasNodeItemByKind[T] {
  return createTypedCanvasNodeSnapshot(kind, point, createCanvasEntityId())
}
