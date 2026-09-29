import { computed, ref, type ComputedRef, type Ref } from 'vue'
import { i18n } from '../../../../shared/i18n'
import type { Edge, EdgeChange, Node, NodeChange } from '@vue-flow/core'
import type { BoardSnapshot, CanvasGroup } from '../../../board/ports/canvas'
import type { PendingImageExecutionLease } from '../execution/pending-execution'
import {
  commitCanvasGraphDeleteGroupIntent,
  isCanvasGraphNodeId,
  resolveCanvasGraphNodesBounds,
  toCanvasGraphNodeId,
  type CanvasGraphGroupId,
  type CanvasGraphNodeId,
  type CanvasGraphStore,
  type CanvasGraphTransaction,
} from '../../domain/graph'
import {
  CANVAS_GROUP_FRAME_PADDING_PX,
  canvasRectToGroupGeometry,
  findTopmostCanvasGroupFrameAtPoint,
  projectCanvasGroupNodeLayers,
  resolveCanvasGroupFrames,
  type CanvasGroupFrame,
  type CanvasPoint,
  type CanvasRect,
} from '../../domain/graph/canvasGroupGeometry.ts'
import { resolveFlowNodeCanvasRef } from '../../infrastructure/vue-flow-adapter/flowSnapshotAdapter.ts'
import { releaseCanvasGraphDeleteExecutions } from '../../infrastructure/vue-flow-adapter/flowGraphDeleteProjection'
import type { CanvasVueFlowGraphAdapter } from '../../infrastructure/vue-flow-adapter/useCanvasVueFlowGraphAdapter'
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

export type CanvasGroupRuntimeCommands = {
  selectGroupAtPoint: (point: CanvasPoint) => boolean
  handlePaneClick: () => void
  handleNodeClick: (nodeId: string) => boolean
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
  // Single-slot preview for the group currently being dragged/resized.
  // Drag and resize are mutually exclusive (each begin() bails while the
  // other is active), so one slot covers every pointermove path without
  // cloning a whole Map per event.
  const activeGroupPreview = ref<{ groupId: CanvasGraphGroupId; bounds: CanvasRect } | null>(null)

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

  function commitSelectionTransaction(
    stage: (store: CanvasGraphStore, transaction: CanvasGraphTransaction) => void,
  ): boolean {
    let transaction: CanvasGraphTransaction | undefined
    try {
      const store = options.readCanvasGraphStore()
      transaction = store.beginTransaction('node-update', {
        recordHistory: false,
        markDirty: false,
      })
      stage(store, transaction)
      return transaction.commit().didChange
    } catch (error) {
      transaction?.cancel()
      console.error('[Canvas] Failed to commit selection transaction', error)
      options.onWarning(i18n.global.t('canvas.groupOps.updateFailed'))
      return false
    }
  }

  function clearFlowSelection(): boolean {
    if (!options.isGraphSessionAttached()) return false
    return commitSelectionTransaction((store, transaction) => {
      store.getNodeIds().forEach((nodeId) => transaction.selectNode(nodeId, false))
      store.getEdgeIds().forEach((edgeId) => transaction.selectEdge(edgeId, false))
    })
  }

  function selectOnlyFlowNodesByGraphIds(nodeIds: readonly CanvasGraphNodeId[]): boolean {
    const selectedNodeIds = new Set(nodeIds)
    if (!options.isGraphSessionAttached()) return false
    return commitSelectionTransaction((store, transaction) => {
      store
        .getNodeIds()
        .forEach((nodeId) => transaction.selectNode(nodeId, selectedNodeIds.has(nodeId)))
      store.getEdgeIds().forEach((edgeId) => transaction.selectEdge(edgeId, false))
    })
  }

  function resolveGraphNodeIdForFlowNodeId(flowNodeId: string): CanvasGraphNodeId | null {
    const node = options.nodes.value.find((item) => item.id === flowNodeId)
    const ref = resolveFlowNodeCanvasRef(node ?? { id: flowNodeId, data: undefined })
    return ref ? toCanvasGraphNodeId(ref.nodeType, ref.id) : null
  }

  function hasSelectedEdges() {
    if (!options.isGraphSessionAttached()) {
      return options.edges.value.some((edge) => (edge as SelectableFlowItem).selected === true)
    }
    const store = options.readCanvasGraphStore()
    return store.getEdgeIds().some((edgeId) => store.getEdge(edgeId)?.selected === true)
  }

  function readSelectedGraphNodeIds(): CanvasGraphNodeId[] | null {
    // Selection intent always flows through the store while a session is
    // attached (native changes via reconcile, group intents via selection
    // transactions), so group actions read the canonical fact instead of the
    // Flow mirror. Detached (between board attachments) there is no store to
    // read, so fall back to the Flow selection. A selection that includes
    // non-canonical renderer ids still blocks the action, as before.
    const selectedFlowNodeIds = options.nodes.value
      .filter((node) => (node as SelectableFlowItem).selected === true)
      .map((node) => node.id)
    if (!options.isGraphSessionAttached()) {
      if (selectedFlowNodeIds.length === 0) return []
      const selectedGraphNodeIds = selectedFlowNodeIds
        .map((nodeId) => resolveGraphNodeIdForFlowNodeId(nodeId))
        .filter((nodeId): nodeId is CanvasGraphNodeId => nodeId !== null)
      return selectedGraphNodeIds.length === selectedFlowNodeIds.length
        ? selectedGraphNodeIds
        : null
    }
    if (selectedFlowNodeIds.some((nodeId) => !isCanvasGraphNodeId(nodeId))) return null
    const store = options.readCanvasGraphStore()
    return store.getNodeIds().filter((nodeId) => store.getNode(nodeId)?.selected === true)
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
      const preview = activeGroupPreview.value
      return preview && preview.groupId === frame.id ? { ...frame, bounds: preview.bounds } : frame
    }),
  )

  function setPreviewGroupBounds(groupId: CanvasGraphGroupId, bounds: CanvasRect) {
    const preview = activeGroupPreview.value
    if (
      preview &&
      preview.groupId === groupId &&
      preview.bounds.left === bounds.left &&
      preview.bounds.top === bounds.top &&
      preview.bounds.right === bounds.right &&
      preview.bounds.bottom === bounds.bottom &&
      preview.bounds.width === bounds.width &&
      preview.bounds.height === bounds.height
    ) {
      return
    }
    activeGroupPreview.value = { groupId, bounds }
  }

  function clearPreviewGroupBounds(groupId: CanvasGraphGroupId) {
    if (activeGroupPreview.value?.groupId !== groupId) return
    activeGroupPreview.value = null
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

  function handlePaneClick() {
    membership.capturePendingGroupJoinFromSelectedGroup()
    clearSelectedGroup()
  }

  function handleNodeClick(nodeId: string): boolean {
    return membership.joinPendingGroupNode(nodeId)
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
      // Selection rides the same transaction so one user action produces one
      // commit: undoing the grouping also restores the selection it replaced.
      transaction.createGroup(action.nodeIds, canvasRectToGroupGeometry(bounds))
      action.nodeIds.forEach((nodeId) => transaction.selectNode(nodeId, false))
      store.getEdgeIds().forEach((edgeId) => transaction.selectEdge(edgeId, false))
      summary = transaction.commit()
    } catch (_error) {
      transaction.cancel()
      options.onWarning(i18n.global.t('canvas.groupOps.groupFailed'))
      return false
    }
    if (!summary.didChange) return false
    const createdGroupId =
      summary.projectionPatch.groups.added[0]?.id ?? summary.changedGroupIds[0] ?? null
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
