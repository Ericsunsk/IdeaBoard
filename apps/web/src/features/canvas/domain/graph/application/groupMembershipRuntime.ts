import { computed, ref, type ComputedRef, type Ref } from 'vue'
import type { Node, NodeChange } from '@vue-flow/core'
import type { CanvasGroup } from '../../../../board'
import {
  CanvasGraphStaleTransactionError,
  isCanvasGraphNodeId,
  readCanvasGraphNodeBounds,
  resolveCanvasGraphNodeDimensions,
  type CanvasGraphGroupId,
  type CanvasGraphNodeId,
  type CanvasGraphStore,
  type CanvasGraphTransaction,
} from '..'
import {
  rectContainsRect,
  rectsIntersect,
  type CanvasGroupFrame,
  type CanvasPoint,
  type CanvasRect,
} from '../canvasGroupGeometry.ts'

export type PendingGroupJoin = {
  groupId: CanvasGraphGroupId
  nodeIds: CanvasGraphNodeId[]
}

export type ResolvedFlowNodeChanges = {
  graphNodeIds: CanvasGraphNodeId[]
  flowNodeIds: string[]
}

export type GroupMembershipRuntimeOptions = {
  readCanvasGraphStore: () => CanvasGraphStore
  isGraphSessionAttached: () => boolean
  selectedGroupId: Ref<CanvasGraphGroupId | null>
  groupFrames: ComputedRef<CanvasGroupFrame[]>
  readGroup: (groupId: CanvasGraphGroupId | null) => CanvasGroup | null
  resolveGraphNodeIdForFlowNode: (node: Pick<Node, 'id' | 'data'>) => CanvasGraphNodeId | null
  resolveGraphNodeIdForFlowNodeId: (flowNodeId: string) => CanvasGraphNodeId | null
  selectOnlyFlowNodesByGraphIds: (nodeIds: readonly CanvasGraphNodeId[]) => boolean
  clearSelectedGroup: () => boolean
  onWarning: (message: string) => void
  updateFailedMessage: string
}

export type GroupMembershipRuntime = {
  pendingGroupJoin: Ref<PendingGroupJoin | null>
  selectedGroupCoveredExternalNodeIds: ComputedRef<CanvasGraphNodeId[]>
  clearPendingGroupJoin: () => boolean
  prunePendingGroupJoin: () => boolean
  capturePendingGroupJoinFromSelectedGroup: () => boolean
  joinPendingGroupNode: (flowNode: Pick<Node, 'id' | 'data'>) => boolean
  applyDraggedNodeMembershipChanges: (
    store: CanvasGraphStore,
    transaction: CanvasGraphTransaction,
    changes: readonly NodeChange[],
  ) => (() => void) | void
  resolveRemovedFlowNodeChanges: (changes: readonly NodeChange[]) => ResolvedFlowNodeChanges | null
  clearPendingGroupJoinForNodeChanges: (changes: NodeChange[]) => void
}

