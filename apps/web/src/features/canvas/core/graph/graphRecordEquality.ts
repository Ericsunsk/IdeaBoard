import type { CanvasGraphGroupGeometry, CanvasGraphGroupRecord } from './graphStoreTypes'

/** Pure value comparisons shared by graph mutation and patch algebra. */
export function isSameGroupGeometry(
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

export function hasSameGroupMembership(
  left: CanvasGraphGroupRecord,
  right: CanvasGraphGroupRecord,
): boolean {
  return (
    left.nodeIds.length === right.nodeIds.length &&
    left.nodeIds.every((nodeId, index) => nodeId === right.nodeIds[index])
  )
}

/** Group records match when both membership order and geometry match. */
export function hasSameGroupRecord(
  left: CanvasGraphGroupRecord,
  right: CanvasGraphGroupRecord,
): boolean {
  return hasSameGroupMembership(left, right) && isSameGroupGeometry(left, right)
}
