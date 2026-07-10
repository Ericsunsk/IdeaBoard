import { computed, ref, type ComputedRef, type Ref } from 'vue'
import type { Edge, EdgeChange, Node, NodeChange } from '@vue-flow/core'
import type { CanvasBoardSnapshot } from '../../canvas-board/domain'
import {
  commitCanvasGraphDeleteGroupIntent,
  createCanvasGraphStore,
  isCanvasGraphNodeId,
  toCanvasGraphNodeId,
  type CanvasGraphGroupId,
  type CanvasGraphNodeId,
  type CanvasGraphTransaction,
} from '../graph-runtime'
import {
  canvasRectToGroupGeometry,
  findTopmostCanvasGroupFrameAtPoint,
  moveCanvasRect,
  projectCanvasGroupNodeLayers,
  readFlowNodeBounds,
  rectContainsRect,
  rectsIntersect,
  resizeCanvasRectFromHandle,
  resolveCanvasGroupBoundsFromNodes,
  resolveCanvasGroupFrames,
  type CanvasGroupFrame,
  type CanvasGroupResizeHandle,
  type CanvasPoint,
  type CanvasRect,
} from '../utils/canvasGroupGeometry'
import {
  resolveFlowNodeCanvasRef,
  type FlowToCanvasBoardSnapshotOptions,
} from '../utils/flowSnapshotAdapter'
import { createFlowGraphCommandOptions } from './flowGraphAdapterCommandSideEffects'
import { createFlowEdgeProjectionIndex } from './flowConnectionIdentity'
import { applyFlowGraphDeleteProjection } from './flowGraphDeleteProjection'
import type { CanvasVueFlowGraphAdapter } from './useCanvasVueFlowGraphAdapter'
import type { FlowHistorySnapshot } from './useFlowHistory'

type SelectableFlowItem = {
  id: string
  selected?: boolean
}

type ActiveGroupDrag = {
  groupId: CanvasGraphGroupId
  startPoint: CanvasPoint
  latestPoint: CanvasPoint
  historyBefore: FlowHistorySnapshot
  startPositions: Map<CanvasGraphNodeId, CanvasPoint>
  startBounds: CanvasRect
  didMove: boolean
}

type ActiveGroupResize = {
  groupId: CanvasGraphGroupId
  handle: CanvasGroupResizeHandle
  startPoint: CanvasPoint
  latestPoint: CanvasPoint
  historyBefore: FlowHistorySnapshot
  startBounds: CanvasRect
  didResize: boolean
}

type PendingGroupJoin = {
  groupId: CanvasGraphGroupId
  nodeIds: CanvasGraphNodeId[]
}

type FlowHistoryGroupRuntime = {
  capture: () => FlowHistorySnapshot
  push: (historyBefore: FlowHistorySnapshot) => boolean
}

export type CanvasSelectionGroupAction =
  | {
      kind: 'group'
      label: '组合'
      nodeIds: CanvasGraphNodeId[]
    }
  | {
      kind: 'ungroup'
      label: '解组'
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
  handleCommittedGroupsChanged: () => void
  resetTransientSelectionState: () => void
  clearPendingGroupJoin: () => boolean
  onNodeDragStart: () => void
  onBeforeNodesChange: (changes: NodeChange[]) => void
  onBeforeEdgesChange: (changes: EdgeChange[]) => void
  resolveRemovedFlowNodeChanges: (changes: readonly NodeChange[]) => ResolvedFlowNodeChanges | null
  applyDraggedNodeMembershipChanges: (changes: readonly NodeChange[]) => void
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
  flowHistory: FlowHistoryGroupRuntime
  createCurrentCanvasGraphStore: (
    snapshotOptions?: FlowToCanvasBoardSnapshotOptions,
  ) => ReturnType<typeof createCanvasGraphStore>
  commitGroups: (groups: CanvasBoardSnapshot['groups']) => void
  markDirty: () => void
  onWarning: (message: string) => void
  onNativeRemove?: () => void
  stopDeletedRunningTasks?: (taskIds: string[]) => void
}

