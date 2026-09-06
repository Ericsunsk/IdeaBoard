import type { CanvasWorkspaceRuntime } from '../types'
import type { CanvasWorkspaceRuntimeParts } from './canvasWorkspaceRuntimePartsTypes'

export function createCanvasWorkspaceRuntimeFacade(
  parts: CanvasWorkspaceRuntimeParts,
): CanvasWorkspaceRuntime {
  const { board, graph, viewport, persistence, imageWorkflow } = parts

  return {
    viewModel: {
      boardChrome: {
        currentBoardId: board.boardSession.currentBoardId,
        isBoardMenuOpen: board.boardChrome.isBoardMenuOpen,
        isRenamingBoard: board.boardChrome.isRenamingBoard,
        boardNameDraft: board.boardChrome.boardNameDraft,
        isRenamePending: board.boardChrome.isRenamePending,
        catalogRecovery: board.boardChrome.catalogRecovery,
        currentBoardName: board.boardChrome.currentBoardName,
        boardOptions: board.boardChrome.boardOptions,
      },
      flowPane: {
        nodes: board.flowPane.nodes,
        edges: board.flowPane.edges,
        renderEdges: board.flowPane.renderEdges,
      },
      persistence: { history: persistence.history },
      viewportControls: { zoomLevel: viewport.zoomLevel },
      graphEditing: graph.viewModel,
      imageWorkflow: imageWorkflow.viewModel,
    },
    commands: {
      boardSession: {
        setBoardNameInputRef: board.boardChrome.setBoardNameInputRef,
        toggleBoardMenu: board.boardChrome.toggleBoardMenu,
        startBoardRename: board.boardChrome.startBoardRename,
        cancelBoardRename: board.boardChrome.cancelBoardRename,
        closeBoardHeaderEditing: board.boardChrome.closeBoardHeaderEditing,
        onConfirmBoardRename: board.boardChrome.onConfirmBoardRename,
        reloadCatalog: board.boardChrome.reloadCatalog,
        onSelectBoard: board.boardChrome.onSelectBoard,
        onNavigateRequest: board.boardChrome.onNavigateRequest,
        onLogoutRequest: board.boardChrome.onLogoutRequest,
      },
      flowPane: {
        onNodesChange: board.flowPane.onNodesChange,
        onEdgesChange: board.flowPane.onEdgesChange,
        reconcileRendererSelection: board.flowPane.reconcileRendererSelection,
        connectionAdapter: graph.flowPane.connectionAdapter,
        nodeDragTransactions: board.flowPane.nodeDragTransactions,
      },
      persistence: {
        flushAutosave: persistence.flushAutosave,
        recoverBoardSession: persistence.recoverBoardSession,
      },
      viewportControls: {
        onZoomIn: viewport.onZoomIn,
        onZoomOut: viewport.onZoomOut,
        onPanViewport: viewport.onPanViewport,
        onMiniMapCenterRequested: viewport.onMiniMapCenterRequested,
        onMiniMapDragStart: viewport.onMiniMapDragStart,
        onMiniMapDragMove: viewport.onMiniMapDragMove,
        onMiniMapDragEnd: viewport.onMiniMapDragEnd,
        onViewportChangeEnd: viewport.onViewportChangeEnd,
        onFlowMoveStart: viewport.onFlowMoveStart,
        onFlowMove: viewport.onFlowMove,
        onFlowMoveEnd: viewport.onFlowMoveEnd,
        onNodeDragStart: viewport.onNodeDragStart,
        onNodeDragStop: viewport.onNodeDragStop,
        flushViewportPersistence: viewport.flushViewportPersistence,
      },
      graphEditing: graph.commands,
      imageWorkflow: imageWorkflow.commands,
    },
  } satisfies CanvasWorkspaceRuntime
}
