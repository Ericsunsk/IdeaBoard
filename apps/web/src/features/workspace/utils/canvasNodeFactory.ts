import {
  createTypedCanvasNodeSnapshot,
  type CanvasCreatableNodeKind,
} from '../config/canvasNodeRegistry'
import type { CanvasNodeItemByKind, CanvasPoint } from '../types/canvasNodes'

function createCanvasEntityId(): number {
  return Date.now() + Math.random()
}

export function createCanvasNodeAt<T extends CanvasCreatableNodeKind>(
  kind: T,
  point: CanvasPoint,
): CanvasNodeItemByKind[T] {
  return createTypedCanvasNodeSnapshot(kind, point, createCanvasEntityId())
}
