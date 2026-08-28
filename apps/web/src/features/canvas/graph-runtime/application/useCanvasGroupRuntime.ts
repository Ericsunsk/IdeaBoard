import { computed, ref, type ComputedRef, type Ref } from 'vue'
import { i18n } from '../../../../shared/i18n'
import type { Edge, EdgeChange, Node, NodeChange } from '@vue-flow/core'
import type { CanvasBoardSnapshot } from '../../../canvas-board/domain'
import type { PendingImageExecutionLease } from '../../execution/pending-execution'
import {
  commitCanvasGraphDeleteGroupIntent,
  CanvasGraphStaleTransactionError,
  isCanvasGraphNodeId,
  readCanvasGraphNodeBounds,
  resolveCanvasGraphNodeDimensions,
  resolveCanvasGraphNodesBounds,
  toCanvasGraphNodeId,
  type CanvasGraphGroupId,
  type CanvasGraphNodeId,
  type CanvasGraphStore,
  type CanvasGraphTransaction,
} from '..'
import {
  CANVAS_GROUP_FRAME_PADDING_PX,
  canvasRectToGroupGeometry,
  findTopmostCanvasGroupFrameAtPoint,
  moveCanvasRect,
  projectCanvasGroupNodeLayers,
  rectContainsRect,
  rectsIntersect,
  resizeCanvasRectFromHandle,
  resolveCanvasGroupFrames,
  type CanvasGroupFrame,
  type CanvasGroupResizeHandle,
  type CanvasPoint,
  type CanvasRect,
} from '../../utils/canvasGroupGeometry'
import { resolveFlowNodeCanvasRef } from '../../utils/flowSnapshotAdapter'
import { releaseCanvasGraphDeleteExecutions } from '../../vue-flow-adapter/flowGraphDeleteProjection'
import type { CanvasVueFlowGraphAdapter } from '../../vue-flow-adapter/useCanvasVueFlowGraphAdapter'

type SelectableFlowItem = {
  id: string
  selected?: boolean
}

type ActiveGroupDrag = {
  store: CanvasGraphStore
  transaction: CanvasGraphTransaction
  groupId: CanvasGraphGroupId
  startPoint: CanvasPoint
  latestPoint: CanvasPoint
  startPositions: Map<CanvasGraphNodeId, CanvasPoint>
  startBounds: CanvasRect
}

type ActiveGroupResize = {
  store: CanvasGraphStore
  transaction: CanvasGraphTransaction
  groupId: CanvasGraphGroupId
  handle: CanvasGroupResizeHandle
  startPoint: CanvasPoint
  latestPoint: CanvasPoint
  startBounds: CanvasRect
}

type PendingGroupJoin = {
  groupId: CanvasGraphGroupId
  nodeIds: CanvasGraphNodeId[]
}

export type CanvasSelectionGroupAction =
  | {
      kind: 'group'
      label: string
      nodeIds: CanvasGraphNodeId[]
    }
  | {
      kind: 'ungroup'
      label: string
      groupId: CanvasGraphGroupId
      nodeIds: CanvasGraphNodeId[]
    }

export type CanvasGroupRuntimeView = {
  groupFrames: ComputedRef<CanvasGroupFrame[]>
  selectionGroupAction: ComputedRef<CanvasSelectionGroupAction | null>
}

export type CanvasGroupNodeClickPayload = {
  node: Pick<Node, 'id' | 'data'>
  selectNode: () => void
}

export type CanvasGroupRuntimeCommands = {
  selectGroupAtPoint: (point: CanvasPoint) => boolean
  handlePaneClick: (selectPane: () => void) => void
  handleNodeClick: (payload: CanvasGroupNodeClickPayload) => void
  groupSelectedNodes: () => boolean
  ungroupSelectedGroup: () => boolean
  deleteSelectedGroup: () => boolean
  groupDrag: {
    begin: (point: CanvasPoint) => boolean
    move: (point: CanvasPoint) => boolean
    commit: () => boolean
    cancel: () => boolean
  }
  groupResize: {
    begin: (
      groupId: CanvasGraphGroupId,
      handle: CanvasGroupResizeHandle,
      point: CanvasPoint,
    ) => boolean
    move: (point: CanvasPoint) => boolean
    commit: () => boolean
    cancel: () => boolean
  }
}

