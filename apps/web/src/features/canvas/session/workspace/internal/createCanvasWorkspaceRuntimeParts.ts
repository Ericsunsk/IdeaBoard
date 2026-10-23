import type { CanvasCollaborationSession } from '../../collaboration'
import type { RouteLocationNormalizedLoaded, Router } from 'vue-router'
import type { BoardSnapshot, ImageNodeItem } from '../../../../board/ports/canvas'
import { useCanvasViewportRuntime } from '../../../session/flow-adapter'
import { useBoardRuntime } from './useBoardRuntime'
import { useCanvasFeedback } from '../useCanvasFeedback.ts'
import type { CanvasVueFlowAdapter } from '../../../session/flow-adapter'
import {
  type CanvasGraphEditingBrowserAdapter,
  type CanvasGraphInteractionBrowserAdapter,
  type CanvasGraphEditingUploadAdapter,
} from '../../graph-editing'
import type { CanvasImageWorkflowUiAdapter } from '../../image-workflow'
import { createCanvasTaskLifecycle } from '../../execution/task-lifecycle/canvasTaskLifecycle'
import { cancelCanvasWorkspaceTask } from '../../execution'
import type { CanvasWorkspaceRuntime } from '../model.ts'
import { createCanvasWorkspaceAutosave } from './createCanvasWorkspaceAutosave'
import { createCanvasWorkspaceGraphRuntime } from './createCanvasWorkspaceGraphRuntime'
import { createCanvasWorkspaceImageWorkflow } from './createCanvasWorkspaceImageWorkflow'
import { createCanvasWorkspaceRuntimeFacade } from './createCanvasWorkspaceRuntimeFacade'
import { createPendingImageExecutionCore } from '../../execution'
import type { LibraryAssetMaterializer } from '../../../../library'

export type UseCanvasWorkspaceRuntimeOptions = {
  route: RouteLocationNormalizedLoaded
  router: Router
  getUserIdentity: () => { userId: number; userUid: string }
  getWorkspaceId?: (() => string) | undefined
  /** Workspace availability gate injected from the screen seam. */
  isWorkspaceAvailable?: (() => boolean) | undefined
  flowPaneAdapter: CanvasVueFlowAdapter
  uploadApi: CanvasGraphEditingUploadAdapter
  materializeLibraryAsset: LibraryAssetMaterializer
  graphEditingBrowser: CanvasGraphEditingBrowserAdapter
  graphInteractionBrowser: CanvasGraphInteractionBrowserAdapter
  openNodeImagePreview: (snapshot: ImageNodeItem, mode: 'latest-output') => Promise<void>
  openImageUrlPreview: (src: string, ratio?: number) => Promise<void>
  imageWorkflowUi: CanvasImageWorkflowUiAdapter
  logoutAndRedirect: (path: string) => Promise<unknown> | unknown
  openRecharge: () => void
  collaboration?: CanvasCollaborationSession | undefined
}

export function createCanvasWorkspaceRuntimeParts(options: UseCanvasWorkspaceRuntimeOptions) {
  const feedback = useCanvasFeedback(options.openRecharge)
  const taskLifecycle = createCanvasTaskLifecycle({
    cancel: (taskId) => cancelCanvasWorkspaceTask(taskId),
  })
  const executionCore = createPendingImageExecutionCore()
  let hydratePendingImageLeases = (_snapshot?: BoardSnapshot) => 0

  const board = useBoardRuntime({
    route: options.route,
    router: options.router,
    getUserIdentity: options.getUserIdentity,
    getWorkspaceId: options.getWorkspaceId,
    logoutAndRedirect: options.logoutAndRedirect,
    feedback: feedback.boardSession,
    executionCore,
    hydratePendingImageLeases: (snapshot) => {
      hydratePendingImageLeases(snapshot)
    },
    cancelTasks: taskLifecycle.cancelTasks,
    vueFlowGraphActions: options.flowPaneAdapter.vueFlowGraphActions,
    collaboration: options.collaboration,
  })

  const graph = createCanvasWorkspaceGraphRuntime({
    board,
    flowPaneAdapter: options.flowPaneAdapter,
    upload: options.uploadApi,
    getWorkspaceId: options.getWorkspaceId,
    materializeLibraryAsset: options.materializeLibraryAsset,
    editingBrowser: options.graphEditingBrowser,
    interactionBrowser: options.graphInteractionBrowser,
    preview: {
      openImageUrl: options.openImageUrlPreview,
      openNodeImage: options.openNodeImagePreview,
    },
    feedback: feedback.graph,
    executionCore,
  })

  const viewport = useCanvasViewportRuntime({
    nodes: board.flowPane.nodes,
    edges: board.flowPane.edges,
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
    flowPaneAdapter: { ...options.flowPaneAdapter, ...viewport.viewportCommands },
    flushAutosave: flushAutosaveWithViewportPersistence,
    openImageUrlPreview: options.openImageUrlPreview,
    ui: options.imageWorkflowUi,
    feedback: {
      imageTools: feedback.imageTools,
      taskExecution: feedback.taskExecution,
    },
    executionCore,
    cancelTasks: taskLifecycle.cancelTasks,
    isWorkspaceAvailable:
      options.isWorkspaceAvailable ??
      (() => {
        try {
          return !!options.getWorkspaceId?.()
        } catch {
          return false
        }
      }),
  })
  hydratePendingImageLeases = imageWorkflow.hydratePendingImageLeases ?? (() => 0)

  async function recoverBoardSession(): Promise<void> {
    await viewport.loadBoardAndProbe()
    await imageWorkflow.reattachPendingImageResults()
    graph.recoveryPort.resumePendingLibraryImports()
  }

  return {
    board,
    graph,
    viewport,
    persistence: {
      flushAutosave: flushAutosaveWithViewportPersistence,
      history: board.boardSession.history,
      recoverBoardSession,
    },
    imageWorkflow,
  }
}

export function createCanvasWorkspaceRuntime(
  options: UseCanvasWorkspaceRuntimeOptions,
): CanvasWorkspaceRuntime {
  return createCanvasWorkspaceRuntimeFacade(createCanvasWorkspaceRuntimeParts(options))
}
