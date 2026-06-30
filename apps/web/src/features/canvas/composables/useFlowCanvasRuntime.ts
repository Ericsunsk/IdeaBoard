import type { Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import { useFlowHistory } from './useFlowHistory'
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
  fullNodes: Ref<Node[]>
  fullEdges: Ref<Edge[]>
  boardBaseSnapshot: Ref<CanvasBoardSnapshot>
  isDirty: Ref<boolean>
  saveStatus: Ref<SaveStatus>
  isLoadingBoardSnapshot: Ref<boolean>
  isSavingBoard: Ref<boolean>
  boardSession: CanvasBoardSession
  getViewport: () => ViewportState
  setViewport: (viewport: ViewportState) => void
  setNodes: (nodes: Node[]) => void
  setEdges: (edges: Edge[]) => void
  onWarning: (message: string) => void
  onSuccess: (message: string) => void
  onError: (message: string) => void
}

export function useFlowCanvasRuntime(options: UseFlowCanvasRuntimeOptions) {
  let isNodeDragActive = false

  function hasSameItemsByReference<T>(current: T[], next: T[]): boolean {
    return current.length === next.length && current.every((item, index) => item === next[index])
  }

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
    const selectedNodeIds = collectSelectedItemIds(options.nodes.value, options.fullNodes.value)
    const selectedEdgeIds = collectSelectedItemIds(options.edges.value, options.fullEdges.value)
    return {
      nodes: preserveSelectedItems(nextGraph.nodes, selectedNodeIds),
      edges: preserveSelectedItems(nextGraph.edges, selectedEdgeIds),
    }
  }

  function syncVueFlowItemIntoFullGraph<T extends SelectableFlowItem>(
    fullItem: T,
    vueFlowItem: T,
  ): T {
    if (fullItem.selected === true && vueFlowItem.selected === undefined) {
      return { ...vueFlowItem, selected: true }
    }
    return vueFlowItem
  }

  function commitVueFlowGraph(nextGraph: { nodes: Node[]; edges: Edge[] }) {
    if (!hasSameItemsByReference(options.nodes.value, nextGraph.nodes)) {
      options.nodes.value = nextGraph.nodes
      options.setNodes(nextGraph.nodes)
    }
    if (!hasSameItemsByReference(options.edges.value, nextGraph.edges)) {
      options.edges.value = nextGraph.edges
      options.setEdges(nextGraph.edges)
    }
  }

  function commitFlowGraph(
    nextGraph: { nodes: Node[]; edges: Edge[] },
    commitOptions: FlowGraphCommitOptions = {},
  ) {
    const resolvedGraph = commitOptions.preserveSelection
      ? preserveCurrentSelection(nextGraph)
      : nextGraph
    options.fullNodes.value = resolvedGraph.nodes
    options.fullEdges.value = resolvedGraph.edges
    commitVueFlowGraph(resolvedGraph)
  }

  function commitFlowNodes(nextNodes: Node[]) {
    commitFlowGraph({ nodes: nextNodes, edges: options.fullEdges.value })
  }

  function commitFlowEdges(nextEdges: Edge[]) {
    commitFlowGraph({ nodes: options.fullNodes.value, edges: nextEdges })
  }

  function syncVueFlowGraphIntoFullGraph() {
    if (options.nodes.value.length === 0 && options.edges.value.length === 0) return

    const vueFlowNodesById = new Map(options.nodes.value.map((node) => [node.id, node]))
    const vueFlowEdgesById = new Map(options.edges.value.map((edge) => [edge.id, edge]))
    let nextNodes: Node[] | null = null
    let nextEdges: Edge[] | null = null

    options.fullNodes.value.forEach((node, index) => {
      const vueFlowNode = vueFlowNodesById.get(node.id)
      if (!vueFlowNode || vueFlowNode === node) return
      if (!nextNodes) nextNodes = [...options.fullNodes.value]
      nextNodes[index] = syncVueFlowItemIntoFullGraph(node, vueFlowNode)
    })

    options.fullEdges.value.forEach((edge, index) => {
      const vueFlowEdge = vueFlowEdgesById.get(edge.id)
      if (!vueFlowEdge || vueFlowEdge === edge) return
      if (!nextEdges) nextEdges = [...options.fullEdges.value]
      nextEdges[index] = syncVueFlowItemIntoFullGraph(edge, vueFlowEdge)
    })

    if (nextNodes) options.fullNodes.value = nextNodes
    if (nextEdges) options.fullEdges.value = nextEdges
  }

  function captureFullGraphHistory() {
    syncVueFlowGraphIntoFullGraph()
    return flowHistory.capture()
  }

  function refreshSnapshot() {
    // Snapshot preview UI has been removed; callers keep this hook for flow parity.
  }

  const flowHistory = useFlowHistory({
    nodes: options.fullNodes,
    edges: options.fullEdges,
    maxSteps: 10,
    onRestore: () => {
      markDirty()
    },
    commitNodes: (nodes) => commitFlowNodes(nodes as Node[]),
    commitEdges: (edges) => commitFlowEdges(edges as Edge[]),
  })

  const boardPersistence = useFlowBoardPersistence({
    fullNodes: options.fullNodes,
    fullEdges: options.fullEdges,
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
    syncVueFlowGraphIntoFullGraph()
    isNodeDragActive = true
    flowHistory.onNodeDragStart()
  }

  function commitNodeDragTransaction() {
    try {
      syncVueFlowGraphIntoFullGraph()
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
    nodes: options.fullNodes,
    edges: options.fullEdges,
    captureHistory: captureFullGraphHistory,
    pushHistory: flowHistory.push,
    beginDragHistory: beginNodeDragTransaction,
    commitDragHistory: commitNodeDragTransaction,
    markDirty,
    refreshSnapshot: () => {
      syncVueFlowGraphIntoFullGraph()
      refreshSnapshot()
    },
    commitGraph: commitFlowGraph,
    commitNodes: commitFlowNodes,
    commitEdges: commitFlowEdges,
  })

  return {
    currentBoardId,
    loadInitialOrRouteBoard,
    flushAutosave,
    flowHistory,
    flowTransactions,
    fullNodes: options.fullNodes,
    fullEdges: options.fullEdges,
    refreshSnapshot,
  }
}
