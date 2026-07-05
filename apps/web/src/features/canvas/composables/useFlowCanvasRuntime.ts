import type { Ref } from 'vue'
import type { Edge, EdgeChange, Node, NodeChange } from '@vue-flow/core'
import { useFlowHistory, type FlowHistorySnapshot } from './useFlowHistory'
import { useFlowBoardPersistence } from './useFlowBoardPersistence'
import { useFlowAutosave } from './useFlowAutosave'
import {
  useCanvasVueFlowGraphAdapter,
  type CanvasVueFlowGraphActions,
  type CanvasVueFlowGraphAdapter,
  type CanvasVueFlowGraphCommandContext,
  type CanvasVueFlowGraphCommandResult,
} from './useCanvasVueFlowGraphAdapter'
import {
  createFlowGraphCommandOptions,
  readFlowGraphCommandSideEffects,
} from './flowGraphAdapterCommandSideEffects'
import type { CanvasBoardSnapshot } from '../../canvas-board/domain'
import type { CanvasBoardSession } from '../board-session'

type SaveStatus = 'idle' | 'saved' | 'failed'

type ViewportState = {
  x: number
  y: number
  zoom: number
}

type SelectableFlowItem = {
  id: string
  selected?: boolean
}

type FlowGraphCommitOptions = {
  preserveSelection?: boolean
}

export type CanvasFlowGraphRuntimeControls = {
  captureHistory: () => FlowHistorySnapshot
  markDirty: () => void
  nodeDrag: {
    begin: () => void
    commit: () => void
  }
}

export type UseFlowCanvasRuntimeOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  boardBaseSnapshot: Ref<CanvasBoardSnapshot>
  isDirty: Ref<boolean>
  saveStatus: Ref<SaveStatus>
  isLoadingBoardSnapshot: Ref<boolean>
  isSavingBoard: Ref<boolean>
  boardSession: CanvasBoardSession
  getViewport: () => ViewportState
  setViewport: (viewport: ViewportState) => void
  onWarning: (message: string) => void
  onSuccess: (message: string) => void
  onError: (message: string) => void
  onNativeRemove?: () => void
  scheduleMicrotask?: (callback: () => void) => void
  vueFlowGraphActions?: CanvasVueFlowGraphActions
}