export function createGroupMembershipRuntime(
  options: GroupMembershipRuntimeOptions,
): GroupMembershipRuntime {
  const pendingGroupJoin = ref<PendingGroupJoin | null>(null)

  function clearPendingGroupJoin(): boolean {
    if (!pendingGroupJoin.value) return false
    pendingGroupJoin.value = null
    return true
  }

  function prunePendingGroupJoin() {
    const pending = pendingGroupJoin.value
    if (!pending) return false
    const store = options.readCanvasGraphStore()
    if (!store.getGroup(pending.groupId)) return clearPendingGroupJoin()
    const validNodeIds = pending.nodeIds.filter(
      (nodeId) => !store.getGroupIdForNode(nodeId) && !!store.getNode(nodeId),
    )
    if (validNodeIds.length === pending.nodeIds.length) return false
    if (validNodeIds.length === 0) return clearPendingGroupJoin()
    pendingGroupJoin.value = { ...pending, nodeIds: validNodeIds }
    return true
  }

  const selectedGroupCoveredExternalNodeIds = computed(() => {
    if (!options.isGraphSessionAttached()) return []
    const groupId = options.selectedGroupId.value
    if (!groupId) return []
    const frame = options.groupFrames.value.find((item) => item.id === groupId)
    if (!frame) return []
    const store = options.readCanvasGraphStore()

    const coveredNodeIds: CanvasGraphNodeId[] = []
    store.getNodeIds().forEach((nodeId) => {
      if (store.getGroupIdForNode(nodeId)) return
      const nodeBounds = readCanvasGraphNodeBounds(store, nodeId)
      if (nodeBounds && rectsIntersect(frame.bounds, nodeBounds)) coveredNodeIds.push(nodeId)
    })
    return coveredNodeIds
  })

  function capturePendingGroupJoinFromSelectedGroup(): boolean {
    const groupId = options.selectedGroupId.value
    if (!groupId || !options.readGroup(groupId)) return clearPendingGroupJoin()
    const nodeIds = selectedGroupCoveredExternalNodeIds.value
    if (nodeIds.length === 0) return clearPendingGroupJoin()
    pendingGroupJoin.value = { groupId, nodeIds }
    return true
  }

  function joinPendingGroupNode(flowNode: Pick<Node, 'id' | 'data'>): boolean {
    const pending = pendingGroupJoin.value
    if (!pending) return false
    const nodeId = options.resolveGraphNodeIdForFlowNode(flowNode)
    clearPendingGroupJoin()
    if (!nodeId || !pending.nodeIds.includes(nodeId)) return false

    const store = options.readCanvasGraphStore()
    if (!store.getGroup(pending.groupId) || store.getGroupIdForNode(nodeId)) return false
    const transaction = store.beginTransaction('group-membership')
    let summary: ReturnType<CanvasGraphTransaction['commit']>
    try {
      summary = transaction.moveNodeToGroup(nodeId, pending.groupId).commit()
    } catch (_error) {
      transaction.cancel()
      options.onWarning(options.updateFailedMessage)
      return false
    }
    if (!summary.didChange) return false
    options.selectOnlyFlowNodesByGraphIds([nodeId])
    return true
  }

  function readNodeBoundsForMembership(
    store: CanvasGraphStore,
    nodeId: CanvasGraphNodeId,
    positions?: ReadonlyMap<CanvasGraphNodeId, CanvasPoint>,
  ): CanvasRect | null {
    const position = positions?.get(nodeId)
    if (!position) return readCanvasGraphNodeBounds(store, nodeId)
    const node = store.getNode(nodeId)
    if (!node) return null
    const dimensions = resolveCanvasGraphNodeDimensions(node.kind, node.snapshot)
    return {
      left: position.x,
      top: position.y,
      right: position.x + dimensions.width,
      bottom: position.y + dimensions.height,
      ...dimensions,
    }
  }

  function resolveNodeContainedGroupIds(
    store: CanvasGraphStore,
    nodeId: CanvasGraphNodeId,
    positions?: ReadonlyMap<CanvasGraphNodeId, CanvasPoint>,
  ): CanvasGraphGroupId[] {
    const nodeBounds = readNodeBoundsForMembership(store, nodeId, positions)
    if (!nodeBounds) return []
    return options.groupFrames.value
      .filter((frame) => rectContainsRect(frame.bounds, nodeBounds))
      .map((frame) => frame.id)
  }

  function isNodeIntersectingGroup(
    store: CanvasGraphStore,
    nodeId: CanvasGraphNodeId,
    groupId: CanvasGraphGroupId,
    positions?: ReadonlyMap<CanvasGraphNodeId, CanvasPoint>,
  ): boolean {
    const frame = options.groupFrames.value.find((item) => item.id === groupId)
    const nodeBounds = readNodeBoundsForMembership(store, nodeId, positions)
    if (!nodeBounds || !frame) return false
    return rectsIntersect(frame.bounds, nodeBounds)
  }

  function resolveNodeMembershipTarget(
    store: CanvasGraphStore,
    nodeId: CanvasGraphNodeId,
    positions?: ReadonlyMap<CanvasGraphNodeId, CanvasPoint>,
  ): CanvasGraphGroupId | null | undefined {
    const currentGroupId = store.getGroupIdForNode(nodeId)
    const containingGroupIds = resolveNodeContainedGroupIds(store, nodeId, positions)
    if (currentGroupId) {
      if (isNodeIntersectingGroup(store, nodeId, currentGroupId, positions)) return undefined
      return containingGroupIds.length === 1 ? containingGroupIds[0] : null
    }

    if (containingGroupIds.length !== 1) return undefined
    return containingGroupIds[0]
  }

  function stageNodeMembershipReconciliation(
    store: CanvasGraphStore,
    transaction: CanvasGraphTransaction,
    nodeIds: readonly CanvasGraphNodeId[],
    positions: ReadonlyMap<CanvasGraphNodeId, CanvasPoint>,
  ): void {
    nodeIds.forEach((nodeId) => {
      const targetGroupId = resolveNodeMembershipTarget(store, nodeId, positions)
      if (targetGroupId !== undefined) transaction.moveNodeToGroup(nodeId, targetGroupId)
    })
  }

  function stageDraggedNodeMembershipChanges(
    store: CanvasGraphStore,
    transaction: CanvasGraphTransaction,
    changes: readonly NodeChange[],
  ): (() => void) | void {
    const positions = new Map<CanvasGraphNodeId, CanvasPoint>()
    changes.forEach((change) => {
      if (change.type !== 'position' || !isCanvasGraphNodeId(change.id)) return
      positions.set(change.id, change.position)
    })
    const nodeIds = Array.from(positions.keys())
    if (nodeIds.length === 0) return
    const selectedGroupIdBefore = options.selectedGroupId.value
    const selectedGroupBefore = selectedGroupIdBefore
      ? store.getGroup(selectedGroupIdBefore)
      : undefined
    const selectedGroupNodeIdsBefore = selectedGroupBefore
      ? new Set(selectedGroupBefore.nodeIds.filter(isCanvasGraphNodeId))
      : null
    try {
      stageNodeMembershipReconciliation(store, transaction, nodeIds, positions)
    } catch (error) {
      if (error instanceof CanvasGraphStaleTransactionError) throw error
      options.onWarning(options.updateFailedMessage)
      return
    }
    return () => {
      if (!selectedGroupIdBefore || !selectedGroupNodeIdsBefore) return
      const selectedGroupAfter = store.getGroup(selectedGroupIdBefore)
      const selectedGroupNodeIdsAfter = new Set(
        selectedGroupAfter?.nodeIds.filter(isCanvasGraphNodeId) ?? [],
      )
      const movedOutNodeIds = nodeIds.filter(
        (nodeId) =>
          selectedGroupNodeIdsBefore.has(nodeId) && !selectedGroupNodeIdsAfter.has(nodeId),
      )
      if (movedOutNodeIds.length === 0) return
      options.clearSelectedGroup()
      options.selectOnlyFlowNodesByGraphIds(movedOutNodeIds)
    }
  }

  function resolveRemovedFlowNodeChanges(
    changes: readonly NodeChange[],
  ): ResolvedFlowNodeChanges | null {
    const graphNodeIds: CanvasGraphNodeId[] = []
    const flowNodeIds: string[] = []
    for (const change of changes) {
      if (change.type !== 'remove') continue
      const graphNodeId = options.resolveGraphNodeIdForFlowNodeId(change.id)
      if (!graphNodeId) return null
      graphNodeIds.push(graphNodeId)
      flowNodeIds.push(change.id)
    }
    return {
      graphNodeIds,
      flowNodeIds,
    }
  }

  function clearPendingGroupJoinForNodeChanges(changes: NodeChange[]) {
    const pending = pendingGroupJoin.value
    if (!pending) return
    const pendingNodeIds = new Set(pending.nodeIds)
    if (
      changes.some((change) => {
        if (change.type !== 'position' && change.type !== 'remove') return false
        const nodeId = options.resolveGraphNodeIdForFlowNodeId(change.id)
        return !!nodeId && pendingNodeIds.has(nodeId)
      })
    ) {
      clearPendingGroupJoin()
    }
  }

  return {
    pendingGroupJoin,
    selectedGroupCoveredExternalNodeIds,
    clearPendingGroupJoin,
    prunePendingGroupJoin,
    capturePendingGroupJoinFromSelectedGroup,
    joinPendingGroupNode,
    applyDraggedNodeMembershipChanges: stageDraggedNodeMembershipChanges,
    resolveRemovedFlowNodeChanges,
    clearPendingGroupJoinForNodeChanges,
  }
}
