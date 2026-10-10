import type { ComputedRef } from 'vue'
import {
  isCanvasGraphNodeId,
  readCanvasGraphNodeBounds,
  type CanvasGraphGroupId,
  type CanvasGraphNodeId,
  type CanvasGraphStore,
  type CanvasGraphTransaction,
} from '../../domain/graph'
import {
  canvasRectToGroupGeometry,
  rectsIntersect,
  resizeCanvasRectFromHandle,
  type CanvasGroupFrame,
  type CanvasGroupResizeHandle,
  type CanvasPoint,
  type CanvasRect,
} from '../../domain/graph/canvasGroupGeometry.ts'

type ActiveGroupResize = {
  store: CanvasGraphStore
  transaction: CanvasGraphTransaction
  groupId: CanvasGraphGroupId
  handle: CanvasGroupResizeHandle
  startPoint: CanvasPoint
  latestPoint: CanvasPoint
  startBounds: CanvasRect
}

export type GroupResizeRuntimeOptions = {
  groupFrames: ComputedRef<CanvasGroupFrame[]>
  readCanvasGraphStore: () => CanvasGraphStore
  isDragActive: () => boolean
  clearPendingGroupJoin: () => boolean
  selectGroup: (groupId: CanvasGraphGroupId) => boolean
  setPreviewGroupBounds: (groupId: CanvasGraphGroupId, bounds: CanvasRect) => void
  clearPreviewGroupBounds: (groupId: CanvasGraphGroupId) => void
  onWarning: (message: string) => void
  boundsUpdateFailedMessage: string
}

export type GroupResizeRuntime = {
  isActive: () => boolean
  begin: (
    groupId: CanvasGraphGroupId,
    handle: CanvasGroupResizeHandle,
    point: CanvasPoint,
  ) => boolean
  move: (point: CanvasPoint) => boolean
  commit: () => boolean
  cancel: () => boolean
}

export function createGroupResizeRuntime(options: GroupResizeRuntimeOptions): GroupResizeRuntime {
  let activeGroupResize: ActiveGroupResize | null = null

  function isActive(): boolean {
    return activeGroupResize !== null
  }

  function readGroupFrame(groupId: CanvasGraphGroupId): CanvasGroupFrame | null {
    return options.groupFrames.value.find((frame) => frame.id === groupId) ?? null
  }

  function readGroupMemberExitsForBounds(
    store: CanvasGraphStore,
    groupId: CanvasGraphGroupId,
    bounds: CanvasRect,
  ): CanvasGraphNodeId[] {
    const group = store.getGroup(groupId)
    if (!group) return []
    return group.nodeIds.filter(isCanvasGraphNodeId).filter((nodeId) => {
      const nodeBounds = readCanvasGraphNodeBounds(store, nodeId)
      return !nodeBounds || !rectsIntersect(bounds, nodeBounds)
    })
  }

  function begin(
    groupId: CanvasGraphGroupId,
    handle: CanvasGroupResizeHandle,
    point: CanvasPoint,
  ): boolean {
    if (options.isDragActive() || activeGroupResize) return false
    options.clearPendingGroupJoin()
    const frame = readGroupFrame(groupId)
    if (!frame) return false
    options.selectGroup(groupId)
    const store = options.readCanvasGraphStore()
    activeGroupResize = {
      store,
      transaction: store.beginTransaction('group-resize'),
      groupId,
      handle,
      startPoint: point,
      latestPoint: point,
      startBounds: frame.bounds,
    }
    return true
  }

  function move(point: CanvasPoint): boolean {
    const resize = activeGroupResize
    if (!resize) return false
    const delta = {
      x: point.x - resize.startPoint.x,
      y: point.y - resize.startPoint.y,
    }
    resize.latestPoint = point
    const nextBounds = resizeCanvasRectFromHandle({
      bounds: resize.startBounds,
      handle: resize.handle,
      delta,
    })
    options.setPreviewGroupBounds(resize.groupId, nextBounds)
    return true
  }

  function commit(): boolean {
    const resize = activeGroupResize
    if (!resize) return false
    activeGroupResize = null
    const delta = {
      x: resize.latestPoint.x - resize.startPoint.x,
      y: resize.latestPoint.y - resize.startPoint.y,
    }
    const nextBounds = resizeCanvasRectFromHandle({
      bounds: resize.startBounds,
      handle: resize.handle,
      delta,
    })
    if (
      nextBounds.left === resize.startBounds.left &&
      nextBounds.top === resize.startBounds.top &&
      nextBounds.width === resize.startBounds.width &&
      nextBounds.height === resize.startBounds.height
    ) {
      resize.transaction.cancel()
      options.clearPreviewGroupBounds(resize.groupId)
      return false
    }
    let summary: ReturnType<CanvasGraphTransaction['commit']>
    try {
      resize.transaction.updateGroupGeometry(resize.groupId, canvasRectToGroupGeometry(nextBounds))
      readGroupMemberExitsForBounds(resize.store, resize.groupId, nextBounds).forEach((nodeId) => {
        resize.transaction.moveNodeToGroup(nodeId, null)
      })
      summary = resize.transaction.commit()
    } catch (_error) {
      resize.transaction.cancel()
      options.clearPreviewGroupBounds(resize.groupId)
      options.onWarning(options.boundsUpdateFailedMessage)
      return false
    }
    options.clearPreviewGroupBounds(resize.groupId)
    return summary.didChange
  }

  function cancel(): boolean {
    const resize = activeGroupResize
    if (!resize) return false
    activeGroupResize = null
    resize.transaction.cancel()
    options.clearPreviewGroupBounds(resize.groupId)
    return true
  }

  return {
    isActive,
    begin,
    move,
    commit,
    cancel,
  }
}