type ResolvedFlowNodeChanges = {
  graphNodeIds: CanvasGraphNodeId[]
  flowNodeIds: string[]
}

export type CanvasGroupRuntimeInternalHooks = {
  selectedGroupId: Ref<CanvasGraphGroupId | null>
  selectedGroupCoveredExternalNodeIds: ComputedRef<CanvasGraphNodeId[]>
  projectCurrentGroupNodeLayers: (nodes: readonly Node[]) => Node[]
  projectCommittedGroupNodeLayer: (node: Node) => Node
  handleCommittedGroupsChanged: () => void
  resetTransientSelectionState: () => void
  clearPendingGroupJoin: () => boolean
  onNodeDragStart: () => void
  onBeforeNodesChange: (changes: NodeChange[]) => void
  onBeforeEdgesChange: (changes: EdgeChange[]) => void
  resolveRemovedFlowNodeChanges: (changes: readonly NodeChange[]) => ResolvedFlowNodeChanges | null
  applyDraggedNodeMembershipChanges: (
    store: CanvasGraphStore,
    transaction: CanvasGraphTransaction,
    changes: readonly NodeChange[],
  ) => (() => void) | void
}

export type CanvasGroupRuntimeControls = {
  view: CanvasGroupRuntimeView
  commands: CanvasGroupRuntimeCommands
  internal: CanvasGroupRuntimeInternalHooks
}

export type UseCanvasGroupRuntimeOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  boardBaseSnapshot: Ref<CanvasBoardSnapshot>
  graphAdapter: CanvasVueFlowGraphAdapter
  readCanvasGraphStore: () => CanvasGraphStore
  isGraphSessionAttached: () => boolean
  onWarning: (message: string) => void
  captureActiveNodeExecutions: (nodeIds: readonly string[]) => PendingImageExecutionLease[]
  releaseNodeExecutions: (leases: readonly PendingImageExecutionLease[]) => void
}

