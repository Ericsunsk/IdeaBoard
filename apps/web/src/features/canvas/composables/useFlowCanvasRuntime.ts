import type { Ref } from 'vue'
import type { Edge, EdgeChange, Node, NodeChange } from '@vue-flow/core'
import { useFlowHistory, type FlowHistorySnapshot } from './useFlowHistory'
import { useFlowTransactions } from './useFlowTransactions'
import { useFlowBoardPersistence } from './useFlowBoardPersistence'
import { useFlowAutosave } from './useFlowAutosave'
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
}

export function useFlowCanvasRuntime(options: UseFlowCanvasRuntimeOptions) {
  let isNodeDragActive = false
  let nativeRemoveHistoryBefore: FlowHistorySnapshot | null = null
  let isNativeRemoveCommitScheduled = false
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

  function commitFlowGraph(
    nextGraph: { nodes: Node[]; edges: Edge[] },
    commitOptions: FlowGraphCommitOptions = {},
  ) {
    const resolvedGraph = commitOptions.preserveSelection
      ? preserveCurrentSelection(nextGraph)
      : nextGraph
    options.nodes.value = resolvedGraph.nodes
    options.edges.value = resolvedGraph.edges
  }

  function commitFlowNodes(nextNodes: Node[]) {
    commitFlowGraph({ nodes: nextNodes, edges: options.edges.value })
  }

  function commitFlowEdges(nextEdges: Edge[]) {
    commitFlowGraph({ nodes: options.nodes.value, edges: nextEdges })
  }

  function captureFullGraphHistory() {
    return flowHistory.capture()
  }

  function refreshSnapshot() {
    // Snapshot preview UI has been removed; callers keep this hook for flow parity.
  }

  const flowHistory = useFlowHistory({
    nodes: options.nodes,
    edges: options.edges,
    maxSteps: 10,
    onRestore: () => {
      markDirty()
    },
    commitNodes: (nodes) => commitFlowNodes(nodes as Node[]),
    commitEdges: (edges) => commitFlowEdges(edges as Edge[]),
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
    refreshSnapshot,
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
    if (options.isDirty.value) {
      void flushAutosave({ silent: true })
    }
  }

  const flowTransactions = useFlowTransactions({
    nodes: options.nodes,
    edges: options.edges,
    captureHistory: captureFullGraphHistory,
    pushHistory: flowHistory.push,
    beginDragHistory: beginNodeDragTransaction,
    commitDragHistory: commitNodeDragTransaction,
    markDirty,
    refreshSnapshot: () => {
      refreshSnapshot()
    },
    commitGraph: commitFlowGraph,
    commitNodes: commitFlowNodes,
    commitEdges: commitFlowEdges,
  })

  function commitNativeRemoveOperation() {
    isNativeRemoveCommitScheduled = false
    const historyBefore = nativeRemoveHistoryBefore
    nativeRemoveHistoryBefore = null
    if (!historyBefore) return
    const didChange = flowHistory.push(historyBefore)
    if (!didChange) return
    options.onNativeRemove?.()
    markDirty()
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
    if (changes.some((change) => change.type === 'remove')) scheduleNativeRemoveCommit()
  }

  function onEdgesChange(changes: EdgeChange[]) {
    if (changes.length === 0) return
    if (changes.some((change) => change.type === 'remove')) scheduleNativeRemoveCommit()
  }

  return {
    currentBoardId,
    loadInitialOrRouteBoard,
    flushAutosave,
    flowHistory,
    flowTransactions,
    onNodesChange,
    onEdgesChange,
    fullNodes: options.nodes,
    fullEdges: options.edges,
    refreshSnapshot,
  }
}
