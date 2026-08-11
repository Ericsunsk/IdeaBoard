import type { RouteLocationNormalizedLoaded, Router } from 'vue-router'
import type { ImageNodeItem } from '../../../canvas-board/domain'
import { useCanvasViewportRuntime } from '../../composables/useCanvasViewportRuntime'
import { useCanvasBoardRuntime } from '../../composables/useCanvasBoardRuntime'
import { useCanvasFeedback } from '../../feedback/useCanvasFeedback'
import type { CanvasFlowPaneAdapter } from '../../flow-pane'
import {
  type CanvasGraphEditingBrowserAdapter,
  type CanvasGraphInteractionBrowserAdapter,
  type CanvasGraphEditingUploadAdapter,
} from '../../graph-editing'
import type { CanvasImageWorkflowUiAdapter } from '../../image-workflow'
import { createCanvasTaskLifecycle } from '../../task-lifecycle'
import { taskApi } from '../../../../shared/api/task'
import type { CanvasWorkspaceRuntime } from '../types'
import { createCanvasWorkspaceAutosave } from './createCanvasWorkspaceAutosave'
import { createCanvasWorkspaceGraphRuntime } from './createCanvasWorkspaceGraphRuntime'
import { createCanvasWorkspaceImageWorkflow } from './createCanvasWorkspaceImageWorkflow'
import { createCanvasWorkspaceRuntimeFacade } from './createCanvasWorkspaceRuntimeFacade'
import { createCanvasExecutionRuntime } from '../../execution/runtime/CanvasExecutionRuntime'

export type UseCanvasWorkspaceRuntimeOptions = {
  route: RouteLocationNormalizedLoaded
  router: Router
  getUserIdentity: () => { userId: number; userUid: string }
  flowPaneAdapter: CanvasFlowPaneAdapter
  uploadApi: CanvasGraphEditingUploadAdapter
  graphEditingBrowser: CanvasGraphEditingBrowserAdapter
  graphInteractionBrowser: CanvasGraphInteractionBrowserAdapter
  openNodeImagePreview: (snapshot: ImageNodeItem, mode: 'latest-output') => Promise<void>
  openImageUrlPreview: (src: string, ratio?: number) => Promise<void>
  imageWorkflowUi: CanvasImageWorkflowUiAdapter
  logoutAndRedirect: (path: string) => Promise<unknown> | unknown
  openRecharge: () => void
}

export function createCanvasWorkspaceRuntimeParts(options: UseCanvasWorkspaceRuntimeOptions) {
  const feedback = useCanvasFeedback(options.openRecharge)
  const taskLifecycle = createCanvasTaskLifecycle({ cancel: taskApi.cancel })
  const executionRuntime = createCanvasExecutionRuntime()

  const board = useCanvasBoardRuntime({
    route: options.route,
    router: options.router,
    getUserIdentity: options.getUserIdentity,
    logoutAndRedirect: options.logoutAndRedirect,
    feedback: feedback.boardSession,
    cancelTasks: taskLifecycle.cancelTasks,
    executionRuntime,
    vueFlowGraphActions: options.flowPaneAdapter.vueFlowGraphActions,
  })

  const graph = createCanvasWorkspaceGraphRuntime({
    board,
    flowPaneAdapter: options.flowPaneAdapter,
    upload: options.uploadApi,
    editingBrowser: options.graphEditingBrowser,
    interactionBrowser: options.graphInteractionBrowser,
    preview: {
      openImageUrl: options.openImageUrlPreview,
      openNodeImage: options.openNodeImagePreview,
    },
    feedback: feedback.graph,
    executionRuntime,
  })

  const viewport = useCanvasViewportRuntime({
    nodes: board.flowPane.nodes,
    edges: board.flowPane.edges,
    fullNodes: board.flowPane.nodes,
    fullEdges: board.flowPane.edges,
    currentBoardId: board.boardSession.currentBoardId,
    getUserUid: () => options.getUserIdentity().userUid,
    viewport: options.flowPaneAdapter.viewport,
    getViewport: options.flowPaneAdapter.getViewport,
    setViewport: options.flowPaneAdapter.setViewport,
    setCenter: options.flowPaneAdapter.setCenter,
    zoomIn: options.flowPaneAdapter.zoomIn,
    zoomOut: options.flowPaneAdapter.zoomOut,
    panBy: options.flowPaneAdapter.panBy,
    loadInitialOrRouteBoard: board.boardSession.loadInitialOrRouteBoard,
    closeBoardHeaderEditing: board.boardChrome.closeBoardHeaderEditing,
    startCanvasInteraction: graph.interactionPort.startInteraction,
    stopCanvasInteraction: graph.interactionPort.stopInteraction,
  })

  const flushAutosaveWithViewportPersistence = createCanvasWorkspaceAutosave({
    flushViewportPersistence: viewport.flushViewportPersistence,
    flushBoardAutosave: board.boardSession.flushAutosave,
  })

  const imageWorkflow = createCanvasWorkspaceImageWorkflow({
    board,
    graph,
    flowPaneAdapter: options.flowPaneAdapter,
    flushAutosave: flushAutosaveWithViewportPersistence,
    openImageUrlPreview: options.openImageUrlPreview,
    ui: options.imageWorkflowUi,
    feedback: {
      imageTools: feedback.imageTools,
      taskExecution: feedback.taskExecution,
    },
    executionRuntime,
    cancelTasks: taskLifecycle.cancelTasks,
  })

  return {
    board,
    graph,
    viewport,
    persistence: {
      flushAutosave: flushAutosaveWithViewportPersistence,
      history: board.boardSession.history,
      loadCanvasBoardAndProbe: viewport.loadCanvasBoardAndProbe,
      reattachPendingImageResults: imageWorkflow.reattachPendingImageResults,
    },
    imageWorkflow,
  }
}

export function createCanvasWorkspaceRuntime(
  options: UseCanvasWorkspaceRuntimeOptions,
): CanvasWorkspaceRuntime {
  return createCanvasWorkspaceRuntimeFacade(createCanvasWorkspaceRuntimeParts(options))
}
