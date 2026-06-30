import { getCurrentScope, onScopeDispose, type Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import {
  cullFlowGraphForViewport,
  FALLBACK_FLOW_VIEWPORT_SIZE,
  FLOW_GRAPH_VIEWPORT_OVERSCAN_PX,
  isCanvasFlowCullingEnabled,
} from '../utils/flowViewportCulling'
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

type ViewportSize = {
  width: number
  height: number
}

type FlowGraphCommitOptions = {
  viewport?: ViewportState
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
  getViewportSize?: () => ViewportSize
  setViewport: (viewport: ViewportState) => void
  setNodes: (nodes: Node[]) => void
  setEdges: (edges: Edge[]) => void
  onWarning: (message: string) => void
  onSuccess: (message: string) => void
  onError: (message: string) => void
}

export function useFlowCanvasRuntime(options: UseFlowCanvasRuntimeOptions) {
  let isNodeDragActive = false
  let scheduledMountedGraphViewport: ViewportState | undefined
  let scheduledMountedGraphRefreshId: number | null = null
  let scheduledMountedGraphRefreshToken = 0

  function getMountedGraphViewport(override?: ViewportState) {
    return override ?? options.getViewport()
  }

  function getMountedGraphViewportSize() {
    return options.getViewportSize?.() ?? FALLBACK_FLOW_VIEWPORT_SIZE
  }

  function hasSameItemsByReference<T>(current: T[], next: T[]): boolean {
    return current.length === next.length && current.every((item, index) => item === next[index])
  }

  function commitMountedGraph(nextGraph: { nodes: Node[]; edges: Edge[] }) {
    if (!hasSameItemsByReference(options.nodes.value, nextGraph.nodes)) {
      options.nodes.value = nextGraph.nodes
      options.setNodes(nextGraph.nodes)
    }
    if (!hasSameItemsByReference(options.edges.value, nextGraph.edges)) {
      options.edges.value = nextGraph.edges
      options.setEdges(nextGraph.edges)
    }
  }

  function updateMountedGraph(viewport?: ViewportState) {
    if (!isCanvasFlowCullingEnabled()) {
      commitMountedGraph({
        nodes: options.fullNodes.value,
        edges: options.fullEdges.value,
      })
      return
    }

    const visibleGraph = cullFlowGraphForViewport(
      {
        nodes: options.fullNodes.value,
        edges: options.fullEdges.value,
      },
      {
        viewport: getMountedGraphViewport(viewport),
        viewportSize: getMountedGraphViewportSize(),
        overscanPx: FLOW_GRAPH_VIEWPORT_OVERSCAN_PX,
      },
    )
    commitMountedGraph(visibleGraph)
  }

  function cancelScheduledMountedGraphRefresh() {
    if (scheduledMountedGraphRefreshId === null) return
    const frameId = scheduledMountedGraphRefreshId
    scheduledMountedGraphRefreshId = null
    scheduledMountedGraphRefreshToken += 1
    if (typeof globalThis.cancelAnimationFrame === 'function') {
      globalThis.cancelAnimationFrame(frameId)
    }
  }

  function flushMountedGraphRefresh(viewport?: ViewportState) {
    const viewportForRefresh = viewport ?? scheduledMountedGraphViewport
    cancelScheduledMountedGraphRefresh()
    scheduledMountedGraphViewport = undefined
    updateMountedGraph(viewportForRefresh)
  }

  function scheduleMountedGraphRefresh(viewport?: ViewportState) {
    if (!isCanvasFlowCullingEnabled()) return
    scheduledMountedGraphViewport = viewport
    if (typeof globalThis.requestAnimationFrame !== 'function') {
      flushMountedGraphRefresh()
      return
    }
    if (scheduledMountedGraphRefreshId !== null) return

    const refreshToken = ++scheduledMountedGraphRefreshToken
    scheduledMountedGraphRefreshId = globalThis.requestAnimationFrame(() => {
      if (
        scheduledMountedGraphRefreshId === null ||
        refreshToken !== scheduledMountedGraphRefreshToken
      ) {
        return
      }
      scheduledMountedGraphRefreshId = null
      const viewportForRefresh = scheduledMountedGraphViewport
      scheduledMountedGraphViewport = undefined
      updateMountedGraph(viewportForRefresh)
    })
  }

  if (getCurrentScope()) {
    onScopeDispose(() => {
      cancelScheduledMountedGraphRefresh()
      scheduledMountedGraphViewport = undefined
    })
  }

  function commitFlowGraph(
    nextGraph: { nodes: Node[]; edges: Edge[] },
    commitOptions?: FlowGraphCommitOptions,
  ) {
    options.fullNodes.value = nextGraph.nodes
    options.fullEdges.value = nextGraph.edges
    flushMountedGraphRefresh(commitOptions?.viewport)
  }

  function commitFlowNodes(nextNodes: Node[]) {
    commitFlowGraph({ nodes: nextNodes, edges: options.fullEdges.value })
  }

  function commitFlowEdges(nextEdges: Edge[]) {
    commitFlowGraph({ nodes: options.fullNodes.value, edges: nextEdges })
  }

  function mergeMountedGraphIntoFullGraph() {
    if (options.nodes.value.length === 0 && options.edges.value.length === 0) return

    const mountedNodesById = new Map(options.nodes.value.map((node) => [node.id, node]))
    const mountedEdgesById = new Map(options.edges.value.map((edge) => [edge.id, edge]))
    let nextNodes: Node[] | null = null
    let nextEdges: Edge[] | null = null

    options.fullNodes.value.forEach((node, index) => {
      const mounted = mountedNodesById.get(node.id)
      if (!mounted || mounted === node) return
      if (!nextNodes) nextNodes = [...options.fullNodes.value]
      nextNodes[index] = mounted
    })

    options.fullEdges.value.forEach((edge, index) => {
      const mounted = mountedEdgesById.get(edge.id)
      if (!mounted || mounted === edge) return
      if (!nextEdges) nextEdges = [...options.fullEdges.value]
      nextEdges[index] = mounted
    })

    if (nextNodes) options.fullNodes.value = nextNodes
    if (nextEdges) options.fullEdges.value = nextEdges
  }

  function captureFullGraphHistory() {
    mergeMountedGraphIntoFullGraph()
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
    syncMountedGraph: mergeMountedGraphIntoFullGraph,
    getMountedGraphStats: () => ({
      nodes: options.nodes.value.length,
      edges: options.edges.value.length,
    }),
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
    mergeMountedGraphIntoFullGraph()
    isNodeDragActive = true
    flowHistory.onNodeDragStart()
  }

  function commitNodeDragTransaction() {
    try {
      mergeMountedGraphIntoFullGraph()
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
      mergeMountedGraphIntoFullGraph()
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
    scheduleMountedGraphRefresh,
    flushMountedGraphRefresh,
  }
}