export function useFlowCanvasRuntime(options: UseFlowCanvasRuntimeOptions) {
  let isNodeDragActive = false
  let nativeRemoveHistoryBefore: FlowHistorySnapshot | null = null
  let isNativeRemoveCommitScheduled = false
  let pendingNativeRemoveNodeChanges: NodeChange[] = []
  let pendingNativeRemoveEdgeChanges: EdgeChange[] = []
  const scheduleMicrotask =
    options.scheduleMicrotask ?? ((callback: () => void) => void Promise.resolve().then(callback))

  function collectSelectedItemIds(...itemGroups: SelectableFlowItem[][]): Set<string> {
    const ids = new Set<string>()
    for (const items of itemGroups) {
      for (const item of items) {
        if (item.selected === true) ids.add(item.id)
      }
    }
    return ids
  }

  function preserveSelectedItems<T extends SelectableFlowItem>(
    nextItems: T[],
    selectedIds: Set<string>,
  ): T[] {
    let resolvedItems: T[] | null = null
    nextItems.forEach((item, index) => {
      const shouldSelect = selectedIds.has(item.id)
      if (item.selected === shouldSelect) return
      if (!resolvedItems) resolvedItems = [...nextItems]
      resolvedItems[index] = { ...item, selected: shouldSelect }
    })
    return resolvedItems ?? nextItems
  }

  function preserveCurrentSelection(nextGraph: { nodes: Node[]; edges: Edge[] }) {
    const selectedNodeIds = collectSelectedItemIds(options.nodes.value)
    const selectedEdgeIds = collectSelectedItemIds(options.edges.value)
    return {
      nodes: preserveSelectedItems(nextGraph.nodes, selectedNodeIds),
      edges: preserveSelectedItems(nextGraph.edges, selectedEdgeIds),
    }
  }

  function captureGraphCommandHistory(context: CanvasVueFlowGraphCommandContext) {
    const sideEffects = readFlowGraphCommandSideEffects(context.metadata)
    if (!sideEffects || sideEffects.recordHistory === false) return null
    return sideEffects.historyBefore ?? flowHistory.capture()
  }

  function commitGraphCommandSideEffects(
    result: CanvasVueFlowGraphCommandResult,
    historyBefore: unknown,
  ) {
    const sideEffects = readFlowGraphCommandSideEffects(result.metadata)
    if (!sideEffects) return
    let didRecordChange = result.didChange
    if (historyBefore) {
      didRecordChange = flowHistory.push(historyBefore as FlowHistorySnapshot)
    }
    if (sideEffects.markDirty !== false && didRecordChange) markDirty()
  }

  const graphAdapter: CanvasVueFlowGraphAdapter = useCanvasVueFlowGraphAdapter({
    nodes: options.nodes,
    edges: options.edges,
    actions: options.vueFlowGraphActions,
    beforeCommand: captureGraphCommandHistory,
    afterCommand: commitGraphCommandSideEffects,
  })

  function commitFlowGraph(
    nextGraph: { nodes: Node[]; edges: Edge[] },
    commitOptions: FlowGraphCommitOptions = {},
  ) {
    const resolvedGraph = commitOptions.preserveSelection
      ? preserveCurrentSelection(nextGraph)
      : nextGraph
    graphAdapter.replaceGraph(resolvedGraph)
  }

  function captureFullGraphHistory() {
    return flowHistory.capture()
  }

  function createRestoreGraphCommandOptions() {
    return createFlowGraphCommandOptions({
      recordHistory: false,
      markDirty: false,
    })
  }

  const flowHistory = useFlowHistory({
    nodes: options.nodes,
    edges: options.edges,
    maxSteps: 10,
    onRestore: () => {
      markDirty()
    },
    commitNodes: (nodes) => graphAdapter.replaceNodes(nodes as Node[], createRestoreGraphCommandOptions()),
    commitEdges: (edges) => graphAdapter.replaceEdges(edges as Edge[], createRestoreGraphCommandOptions()),
  })

  const boardPersistence = useFlowBoardPersistence({
    fullNodes: options.nodes,
    fullEdges: options.edges,
    boardBaseSnapshot: options.boardBaseSnapshot,
    isDirty: options.isDirty,
    saveStatus: options.saveStatus,
    isLoadingBoardSnapshot: options.isLoadingBoardSnapshot,
    isSavingBoard: options.isSavingBoard,
    boardSession: options.boardSession,
    onWarning: options.onWarning,
    onSuccess: options.onSuccess,
    onError: options.onError,
    clearFlowHistoryBaseline: flowHistory.clearBaseline,
    getViewport: options.getViewport,
    setViewport: options.setViewport,
    commitGraph: (graph, commitOptions) =>
      commitFlowGraph(
        {
          nodes: graph.nodes as Node[],
          edges: graph.edges as Edge[],
        },
        commitOptions,
      ),
  })

  const currentBoardId = boardPersistence.currentBoardId
  const loadInitialOrRouteBoard = boardPersistence.loadInitialOrRouteBoard
  const saveCurrentBoardSnapshot = boardPersistence.saveCurrentBoardSnapshot
  const canSaveBoard = boardPersistence.canSaveBoard

  const { markDirtyAndSchedule, flushAutosave } = useFlowAutosave({
    isDirty: options.isDirty,
    isSaving: options.isSavingBoard,
    canRun: () => !isNodeDragActive,
    save: (autosaveOptions) =>
      saveCurrentBoardSnapshot({ waitForPreview: false, silent: autosaveOptions?.silent }),
  })

  function markDirty() {
    if (!canSaveBoard.value) return
    options.isDirty.value = true
    if (options.saveStatus.value !== 'failed') options.saveStatus.value = 'idle'
    markDirtyAndSchedule()
  }

  function beginNodeDragTransaction() {
    isNodeDragActive = true
    flowHistory.onNodeDragStart()
  }

  function commitNodeDragTransaction() {
    try {
      flowHistory.onNodeDragStop(() => {
        markDirty()
      })
    } finally {
      isNodeDragActive = false
    }
  }

  const graphRuntime: CanvasFlowGraphRuntimeControls = {
    captureHistory: captureFullGraphHistory,
    markDirty: () => {
      markDirty()
    },
    nodeDrag: {
      begin: beginNodeDragTransaction,
      commit: commitNodeDragTransaction,
    },
  }

  function hasRemoveChange(changes: Array<NodeChange | EdgeChange>): boolean {
    return changes.some((change) => change.type === 'remove')
  }

  function dedupeRemoveChanges<T extends NodeChange | EdgeChange>(changes: T[]): T[] {
    const removedIds = new Set<string>()
    return changes.filter((change) => {
      if (change.type !== 'remove') return true
      if (removedIds.has(change.id)) return false
      removedIds.add(change.id)
      return true
    })
  }

  function createConnectedEdgeRemoveChanges(
    nodeChanges: NodeChange[],
    edgeChanges: EdgeChange[],
  ): EdgeChange[] {
    const removedNodeIds = new Set(
      nodeChanges
        .filter((change) => change.type === 'remove')
        .map((change) => change.id),
    )
    if (removedNodeIds.size === 0) return edgeChanges

    const removedEdgeIds = new Set(
      edgeChanges
        .filter((change) => change.type === 'remove')
        .map((change) => change.id),
    )
    const cleanupEdgeChanges = options.edges.value
      .filter(
        (edge) =>
          !removedEdgeIds.has(edge.id) &&
          (removedNodeIds.has(edge.source) || removedNodeIds.has(edge.target)),
      )
      .map(
        (edge): EdgeChange => ({
          id: edge.id,
          type: 'remove',
          source: edge.source,
          target: edge.target,
          sourceHandle: edge.sourceHandle ?? null,
          targetHandle: edge.targetHandle ?? null,
        }),
      )

    return cleanupEdgeChanges.length > 0
      ? edgeChanges.concat(cleanupEdgeChanges)
      : edgeChanges
  }

  function hasApplicableRemoveChanges(nodeChanges: NodeChange[], edgeChanges: EdgeChange[]) {
    const currentNodeIds = new Set(options.nodes.value.map((node) => node.id))
    const currentEdgeIds = new Set(options.edges.value.map((edge) => edge.id))
    return (
      nodeChanges.some((change) => change.type === 'remove' && currentNodeIds.has(change.id)) ||
      edgeChanges.some((change) => change.type === 'remove' && currentEdgeIds.has(change.id))
    )
  }

  function commitNativeRemoveOperation() {
    isNativeRemoveCommitScheduled = false
    const historyBefore = nativeRemoveHistoryBefore
    const nodeChanges = dedupeRemoveChanges(pendingNativeRemoveNodeChanges)
    const edgeChanges = dedupeRemoveChanges(
      createConnectedEdgeRemoveChanges(nodeChanges, pendingNativeRemoveEdgeChanges),
    )
    nativeRemoveHistoryBefore = null
    pendingNativeRemoveNodeChanges = []
    pendingNativeRemoveEdgeChanges = []
    if (!historyBefore || !hasApplicableRemoveChanges(nodeChanges, edgeChanges)) return

    const result = graphAdapter.applyGraphChanges(
      { nodes: nodeChanges, edges: edgeChanges },
      createFlowGraphCommandOptions({ historyBefore }),
    )
    if (result.didChange) options.onNativeRemove?.()
  }

  function scheduleNativeRemoveCommit() {
    if (!nativeRemoveHistoryBefore) {
      nativeRemoveHistoryBefore = flowHistory.capture()
    }
    if (isNativeRemoveCommitScheduled) return
    isNativeRemoveCommitScheduled = true
    scheduleMicrotask(() => {
      scheduleMicrotask(commitNativeRemoveOperation)
    })
  }

  function onNodesChange(changes: NodeChange[]) {
    if (changes.length === 0) return
    if (hasRemoveChange(changes)) {
      pendingNativeRemoveNodeChanges = pendingNativeRemoveNodeChanges.concat(changes)
      scheduleNativeRemoveCommit()
      return
    }
    graphAdapter.applyNodeChanges(changes)
  }

  function onEdgesChange(changes: EdgeChange[]) {
    if (changes.length === 0) return
    if (hasRemoveChange(changes)) {
      pendingNativeRemoveEdgeChanges = pendingNativeRemoveEdgeChanges.concat(changes)
      scheduleNativeRemoveCommit()
      return
    }
    graphAdapter.applyEdgeChanges(changes)
  }

  return {
    currentBoardId,
    loadInitialOrRouteBoard,
    flushAutosave,
    flowHistory,
    graphRuntime,
    graphAdapter,
    onNodesChange,
    onEdgesChange,
    fullNodes: options.nodes,
    fullEdges: options.edges,
  }
}