export function useCanvasGroupRuntime(
  options: UseCanvasGroupRuntimeOptions,
): CanvasGroupRuntimeControls {
  let activeGroupDrag: ActiveGroupDrag | null = null
  let activeGroupResize: ActiveGroupResize | null = null
  const selectedGroupId = ref<CanvasGraphGroupId | null>(null)
  const pendingGroupJoin = ref<PendingGroupJoin | null>(null)
  const previewGroupBounds = ref<ReadonlyMap<CanvasGraphGroupId, CanvasRect>>(new Map())

  function projectCurrentGroupNodeLayers(nodes: readonly Node[]) {
    return projectCanvasGroupNodeLayers(nodes, options.boardBaseSnapshot.value.groups) as Node[]
  }

  function projectCommittedGroupNodeLayer(node: Node): Node {
    return projectCurrentGroupNodeLayers([node])[0] ?? node
  }

  function hasGroup(groupId: CanvasGraphGroupId | null): groupId is CanvasGraphGroupId {
    return !!groupId && options.boardBaseSnapshot.value.groups.some((group) => group.id === groupId)
  }

  function pruneSelectedGroup() {
    if (!selectedGroupId.value) return false
    if (hasGroup(selectedGroupId.value)) return false
    selectedGroupId.value = null
    return true
  }

  function handleCommittedGroupsChanged() {
    pruneSelectedGroup()
    prunePendingGroupJoin()
  }

  function clearFlowSelection(): boolean {
    let didChange = false
    const nextNodes = options.nodes.value.map((node) => {
      if ((node as SelectableFlowItem).selected !== true) return node
      didChange = true
      return { ...node, selected: false }
    })
    const nextEdges = options.edges.value.map((edge) => {
      if ((edge as SelectableFlowItem).selected !== true) return edge
      didChange = true
      return { ...edge, selected: false }
    })
    if (!didChange) return false
    options.graphAdapter.replaceGraph({
      nodes: nextNodes,
      edges: nextEdges,
    })
    return true
  }

  function selectOnlyFlowNodesByGraphIds(nodeIds: readonly CanvasGraphNodeId[]): boolean {
    const selectedNodeIds = new Set(nodeIds)
    let didChange = false
    const nextNodes = options.nodes.value.map((node) => {
      const shouldSelect = selectedNodeIds.has(node.id as CanvasGraphNodeId)
      if (((node as SelectableFlowItem).selected === true) === shouldSelect) return node
      didChange = true
      return { ...node, selected: shouldSelect }
    })
    const nextEdges = options.edges.value.map((edge) => {
      if ((edge as SelectableFlowItem).selected !== true) return edge
      didChange = true
      return { ...edge, selected: false }
    })
    if (!didChange) return false
    options.graphAdapter.replaceGraph({
      nodes: nextNodes,
      edges: nextEdges,
    })
    return true
  }

  function resolveGraphNodeIdForFlowNode(
    node: Pick<Node, 'id' | 'data'>,
  ): CanvasGraphNodeId | null {
    const ref = resolveFlowNodeCanvasRef(node)
    return ref ? toCanvasGraphNodeId(ref.nodeType, ref.id) : null
  }

  function resolveGraphNodeIdForFlowNodeId(flowNodeId: string): CanvasGraphNodeId | null {
    const node = options.nodes.value.find((item) => item.id === flowNodeId)
    return resolveGraphNodeIdForFlowNode(node ?? { id: flowNodeId, data: undefined })
  }

  function hasSelectedEdges() {
    return options.edges.value.some((edge) => (edge as SelectableFlowItem).selected === true)
  }

  function readSelectedGraphNodeIds(): CanvasGraphNodeId[] | null {
    const selectedFlowNodeIds = options.nodes.value
      .filter((node) => (node as SelectableFlowItem).selected === true)
      .map((node) => node.id)
    if (selectedFlowNodeIds.length === 0) return []

    const selectedGraphNodeIds = selectedFlowNodeIds
      .map((nodeId) => resolveGraphNodeIdForFlowNodeId(nodeId))
      .filter((nodeId): nodeId is CanvasGraphNodeId => nodeId !== null)
    return selectedGraphNodeIds.length === selectedFlowNodeIds.length ? selectedGraphNodeIds : null
  }

  function isSameNodeSet(left: readonly CanvasGraphNodeId[], right: readonly CanvasGraphNodeId[]) {
    if (left.length !== right.length) return false
    const rightIds = new Set(right)
    return left.every((nodeId) => rightIds.has(nodeId))
  }

  function readGroupedNodeIds() {
    const groupedNodeIds = new Set<CanvasGraphNodeId>()
    options.boardBaseSnapshot.value.groups.forEach((group) => {
      group.nodeIds.forEach((nodeId) => {
        if (isCanvasGraphNodeId(nodeId)) groupedNodeIds.add(nodeId)
      })
    })
    return groupedNodeIds
  }

  const groupFrames = computed(() =>
    resolveCanvasGroupFrames({
      groups: options.boardBaseSnapshot.value.groups,
      selectedGroupId: selectedGroupId.value,
    }).map((frame) => {
      const previewBounds = previewGroupBounds.value.get(frame.id)
      return previewBounds ? { ...frame, bounds: previewBounds } : frame
    }),
  )

  function setPreviewGroupBounds(groupId: CanvasGraphGroupId, bounds: CanvasRect) {
    const nextBounds = new Map(previewGroupBounds.value)
    nextBounds.set(groupId, bounds)
    previewGroupBounds.value = nextBounds
  }

  function clearPreviewGroupBounds(groupId: CanvasGraphGroupId) {
    if (!previewGroupBounds.value.has(groupId)) return
    const nextBounds = new Map(previewGroupBounds.value)
    nextBounds.delete(groupId)
    previewGroupBounds.value = nextBounds
  }

  const selectedGroupCoveredExternalNodeIds = computed(() => {
    if (!options.isGraphSessionAttached()) return []
    const groupId = selectedGroupId.value
    if (!groupId) return []
    const frame = groupFrames.value.find((item) => item.id === groupId)
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

  function readGroup(groupId: CanvasGraphGroupId | null) {
    if (!groupId) return null
    return options.boardBaseSnapshot.value.groups.find((group) => group.id === groupId) ?? null
  }

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

  function selectGroup(groupId: CanvasGraphGroupId): boolean {
    if (!readGroup(groupId)) return false
    if (selectedGroupId.value !== groupId) clearPendingGroupJoin()
    clearFlowSelection()
    const didChange = selectedGroupId.value !== groupId
    selectedGroupId.value = groupId
    return didChange
  }

  function clearSelectedGroup(): boolean {
    if (!selectedGroupId.value) return false
    selectedGroupId.value = null
    return true
  }

  function selectGroupAtPoint(point: CanvasPoint): boolean {
    const frame = findTopmostCanvasGroupFrameAtPoint(groupFrames.value, point)
    if (!frame) return false
    selectGroup(frame.id)
    return true
  }

  function capturePendingGroupJoinFromSelectedGroup(): boolean {
    const groupId = selectedGroupId.value
    if (!groupId || !readGroup(groupId)) return clearPendingGroupJoin()
    const nodeIds = selectedGroupCoveredExternalNodeIds.value
    if (nodeIds.length === 0) return clearPendingGroupJoin()
    pendingGroupJoin.value = { groupId, nodeIds }
    return true
  }

  function joinPendingGroupNode(flowNode: Pick<Node, 'id' | 'data'>): boolean {
    const pending = pendingGroupJoin.value
    if (!pending) return false
    const nodeId = resolveGraphNodeIdForFlowNode(flowNode)
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
      options.onWarning(i18n.global.t('canvas.groupOps.updateFailed'))
      return false
    }
    if (!summary.didChange) return false
    selectOnlyFlowNodesByGraphIds([nodeId])
    return true
  }

  function handlePaneClick(selectPane: () => void) {
    capturePendingGroupJoinFromSelectedGroup()
    selectPane()
    clearSelectedGroup()
  }

  function handleNodeClick(payload: CanvasGroupNodeClickPayload) {
    joinPendingGroupNode(payload.node)
    payload.selectNode()
  }

  function isSelectedGroupNodeDeselection(change: NodeChange | EdgeChange): boolean {
    if (change.type !== 'select' || change.selected !== false) return false
    const selectedGroup = readGroup(selectedGroupId.value)
    if (!selectedGroup) return false
    const graphNodeId = resolveGraphNodeIdForFlowNodeId(change.id)
    return !!graphNodeId && selectedGroup.nodeIds.includes(graphNodeId)
  }

  function maybeClearSelectedGroupForFlowSelection(changes: Array<NodeChange | EdgeChange>) {
    if (!selectedGroupId.value) return
    if (
      changes.some(
        (change) =>
          (change.type === 'select' && change.selected === true) ||
          isSelectedGroupNodeDeselection(change),
      )
    ) {
      clearSelectedGroup()
    }
  }

  function resolveSelectedGroupAction(): CanvasSelectionGroupAction | null {
    const selectedNodeIds = readSelectedGraphNodeIds()
    if (!selectedNodeIds) return null

    const selectedGroup = readGroup(selectedGroupId.value)
    if (selectedGroup && !hasSelectedEdges() && selectedNodeIds.length === 0) {
      const nodeIds = selectedGroup.nodeIds.filter(isCanvasGraphNodeId)
      return {
        kind: 'ungroup',
        label: i18n.global.t('canvas.controls.ungroupSelection'),
        groupId: selectedGroup.id as CanvasGraphGroupId,
        nodeIds,
      }
    }

    if (hasSelectedEdges() || selectedNodeIds.length < 2) return null

    const matchingGroups = options.boardBaseSnapshot.value.groups.filter((group) =>
      isSameNodeSet(group.nodeIds as CanvasGraphNodeId[], selectedNodeIds),
    )
    if (matchingGroups.length === 1) {
      return {
        kind: 'ungroup',
        label: i18n.global.t('canvas.controls.ungroupSelection'),
        groupId: matchingGroups[0].id,
        nodeIds: selectedNodeIds,
      }
    }
    if (matchingGroups.length > 1) return null

    const groupedNodeIds = readGroupedNodeIds()
    if (selectedNodeIds.some((nodeId) => groupedNodeIds.has(nodeId))) return null

    return {
      kind: 'group',
      label: i18n.global.t('canvas.controls.groupSelection'),
      nodeIds: selectedNodeIds,
    }
  }

  const selectionGroupAction = computed(resolveSelectedGroupAction)

  function groupSelectedNodes(): boolean {
    clearPendingGroupJoin()
    if (!options.isGraphSessionAttached()) return false
    const action = selectionGroupAction.value
    if (action?.kind !== 'group') return false
    const store = options.readCanvasGraphStore()
    const bounds = resolveCanvasGraphNodesBounds(
      store,
      action.nodeIds,
      CANVAS_GROUP_FRAME_PADDING_PX,
    )
    if (!bounds) {
      options.onWarning(i18n.global.t('canvas.groupOps.groupFailed'))
      return false
    }

    const transaction = store.beginTransaction('group')
    let summary: ReturnType<CanvasGraphTransaction['commit']>
    try {
      summary = transaction.createGroup(action.nodeIds, canvasRectToGroupGeometry(bounds)).commit()
    } catch (_error) {
      transaction.cancel()
      options.onWarning(i18n.global.t('canvas.groupOps.groupFailed'))
      return false
    }
    if (!summary.didChange) return false
    const createdGroupId =
      summary.projectionPatch.groups.added[0]?.id ?? summary.changedGroupIds[0] ?? null
    clearFlowSelection()
    if (createdGroupId) selectedGroupId.value = createdGroupId
    return true
  }

  function ungroupSelectedGroup(): boolean {
    clearPendingGroupJoin()
    if (!options.isGraphSessionAttached()) return false
    const action = selectionGroupAction.value
    if (action?.kind !== 'ungroup') return false

    const store = options.readCanvasGraphStore()
    const transaction = store.beginTransaction('ungroup')
    let didChange: boolean
    try {
      didChange = transaction.ungroup(action.groupId).commit().didChange
    } catch (_error) {
      transaction.cancel()
      options.onWarning(i18n.global.t('canvas.groupOps.ungroupFailed'))
      return false
    }
    if (!didChange) return false
    if (selectedGroupId.value === action.groupId) clearSelectedGroup()
    return true
  }

  function createPositionChanges(
    positions: ReadonlyMap<CanvasGraphNodeId, CanvasPoint>,
    delta: CanvasPoint,
    dragging: boolean,
  ): NodeChange[] {
    return Array.from(positions, ([nodeId, startPosition]) => ({
      id: nodeId,
      type: 'position',
      position: {
        x: startPosition.x + delta.x,
        y: startPosition.y + delta.y,
      },
      dragging,
    })) as NodeChange[]
  }

  function deleteSelectedGroup(): boolean {
    clearPendingGroupJoin()
    const group = readGroup(selectedGroupId.value)
    if (!group) return false

    const nodeIds = group.nodeIds.filter(isCanvasGraphNodeId)
    const store = options.readCanvasGraphStore()
    const shouldDeleteMembers = nodeIds.length > 0
    const summary = commitCanvasGraphDeleteGroupIntent(store, {
      groupId: group.id as CanvasGraphGroupId,
      deleteMembers: shouldDeleteMembers,
      removeConnectedEdges: true,
    })
    if (!summary.didChange) return false

    if (!shouldDeleteMembers) return true

    releaseCanvasGraphDeleteExecutions({
      summary,
      nodes: options.nodes.value,
      edges: options.edges.value,
      captureActiveNodeExecutions: options.captureActiveNodeExecutions,
      releaseNodeExecutions: options.releaseNodeExecutions,
    })
    return true
  }

  function readGroupMemberNodePositions(store: CanvasGraphStore, groupId: CanvasGraphGroupId) {
    const group = store.getGroup(groupId)
    if (!group) return null
    const positions = new Map<CanvasGraphNodeId, CanvasPoint>()
    group.nodeIds.forEach((nodeId) => {
      const node = store.getNode(nodeId)
      if (!node) return
      positions.set(nodeId, { x: node.snapshot.x, y: node.snapshot.y })
    })
    return positions
  }

  function beginGroupDrag(point: CanvasPoint): boolean {
    if (activeGroupDrag || activeGroupResize) return false
    clearPendingGroupJoin()
    const frame = findTopmostCanvasGroupFrameAtPoint(groupFrames.value, point)
    if (!frame) return false
    const store = options.readCanvasGraphStore()
    const startPositions = readGroupMemberNodePositions(store, frame.id)
    if (!startPositions) return false
    selectGroup(frame.id)
    activeGroupDrag = {
      store,
      transaction: store.beginTransaction('group-move'),
      groupId: frame.id,
      startPoint: point,
      latestPoint: point,
      startPositions,
      startBounds: frame.bounds,
    }
    return true
  }

  function moveGroupDrag(point: CanvasPoint): boolean {
    const drag = activeGroupDrag
    if (!drag) return false
    const delta = {
      x: point.x - drag.startPoint.x,
      y: point.y - drag.startPoint.y,
    }
    drag.latestPoint = point
    if (delta.x === 0 && delta.y === 0) return true
    if (drag.startPositions.size > 0) {
      options.graphAdapter.applyNodeChanges(createPositionChanges(drag.startPositions, delta, true))
    }
    setPreviewGroupBounds(drag.groupId, moveCanvasRect(drag.startBounds, delta))
    return true
  }

  function commitGroupDrag(): boolean {
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
        createPositionChanges(drag.startPositions, delta, false),
      )
      clearPreviewGroupBounds(drag.groupId)
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
        createPositionChanges(drag.startPositions, { x: 0, y: 0 }, false),
      )
      clearPreviewGroupBounds(drag.groupId)
      options.onWarning(i18n.global.t('canvas.groupOps.moveFailed'))
      return false
    }
    options.graphAdapter.applyNodeChanges(createPositionChanges(drag.startPositions, delta, false))
    clearPreviewGroupBounds(drag.groupId)
    if (!summary.didChange) return false
    return true
  }

  function cancelGroupDrag(): boolean {
    const drag = activeGroupDrag
    if (!drag) return false
    activeGroupDrag = null
    drag.transaction.cancel()
    options.graphAdapter.applyNodeChanges(
      createPositionChanges(drag.startPositions, { x: 0, y: 0 }, false),
    )
    clearPreviewGroupBounds(drag.groupId)
    return true
  }

  function readGroupFrame(groupId: CanvasGraphGroupId): CanvasGroupFrame | null {
    return groupFrames.value.find((frame) => frame.id === groupId) ?? null
  }

  function beginGroupResize(
    groupId: CanvasGraphGroupId,
    handle: CanvasGroupResizeHandle,
    point: CanvasPoint,
  ): boolean {
    if (activeGroupDrag || activeGroupResize) return false
    clearPendingGroupJoin()
    const frame = readGroupFrame(groupId)
    if (!frame) return false
    selectGroup(groupId)
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

  function moveGroupResize(point: CanvasPoint): boolean {
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
    setPreviewGroupBounds(resize.groupId, nextBounds)
    return true
  }

  function commitGroupResize(): boolean {
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
      clearPreviewGroupBounds(resize.groupId)
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
      clearPreviewGroupBounds(resize.groupId)
      options.onWarning(i18n.global.t('canvas.groupOps.boundsUpdateFailed'))
      return false
    }
    clearPreviewGroupBounds(resize.groupId)
    if (!summary.didChange) return false
    return true
  }

  function cancelGroupResize(): boolean {
    const resize = activeGroupResize
    if (!resize) return false
    activeGroupResize = null
    resize.transaction.cancel()
    clearPreviewGroupBounds(resize.groupId)
    return true
  }

  function resolveNodeContainedGroupIds(
    store: CanvasGraphStore,
    nodeId: CanvasGraphNodeId,
    positions?: ReadonlyMap<CanvasGraphNodeId, CanvasPoint>,
  ): CanvasGraphGroupId[] {
    const nodeBounds = readNodeBoundsForMembership(store, nodeId, positions)
    if (!nodeBounds) return []
    return groupFrames.value
      .filter((frame) => rectContainsRect(frame.bounds, nodeBounds))
      .map((frame) => frame.id)
  }

  function isNodeIntersectingGroup(
    store: CanvasGraphStore,
    nodeId: CanvasGraphNodeId,
    groupId: CanvasGraphGroupId,
    positions?: ReadonlyMap<CanvasGraphNodeId, CanvasPoint>,
  ): boolean {
    const frame = groupFrames.value.find((item) => item.id === groupId)
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
    const selectedGroupIdBefore = selectedGroupId.value
    const selectedGroupBefore = selectedGroupIdBefore
      ? store.getGroup(selectedGroupIdBefore)
      : undefined
    const selectedGroupNodeIdsBefore = selectedGroupBefore
      ? new Set(selectedGroupBefore.nodeIds.filter(isCanvasGraphNodeId))
      : null
    try {
      stageNodeMembershipReconciliation(store, transaction, nodeIds, positions)
    } catch (error) {
      // Let the drag lifecycle rebase on concurrent graph commits. Other
      // membership failures remain non-fatal and surface through the warning
      // path below.
      if (error instanceof CanvasGraphStaleTransactionError) throw error
      options.onWarning(i18n.global.t('canvas.groupOps.updateFailed'))
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
      clearSelectedGroup()
      selectOnlyFlowNodesByGraphIds(movedOutNodeIds)
    }
  }

  function resolveRemovedFlowNodeChanges(
    changes: readonly NodeChange[],
  ): ResolvedFlowNodeChanges | null {
    const graphNodeIds: CanvasGraphNodeId[] = []
    const flowNodeIds: string[] = []
    for (const change of changes) {
      if (change.type !== 'remove') continue
      const graphNodeId = resolveGraphNodeIdForFlowNodeId(change.id)
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
        const nodeId = resolveGraphNodeIdForFlowNodeId(change.id)
        return !!nodeId && pendingNodeIds.has(nodeId)
      })
    ) {
      clearPendingGroupJoin()
    }
  }

  function resetTransientSelectionState() {
    cancelGroupDrag()
    cancelGroupResize()
    clearPendingGroupJoin()
    clearSelectedGroup()
  }

  function onNodeDragStart() {
    clearPendingGroupJoin()
  }

  function onBeforeNodesChange(changes: NodeChange[]) {
    clearPendingGroupJoinForNodeChanges(changes)
    maybeClearSelectedGroupForFlowSelection(changes)
  }

  function onBeforeEdgesChange(changes: EdgeChange[]) {
    maybeClearSelectedGroupForFlowSelection(changes)
  }

  return {
    view: {
      groupFrames,
      selectionGroupAction,
    },
    commands: {
      selectGroupAtPoint,
      handlePaneClick,
      handleNodeClick,
      groupSelectedNodes,
      ungroupSelectedGroup,
      deleteSelectedGroup,
      groupDrag: {
        begin: beginGroupDrag,
        move: moveGroupDrag,
        commit: commitGroupDrag,
        cancel: cancelGroupDrag,
      },
      groupResize: {
        begin: beginGroupResize,
        move: moveGroupResize,
        commit: commitGroupResize,
        cancel: cancelGroupResize,
      },
    },
    internal: {
      selectedGroupId,
      selectedGroupCoveredExternalNodeIds,
      projectCurrentGroupNodeLayers,
      projectCommittedGroupNodeLayer,
      handleCommittedGroupsChanged,
      resetTransientSelectionState,
      clearPendingGroupJoin,
      onNodeDragStart,
      onBeforeNodesChange,
      onBeforeEdgesChange,
      resolveRemovedFlowNodeChanges,
      applyDraggedNodeMembershipChanges: stageDraggedNodeMembershipChanges,
    },
  }
}
