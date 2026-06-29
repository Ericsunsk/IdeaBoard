import type { Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import { useFlowHistory } from './useFlowHistory'
import { useFlowTransactions } from './useFlowTransactions'
import { useFlowBoardPersistence } from './useFlowBoardPersistence'
import { useFlowAutosave } from './useFlowAutosave'
import type { CanvasBoardSnapshot } from '../../canvas-board/domain'
import type { WorkspaceBoardSession } from '../board-session'

type SaveStatus = 'idle' | 'saved' | 'failed'

type ViewportState = {
  x: number
  y: number
  zoom: number
}

type UseFlowWorkspaceRuntimeOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  boardBaseSnapshot: Ref<CanvasBoardSnapshot>
  isDirty: Ref<boolean>
  saveStatus: Ref<SaveStatus>
  isLoadingBoardSnapshot: Ref<boolean>
  isSavingBoard: Ref<boolean>
  boardSession: WorkspaceBoardSession
  getViewport: () => ViewportState
  setViewport: (viewport: ViewportState) => void
  setNodes: (nodes: Node[]) => void
  setEdges: (edges: Edge[]) => void
  onWarning: (message: string) => void
  onSuccess: (message: string) => void
  onError: (message: string) => void
}

export function useFlowWorkspaceRuntime(options: UseFlowWorkspaceRuntimeOptions) {
  let isNodeDragActive = false

  function commitFlowNodes(nextNodes: Node[]) {
    options.nodes.value = nextNodes
    options.setNodes(nextNodes)
  }

  function commitFlowEdges(nextEdges: Edge[]) {
    options.edges.value = nextEdges
    options.setEdges(nextEdges)
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
    nodes: options.nodes,
    edges: options.edges,
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
    commitNodes: (nodes) => commitFlowNodes(nodes as Node[]),
    commitEdges: (edges) => commitFlowEdges(edges as Edge[]),
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
    captureHistory: flowHistory.capture,
    pushHistory: flowHistory.push,
    beginDragHistory: beginNodeDragTransaction,
    commitDragHistory: commitNodeDragTransaction,
    markDirty,
    refreshSnapshot,
    commitNodes: commitFlowNodes,
    commitEdges: commitFlowEdges,
  })

  return {
    currentBoardId,
    loadInitialOrRouteBoard,
    flushAutosave,
    flowHistory,
    flowTransactions,
    refreshSnapshot,
  }
}
