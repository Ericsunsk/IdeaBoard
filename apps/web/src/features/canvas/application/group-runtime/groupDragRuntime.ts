import type { ComputedRef } from 'vue'
import type { NodeChange } from '@vue-flow/core'
import {
  type CanvasGraphGroupId,
  type CanvasGraphNodeId,
  type CanvasGraphStore,
  type CanvasGraphTransaction,
} from '../../domain/graph'
import {
  canvasRectToGroupGeometry,
  moveCanvasRect,
  type CanvasGroupFrame,
  type CanvasPoint,
  type CanvasRect,
} from '../../domain/graph/canvasGroupGeometry.ts'
import type { CanvasVueFlowGraphAdapter } from '../../infrastructure/vue-flow-adapter/useCanvasVueFlowGraphAdapter'

type ActiveGroupDrag = {
  transaction: CanvasGraphTransaction
  groupId: CanvasGraphGroupId
  startPoint: CanvasPoint
  latestPoint: CanvasPoint
  startPositions: Map<CanvasGraphNodeId, CanvasPoint>
  startBounds: CanvasRect
}

export type GroupDragRuntimeOptions = {
  groupFrames: ComputedRef<CanvasGroupFrame[]>
  graphAdapter: CanvasVueFlowGraphAdapter
  readCanvasGraphStore: () => CanvasGraphStore
  clearPendingGroupJoin: () => boolean
  selectGroup: (groupId: CanvasGraphGroupId) => boolean
  readGroupMemberNodePositions: (
    store: CanvasGraphStore,
    groupId: CanvasGraphGroupId,
  ) => Map<CanvasGraphNodeId, CanvasPoint> | null
  createPositionChanges: (
    positions: ReadonlyMap<CanvasGraphNodeId, CanvasPoint>,
    delta: CanvasPoint,
    dragging: boolean,
  ) => NodeChange[]
  setPreviewGroupBounds: (groupId: CanvasGraphGroupId, bounds: CanvasRect) => void
  clearPreviewGroupBounds: (groupId: CanvasGraphGroupId) => void
  findTopmostGroupFrame: (point: CanvasPoint) => CanvasGroupFrame | null
  onWarning: (message: string) => void
  moveFailedMessage: string
  isResizeActive: () => boolean
}

export function createGroupDragRuntime(options: GroupDragRuntimeOptions) {
  let activeGroupDrag: ActiveGroupDrag | null = null

  function begin(point: CanvasPoint): boolean {
    if (activeGroupDrag || options.isResizeActive()) return false
    options.clearPendingGroupJoin()
    const frame = options.findTopmostGroupFrame(point)
    if (!frame) return false
    const store = options.readCanvasGraphStore()
    const startPositions = options.readGroupMemberNodePositions(store, frame.id)
    if (!startPositions) return false
    options.selectGroup(frame.id)
    activeGroupDrag = {
      transaction: store.beginTransaction('group-move'),
      groupId: frame.id,
      startPoint: point,
      latestPoint: point,
      startPositions,
      startBounds: frame.bounds,
    }
    return true
  }

  function move(point: CanvasPoint): boolean {
    const drag = activeGroupDrag
    if (!drag) return false
    const delta = { x: point.x - drag.startPoint.x, y: point.y - drag.startPoint.y }
    drag.latestPoint = point
    if (delta.x === 0 && delta.y === 0) return true
    if (drag.startPositions.size > 0) {
      options.graphAdapter.applyNodeChanges(
        options.createPositionChanges(drag.startPositions, delta, true),
      )
    }
    options.setPreviewGroupBounds(drag.groupId, moveCanvasRect(drag.startBounds, delta))
    return true
  }

  function commit(): boolean {
    const drag = activeGroupDrag
    if (!drag) return false
    activeGroupDrag = null
    const delta = {
      x: drag.latestPoint.x - drag.startPoint.x,
      y: drag.latestPoint.y - drag.startPoint.y,
    }
    if (delta.x === 0 && delta.y === 0) {
      drag.transaction.cancel()
      options.graphAdapter.applyNodeChanges(
        options.createPositionChanges(drag.startPositions, delta, false),
      )
      options.clearPreviewGroupBounds(drag.groupId)
      return false
    }
    let summary: ReturnType<CanvasGraphTransaction['commit']>
    try {
      drag.startPositions.forEach((startPosition, nodeId) => {
        drag.transaction.updateNode(nodeId, {
          x: startPosition.x + delta.x,
          y: startPosition.y + delta.y,
        })
      })
      drag.transaction.updateGroupGeometry(
        drag.groupId,
        canvasRectToGroupGeometry(moveCanvasRect(drag.startBounds, delta)),
      )
      summary = drag.transaction.commit()
    } catch (_error) {
      drag.transaction.cancel()
      options.graphAdapter.applyNodeChanges(
        options.createPositionChanges(drag.startPositions, { x: 0, y: 0 }, false),
      )
      options.clearPreviewGroupBounds(drag.groupId)
      options.onWarning(options.moveFailedMessage)
      return false
    }
    options.graphAdapter.applyNodeChanges(
      options.createPositionChanges(drag.startPositions, delta, false),
    )
    options.clearPreviewGroupBounds(drag.groupId)
    return summary.didChange
  }

  function cancel(): boolean {
    const drag = activeGroupDrag
    if (!drag) return false
    activeGroupDrag = null
    drag.transaction.cancel()
    options.graphAdapter.applyNodeChanges(
      options.createPositionChanges(drag.startPositions, { x: 0, y: 0 }, false),
    )
    options.clearPreviewGroupBounds(drag.groupId)
    return true
  }

  return { begin, move, commit, cancel, isActive: () => activeGroupDrag !== null }
}