export function useCanvasGroupRuntime(
  options: UseCanvasGroupRuntimeOptions,
): CanvasGroupRuntimeControls {
  let activeGroupDrag: ActiveGroupDrag | null = null
  let activeGroupResize: ActiveGroupResize | null = null
  const selectedGroupId = ref<CanvasGraphGroupId | null>(null)
  const pendingGroupJoin = ref<PendingGroupJoin | null>(null)
  const previewGroupBounds = ref<ReadonlyMap<CanvasGraphGroupId, CanvasRect>>(new Map())

  function createTransientGraphCommandOptions() {
    return createFlowGraphCommandOptions({
      recordHistory: false,
      markDirty: false,
    })
  }

  function projectCurrentGroupNodeLayers(nodes: readonly Node[]) {
    return projectCanvasGroupNodeLayers(nodes, options.boardBaseSnapshot.value.groups) as Node[]
  }

  function syncCurrentGroupNodeLayers() {
    const nextNodes = projectCurrentGroupNodeLayers(options.nodes.value)
    if (nextNodes === options.nodes.value) return
    options.graphAdapter.replaceNodes(nextNodes, createTransientGraphCommandOptions())
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
    syncCurrentGroupNodeLayers()
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
    options.graphAdapter.replaceGraph(
      {
        nodes: nextNodes,
        edges: nextEdges,
      },
      createTransientGraphCommandOptions(),
    )
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
    options.graphAdapter.replaceGraph(
      {
        nodes: nextNodes,
        edges: nextEdges,
      },
      createTransientGraphCommandOptions(),
    )
    return true
  }

  function resolveGraphNodeIdForFlowNode(node: Pick<Node, 'id' | 'data'>): CanvasGraphNodeId | null {
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
    const groupId = selectedGroupId.value
    if (!groupId) return []
    const frame = groupFrames.value.find((item) => item.id === groupId)
    if (!frame) return []

    const coveredNodeIds: CanvasGraphNodeId[] = []
    options.nodes.value.forEach((node) => {
      const nodeId = resolveGraphNodeIdForFlowNodeId(node.id)
      if (!nodeId || readGroupIdForNode(nodeId) !== null) return
      if (rectsIntersect(frame.bounds, readFlowNodeBounds(node))) coveredNodeIds.push(nodeId)
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
    if (!readGroup(pending.groupId)) return clearPendingGroupJoin()
    const validNodeIds = pending.nodeIds.filter(
      (nodeId) =>
        readGroupIdForNode(nodeId) === null &&
        options.nodes.value.some((node) => resolveGraphNodeIdForFlowNodeId(node.id) === nodeId),
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

  function commitGroupsFromStore(
    store: ReturnType<UseCanvasGroupRuntimeOptions['createCurrentCanvasGraphStore']>,
  ) {
    const snapshot = store.exportSnapshot(options.boardBaseSnapshot.value)
    options.commitGroups(snapshot.groups)
  }

  function joinPendingGroupNode(flowNode: Pick<Node, 'id' | 'data'>): boolean {
    const pending = pendingGroupJoin.value
    if (!pending) return false
    const nodeId = resolveGraphNodeIdForFlowNode(flowNode)
    clearPendingGroupJoin()
    if (!nodeId || !pending.nodeIds.includes(nodeId)) return false
    if (!readGroup(pending.groupId) || readGroupIdForNode(nodeId) !== null) return false

    const historyBefore = options.flowHistory.capture()
    const store = options.createCurrentCanvasGraphStore()
    const transaction = store.beginTransaction('group-membership')
    try {
      const summary = transaction.moveNodeToGroup(nodeId, pending.groupId).commit()
      if (!summary.didChange) return false
      commitGroupsFromStore(store)
      selectOnlyFlowNodesByGraphIds([nodeId])
      if (options.flowHistory.push(historyBefore)) options.markDirty()
      return true
    } catch (_error) {
      transaction.cancel()
      options.onWarning('组合关系更新失败，请刷新画布后重试')
      return false
    }
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
        label: '解组',
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
        label: '解组',
        groupId: matchingGroups[0].id,
        nodeIds: selectedNodeIds,
      }
    }
    if (matchingGroups.length > 1) return null

    const groupedNodeIds = readGroupedNodeIds()
    if (selectedNodeIds.some((nodeId) => groupedNodeIds.has(nodeId))) return null

    return {
      kind: 'group',
      label: '组合',
      nodeIds: selectedNodeIds,
    }
  }

  const selectionGroupAction = computed(resolveSelectedGroupAction)

  function updateGroupGeometryInSnapshot(groupId: CanvasGraphGroupId, bounds: CanvasRect): boolean {
    const geometry = canvasRectToGroupGeometry(bounds)
    let didChange = false
    const groups = options.boardBaseSnapshot.value.groups.map((group) => {
      if (group.id !== groupId) return group
      if (
        group.x === geometry.x &&
        group.y === geometry.y &&
        group.width === geometry.width &&
        group.height === geometry.height
      ) {
        return group
      }
      didChange = true
      return {
        ...group,
        ...geometry,
      }
    })
    if (!didChange) return false
    options.commitGroups(groups)
    return true
  }

  function groupSelectedNodes(): boolean {
    clearPendingGroupJoin()
    const action = selectionGroupAction.value
    if (action?.kind !== 'group') return false
    const bounds = resolveCanvasGroupBoundsFromNodes({
      nodes: options.nodes.value,
      nodeIds: action.nodeIds,
    })
    if (!bounds) {
      options.onWarning('组合失败，请刷新画布后重试')
      return false
    }

    const historyBefore = options.flowHistory.capture()
    const store = options.createCurrentCanvasGraphStore()
    const transaction = store.beginTransaction('group')
    let summary: ReturnType<CanvasGraphTransaction['commit']>
    try {
      summary = transaction.createGroup(action.nodeIds, canvasRectToGroupGeometry(bounds)).commit()
    } catch (_error) {
      transaction.cancel()
      options.onWarning('组合失败，请刷新画布后重试')
      return false
    }
    if (!summary.didChange) return false
    const createdGroupId =
      summary.projectionPatch.groups.added[0]?.id ?? summary.changedGroupIds[0] ?? null
    commitGroupsFromStore(store)
    clearFlowSelection()
    if (createdGroupId) selectedGroupId.value = createdGroupId
    if (options.flowHistory.push(historyBefore)) options.markDirty()
    return true
  }

  function ungroupSelectedGroup(): boolean {
    clearPendingGroupJoin()
    const action = selectionGroupAction.value
    if (action?.kind !== 'ungroup') return false

    const historyBefore = options.flowHistory.capture()
    const store = options.createCurrentCanvasGraphStore()
    const transaction = store.beginTransaction('ungroup')
    let didChange: boolean
    try {
      didChange = transaction.ungroup(action.groupId).commit().didChange
    } catch (_error) {
      transaction.cancel()
      options.onWarning('解组失败，请刷新画布后重试')
      return false
    }
    if (!didChange) return false
    commitGroupsFromStore(store)
    if (selectedGroupId.value === action.groupId) clearSelectedGroup()
    if (options.flowHistory.push(historyBefore)) options.markDirty()
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
    const historyBefore = options.flowHistory.capture()
    const store = options.createCurrentCanvasGraphStore()
    const projectionIndex = createFlowEdgeProjectionIndex(store, options.edges.value)
    const shouldDeleteMembers = nodeIds.length > 0
    const summary = commitCanvasGraphDeleteGroupIntent(store, {
      groupId: group.id as CanvasGraphGroupId,
      deleteMembers: shouldDeleteMembers,
      removeConnectedEdges: true,
    })
    if (!summary.didChange) return false

    commitGroupsFromStore(store)
    if (!shouldDeleteMembers) {
      if (options.flowHistory.push(historyBefore)) options.markDirty()
      return true
    }

    const result = applyFlowGraphDeleteProjection({
      graphAdapter: options.graphAdapter,
      nodes: options.nodes.value,
      edges: options.edges.value,
      commitPolicy: { summary, historyBefore },
      projectionIndex,
      onNativeRemove: options.onNativeRemove,
      stopDeletedRunningTasks: options.stopDeletedRunningTasks,
    })
    return result?.didChange ?? false
  }

  function readGroupMemberNodePositions(groupId: CanvasGraphGroupId) {
    const group = readGroup(groupId)
    if (!group) return null
    const positions = new Map<CanvasGraphNodeId, CanvasPoint>()
    group.nodeIds.forEach((nodeId) => {
      if (!isCanvasGraphNodeId(nodeId)) return
      const node = options.nodes.value.find((item) => item.id === nodeId)
      if (!node) return
      positions.set(nodeId, { x: node.position.x, y: node.position.y })
    })
    return positions
  }

  function beginGroupDrag(point: CanvasPoint): boolean {
    clearPendingGroupJoin()
    const frame = findTopmostCanvasGroupFrameAtPoint(groupFrames.value, point)
    if (!frame) return false
    const startPositions = readGroupMemberNodePositions(frame.id)
    if (!startPositions) return false
    selectGroup(frame.id)
    activeGroupDrag = {
      groupId: frame.id,
      startPoint: point,
      latestPoint: point,
      historyBefore: options.flowHistory.capture(),
      startPositions,
      startBounds: frame.bounds,
      didMove: false,
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
    drag.didMove = true
    if (drag.startPositions.size > 0) {
      options.graphAdapter.applyNodeChanges(
        createPositionChanges(drag.startPositions, delta, true),
        createTransientGraphCommandOptions(),
      )
    }
    updateGroupGeometryInSnapshot(drag.groupId, moveCanvasRect(drag.startBounds, delta))
    return true
  }

  function commitGroupDrag(): boolean {
    const drag = activeGroupDrag
    if (!drag) return false
    activeGroupDrag = null
    if (!drag.didMove) return false
    const delta = {
      x: drag.latestPoint.x - drag.startPoint.x,
      y: drag.latestPoint.y - drag.startPoint.y,
    }
    if (drag.startPositions.size > 0) {
      options.graphAdapter.applyNodeChanges(
        createPositionChanges(drag.startPositions, delta, false),
        createTransientGraphCommandOptions(),
      )
    }
    updateGroupGeometryInSnapshot(drag.groupId, moveCanvasRect(drag.startBounds, delta))
    if (options.flowHistory.push(drag.historyBefore)) options.markDirty()
    return true
  }

  function cancelGroupDrag(): boolean {
    const drag = activeGroupDrag
    if (!drag) return false
    activeGroupDrag = null
    options.graphAdapter.applyNodeChanges(
      createPositionChanges(drag.startPositions, { x: 0, y: 0 }, false),
      createTransientGraphCommandOptions(),
    )
    updateGroupGeometryInSnapshot(drag.groupId, drag.startBounds)
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
    clearPendingGroupJoin()
    const frame = readGroupFrame(groupId)
    if (!frame) return false
    selectGroup(groupId)
    activeGroupResize = {
      groupId,
      handle,
      startPoint: point,
      latestPoint: point,
      historyBefore: options.flowHistory.capture(),
      startBounds: frame.bounds,
      didResize: false,
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
    resize.didResize =
      resize.didResize ||
      nextBounds.left !== resize.startBounds.left ||
      nextBounds.top !== resize.startBounds.top ||
      nextBounds.width !== resize.startBounds.width ||
      nextBounds.height !== resize.startBounds.height
    setPreviewGroupBounds(resize.groupId, nextBounds)
    return true
  }

  function commitGroupResize(): boolean {
    const resize = activeGroupResize
    if (!resize) return false
    activeGroupResize = null
    if (!resize.didResize) {
      clearPreviewGroupBounds(resize.groupId)
      return false
    }
    const delta = {
      x: resize.latestPoint.x - resize.startPoint.x,
      y: resize.latestPoint.y - resize.startPoint.y,
    }
    const nextBounds = resizeCanvasRectFromHandle({
      bounds: resize.startBounds,
      handle: resize.handle,
      delta,
    })
    const store = options.createCurrentCanvasGraphStore()
    const transaction = store.beginTransaction('group-resize')
    try {
      transaction.updateGroupGeometry(resize.groupId, canvasRectToGroupGeometry(nextBounds))
      readGroupMemberExitsForBounds(resize.groupId, nextBounds).forEach((nodeId) => {
        transaction.moveNodeToGroup(nodeId, null)
      })
      const summary = transaction.commit()
      clearPreviewGroupBounds(resize.groupId)
      if (!summary.didChange) return false
      commitGroupsFromStore(store)
      if (options.flowHistory.push(resize.historyBefore)) options.markDirty()
      return true
    } catch (_error) {
      transaction.cancel()
      clearPreviewGroupBounds(resize.groupId)
      options.onWarning('组合边界更新失败，请刷新画布后重试')
      return false
    }
  }

  function cancelGroupResize(): boolean {
    const resize = activeGroupResize
    if (!resize) return false
    activeGroupResize = null
    clearPreviewGroupBounds(resize.groupId)
    return true
  }

  function readGroupIdForNode(nodeId: CanvasGraphNodeId): CanvasGraphGroupId | null {
    for (const group of options.boardBaseSnapshot.value.groups) {
      if (group.nodeIds.includes(nodeId)) return group.id as CanvasGraphGroupId
    }
    return null
  }

  function resolveNodeContainedGroupIds(nodeId: CanvasGraphNodeId): CanvasGraphGroupId[] {
    const node = options.nodes.value.find((item) => item.id === nodeId)
    if (!node) return []
    const nodeBounds = readFlowNodeBounds(node)
    return groupFrames.value
      .filter((frame) => rectContainsRect(frame.bounds, nodeBounds))
      .map((frame) => frame.id)
  }

  function isNodeIntersectingGroup(
    nodeId: CanvasGraphNodeId,
    groupId: CanvasGraphGroupId,
  ): boolean {
    const node = options.nodes.value.find((item) => item.id === nodeId)
    const frame = groupFrames.value.find((item) => item.id === groupId)
    if (!node || !frame) return false
    return rectsIntersect(frame.bounds, readFlowNodeBounds(node))
  }

  function resolveNodeMembershipTarget(
    nodeId: CanvasGraphNodeId,
  ): CanvasGraphGroupId | null | undefined {
    const currentGroupId = readGroupIdForNode(nodeId)
    const containingGroupIds = resolveNodeContainedGroupIds(nodeId)
    if (currentGroupId) {
      if (isNodeIntersectingGroup(nodeId, currentGroupId)) return undefined
      return containingGroupIds.length === 1 ? containingGroupIds[0] : null
    }

    if (containingGroupIds.length !== 1) return undefined
    return containingGroupIds[0]
  }

  function applyNodeMembershipReconciliation(nodeIds: readonly CanvasGraphNodeId[]): boolean {
    if (nodeIds.length === 0) return false
    const store = options.createCurrentCanvasGraphStore()
    const transaction = store.beginTransaction('group-membership')
    try {
      nodeIds.forEach((nodeId) => {
        const targetGroupId = resolveNodeMembershipTarget(nodeId)
        if (targetGroupId !== undefined) {
          transaction.moveNodeToGroup(nodeId, targetGroupId)
        }
      })
      const summary = transaction.commit()
      if (summary.didChange) {
        commitGroupsFromStore(store)
        return true
      }
    } catch (_error) {
      transaction.cancel()
      options.onWarning('组合关系更新失败，请刷新画布后重试')
    }
    return false
  }

  function readGroupMemberExitsForBounds(
    groupId: CanvasGraphGroupId,
    bounds: CanvasRect,
  ): CanvasGraphNodeId[] {
    const group = readGroup(groupId)
    if (!group) return []
    return group.nodeIds
      .filter(isCanvasGraphNodeId)
      .filter((nodeId) => {
        const node = options.nodes.value.find((item) => item.id === nodeId)
        return !node || !rectsIntersect(bounds, readFlowNodeBounds(node))
      })
  }

  function applyDraggedNodeMembershipChanges(nodeIds: readonly CanvasGraphNodeId[]) {
    const selectedGroupIdBefore = selectedGroupId.value
    const selectedGroupBefore = readGroup(selectedGroupIdBefore)
    const selectedGroupNodeIdsBefore = selectedGroupBefore
      ? new Set(selectedGroupBefore.nodeIds.filter(isCanvasGraphNodeId))
      : null
    const didChange = applyNodeMembershipReconciliation(nodeIds)
    if (!didChange || !selectedGroupIdBefore || !selectedGroupNodeIdsBefore) return

    const selectedGroupAfter = readGroup(selectedGroupIdBefore)
    const selectedGroupNodeIdsAfter = new Set(
      selectedGroupAfter?.nodeIds.filter(isCanvasGraphNodeId) ?? [],
    )
    const movedOutNodeIds = nodeIds.filter(
      (nodeId) => selectedGroupNodeIdsBefore.has(nodeId) && !selectedGroupNodeIdsAfter.has(nodeId),
    )
    if (movedOutNodeIds.length === 0) return

    clearSelectedGroup()
    selectOnlyFlowNodesByGraphIds(movedOutNodeIds)
  }

  function readDraggedGraphNodeIds(changes: readonly NodeChange[]): CanvasGraphNodeId[] {
    const nodeIds = new Set<CanvasGraphNodeId>()
    changes.forEach((change) => {
      if (change.type !== 'position') return
      const nodeId = resolveGraphNodeIdForFlowNodeId(change.id)
      if (nodeId) nodeIds.add(nodeId)
    })
    return Array.from(nodeIds)
  }

  function applyDraggedFlowNodeMembershipChanges(changes: readonly NodeChange[]) {
    applyDraggedNodeMembershipChanges(readDraggedGraphNodeIds(changes))
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
      handleCommittedGroupsChanged,
      resetTransientSelectionState,
      clearPendingGroupJoin,
      onNodeDragStart,
      onBeforeNodesChange,
      onBeforeEdgesChange,
      resolveRemovedFlowNodeChanges,
      applyDraggedNodeMembershipChanges: applyDraggedFlowNodeMembershipChanges,
    },
  }
}
