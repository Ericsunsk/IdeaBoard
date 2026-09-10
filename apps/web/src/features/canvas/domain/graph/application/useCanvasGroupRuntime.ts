import { computed, ref, type ComputedRef, type Ref } from 'vue'
import { i18n } from '../../../../../shared/i18n'
import type { Edge, EdgeChange, Node, NodeChange } from '@vue-flow/core'
import type { BoardSnapshot, CanvasGroup } from '../../../../board/domain'
import type { PendingImageExecutionLease } from '../../../application/execution/pending-execution'
import {
  commitCanvasGraphDeleteGroupIntent,
  isCanvasGraphNodeId,
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
  projectCanvasGroupNodeLayers,
  resolveCanvasGroupFrames,
  type CanvasGroupFrame,
  type CanvasPoint,
  type CanvasRect,
} from '../canvasGroupGeometry.ts'
import { resolveFlowNodeCanvasRef } from '../../../infrastructure/vue-flow-adapter/flowSnapshotAdapter.ts'
import { releaseCanvasGraphDeleteExecutions } from '../../../infrastructure/vue-flow-adapter/flowGraphDeleteProjection'
import type { CanvasVueFlowGraphAdapter } from '../../../infrastructure/vue-flow-adapter/useCanvasVueFlowGraphAdapter'
import { createGroupDragRuntime } from './groupDragRuntime'
import { createGroupResizeRuntime, type GroupResizeRuntime } from './groupResizeRuntime'
import {
  createGroupMembershipRuntime,
  type ResolvedFlowNodeChanges,
} from './groupMembershipRuntime'

type SelectableFlowItem = {
  id: string
  selected?: boolean
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
  groupResize: GroupResizeRuntime
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
  boardBaseSnapshot: Ref<BoardSnapshot>
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
  const selectedGroupId = ref<CanvasGraphGroupId | null>(null)
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

  function readGroup(groupId: CanvasGraphGroupId | null): CanvasGroup | null {
    if (!groupId) return null
    return options.boardBaseSnapshot.value.groups.find((group) => group.id === groupId) ?? null
  }

  function selectGroup(groupId: CanvasGraphGroupId): boolean {
    if (!readGroup(groupId)) return false
    if (selectedGroupId.value !== groupId) membership.clearPendingGroupJoin()
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

  const membership = createGroupMembershipRuntime({
    readCanvasGraphStore: options.readCanvasGraphStore,
    isGraphSessionAttached: options.isGraphSessionAttached,
    selectedGroupId,
    groupFrames,
    readGroup,
    resolveGraphNodeIdForFlowNode,
    resolveGraphNodeIdForFlowNodeId,
    selectOnlyFlowNodesByGraphIds,
    clearSelectedGroup,
    onWarning: options.onWarning,
    updateFailedMessage: i18n.global.t('canvas.groupOps.updateFailed'),
  })

  function handleCommittedGroupsChanged() {
    pruneSelectedGroup()
    membership.prunePendingGroupJoin()
  }

  function handlePaneClick(selectPane: () => void) {
    membership.capturePendingGroupJoinFromSelectedGroup()
    selectPane()
    clearSelectedGroup()
  }

  function handleNodeClick(payload: CanvasGroupNodeClickPayload) {
    membership.joinPendingGroupNode(payload.node)
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
      const matchedGroup = matchingGroups[0]
      if (!matchedGroup) return null
      return {
        kind: 'ungroup',
        label: i18n.global.t('canvas.controls.ungroupSelection'),
        groupId: matchedGroup.id,
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
    membership.clearPendingGroupJoin()
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
    membership.clearPendingGroupJoin()
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

  function deleteSelectedGroup(): boolean {
    membership.clearPendingGroupJoin()
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

  const { groupResize, groupDrag } = createGroupTransformRuntime(options, {
    groupFrames,
    clearPendingGroupJoin: membership.clearPendingGroupJoin,
    selectGroup,
    setPreviewGroupBounds,
    clearPreviewGroupBounds,
  })

  function resetTransientSelectionState() {
    groupDrag.cancel()
    groupResize.cancel()
    membership.clearPendingGroupJoin()
    clearSelectedGroup()
  }

  function onNodeDragStart() {
    membership.clearPendingGroupJoin()
  }

  function onBeforeNodesChange(changes: NodeChange[]) {
    membership.clearPendingGroupJoinForNodeChanges(changes)
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
      groupDrag,
      groupResize,
    },
    internal: {
      selectedGroupId,
      selectedGroupCoveredExternalNodeIds: membership.selectedGroupCoveredExternalNodeIds,
      projectCurrentGroupNodeLayers,
      projectCommittedGroupNodeLayer,
      handleCommittedGroupsChanged,
      resetTransientSelectionState,
      clearPendingGroupJoin: membership.clearPendingGroupJoin,
      onNodeDragStart,
      onBeforeNodesChange,
      onBeforeEdgesChange,
      resolveRemovedFlowNodeChanges: membership.resolveRemovedFlowNodeChanges,
      applyDraggedNodeMembershipChanges: membership.applyDraggedNodeMembershipChanges,
    },
  }
}

function createGroupTransformRuntime(
  options: UseCanvasGroupRuntimeOptions,
  bindings: Pick<
    Parameters<typeof createGroupDragRuntime>[0],
    | 'groupFrames'
    | 'clearPendingGroupJoin'
    | 'selectGroup'
    | 'setPreviewGroupBounds'
    | 'clearPreviewGroupBounds'
  >,
) {
  const {
    groupFrames,
    clearPendingGroupJoin,
    selectGroup,
    setPreviewGroupBounds,
    clearPreviewGroupBounds,
  } = bindings

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

  const groupResize = createGroupResizeRuntime({
    groupFrames,
    readCanvasGraphStore: options.readCanvasGraphStore,
    isDragActive: () => groupDrag.isActive(),
    clearPendingGroupJoin,
    selectGroup,
    setPreviewGroupBounds,
    clearPreviewGroupBounds,
    onWarning: options.onWarning,
    boundsUpdateFailedMessage: i18n.global.t('canvas.groupOps.boundsUpdateFailed'),
  })

  const groupDrag = createGroupDragRuntime({
    groupFrames,
    graphAdapter: options.graphAdapter,
    readCanvasGraphStore: options.readCanvasGraphStore,
    clearPendingGroupJoin,
    selectGroup,
    readGroupMemberNodePositions,
    createPositionChanges,
    setPreviewGroupBounds,
    clearPreviewGroupBounds,
    findTopmostGroupFrame: (point) => findTopmostCanvasGroupFrameAtPoint(groupFrames.value, point),
    onWarning: options.onWarning,
    moveFailedMessage: i18n.global.t('canvas.groupOps.moveFailed'),
    isResizeActive: () => groupResize.isActive(),
  })

  return { groupResize, groupDrag }
}
