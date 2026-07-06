import { useFlowImageActionOrchestrator } from '../composables/useFlowImageActionOrchestrator'
import { useFlowPromptGenerationOrchestrator } from '../composables/useFlowPromptGenerationOrchestrator'
import { useImageCanvasExtendOverlay } from '../composables/useImageCanvasExtendOverlay'
import { useCanvasImageTools } from '../image-tools'
import type { UseCanvasImageToolsOptions } from '../image-tools/useCanvasImageTools'
import type { CanvasFlowPaneAdapter } from '../flow-pane'
import type { CanvasFeedback } from '../feedback/useCanvasFeedback'
import type { CanvasTaskRuntime } from '../composables/useCanvasTaskRuntime'

export type CanvasImageWorkflowToolsOptions = Omit<
  UseCanvasImageToolsOptions,
  | 'nodes'
  | 'currentBoardId'
  | 'getImageNodeDisplayPayload'
  | 'getImageNodeSnapshot'
  | 'screenToFlowCoordinate'
  | 'getViewport'
  | 'setViewport'
  | 'setCenter'
  | 'createImageNodeFromFile'
  | 'connectImageNodes'
  | 'uploadImageToBoard'
  | 'openImageUrlPreview'
  | 'updateOutputCount'
  | 'runInpaintAction'
  | 'runPromptGeneration'
  | 'isDirectActionRunning'
  | 'clearSelectedFlowElements'
  | 'startCanvasInteraction'
  | 'stopCanvasInteraction'
  | 'feedback'
>

export type CanvasImageWorkflowActionOptions = Omit<
  Parameters<typeof useFlowImageActionOrchestrator>[0],
  'runImageDirectAction' | 'runFusionPromptGeneration' | 'onFlowNodeParamUpdate' | 'onWarning'
>

export type CanvasImageWorkflowPromptGenerationOptions = Pick<
  Parameters<typeof useFlowPromptGenerationOrchestrator>[0],
  'cancelFlowExpandMode' | 'cancelFlowMaskEditorMode'
>

export type CanvasImageWorkflowExpandOverlayOptions = Omit<
  Parameters<typeof useImageCanvasExtendOverlay>[0],
  'nodes' | 'getImageNodeDisplayPayload' | 'onBeforePointerCancel' | 'onCommit'
>

type CanvasImageWorkflowTaskPort = Pick<
  CanvasTaskRuntime,
  | 'runImageInpaintAction'
  | 'runImagePromptGeneration'
  | 'runImagePromptGenerationFromSource'
  | 'runTextPromptGeneration'
  | 'runFusionPromptGeneration'
  | 'runImageDirectAction'
  | 'runCanvasExtendAction'
  | 'isNodeDirectActionRunning'
>

type CanvasImageWorkflowGraphPort = Pick<
  UseCanvasImageToolsOptions,
  | 'nodes'
  | 'currentBoardId'
  | 'getImageNodeDisplayPayload'
  | 'getImageNodeSnapshot'
  | 'createImageNodeFromFile'
  | 'connectImageNodes'
  | 'uploadImageToBoard'
  | 'clearSelectedFlowElements'
  | 'startCanvasInteraction'
  | 'stopCanvasInteraction'
> & {
  getMediaInputImages: Parameters<
    typeof useFlowPromptGenerationOrchestrator
  >[0]['getMediaInputImages']
  resolvePromptInputImageNodeIds: Parameters<
    typeof useFlowPromptGenerationOrchestrator
  >[0]['resolvePromptInputImageNodeIds']
  updateSnapshotNodeParam: Parameters<
    typeof useFlowImageActionOrchestrator
  >[0]['onFlowNodeParamUpdate']
  markNodeExecutionDirty: (nodeId: string, options?: { includeSelf?: boolean }) => void
}

export type CanvasImageWorkflowOptions = {
  graph: CanvasImageWorkflowGraphPort
  task: CanvasImageWorkflowTaskPort
  flowPaneAdapter: CanvasFlowPaneAdapter
  openImageUrlPreview: UseCanvasImageToolsOptions['openImageUrlPreview']
  feedback: CanvasFeedback['imageTools']
}

export function useCanvasImageWorkflow(options: CanvasImageWorkflowOptions) {
  const { graph, task } = options

  function markImageWorkflowNodeDirty(
    nodeId: string,
    payload: Parameters<CanvasImageWorkflowGraphPort['updateSnapshotNodeParam']>[1],
  ) {
    graph.updateSnapshotNodeParam(nodeId, payload)
    graph.markNodeExecutionDirty(nodeId, { includeSelf: true })
  }

  function createImageTools(imageToolOptions: CanvasImageWorkflowToolsOptions) {
    return useCanvasImageTools({
      ...imageToolOptions,
      nodes: graph.nodes,
      currentBoardId: graph.currentBoardId,
      getImageNodeDisplayPayload: graph.getImageNodeDisplayPayload,
      getImageNodeSnapshot: graph.getImageNodeSnapshot,
      screenToFlowCoordinate: options.flowPaneAdapter.screenToFlowCoordinate,
      getViewport: options.flowPaneAdapter.getViewport,
      setViewport: options.flowPaneAdapter.setViewport,
      setCenter: options.flowPaneAdapter.setCenter,
      createImageNodeFromFile: graph.createImageNodeFromFile,
      connectImageNodes: graph.connectImageNodes,
      uploadImageToBoard: graph.uploadImageToBoard,
      openImageUrlPreview: options.openImageUrlPreview,
      updateOutputCount: markImageWorkflowNodeDirty,
      runInpaintAction: task.runImageInpaintAction,
      runPromptGeneration: task.runImagePromptGeneration,
      isDirectActionRunning: task.isNodeDirectActionRunning,
      clearSelectedFlowElements: graph.clearSelectedFlowElements,
      startCanvasInteraction: graph.startCanvasInteraction,
      stopCanvasInteraction: graph.stopCanvasInteraction,
      feedback: options.feedback,
    })
  }

  function createImageActionHandlers(actionOptions: CanvasImageWorkflowActionOptions) {
    return useFlowImageActionOrchestrator({
      ...actionOptions,
      runImageDirectAction: task.runImageDirectAction,
      runFusionPromptGeneration: task.runFusionPromptGeneration,
      onFlowNodeParamUpdate: markImageWorkflowNodeDirty,
      onWarning: options.feedback.actionUnavailable,
    })
  }

  function createPromptGenerationHandlers(
    promptOptions: CanvasImageWorkflowPromptGenerationOptions,
  ) {
    return useFlowPromptGenerationOrchestrator({
      ...promptOptions,
      getMediaInputImages: graph.getMediaInputImages,
      resolvePromptInputImageNodeIds: graph.resolvePromptInputImageNodeIds,
      runTextPromptGeneration: task.runTextPromptGeneration,
      runImagePromptGeneration: task.runImagePromptGeneration,
      runImagePromptGenerationFromSource: task.runImagePromptGenerationFromSource,
    })
  }

  function createCanvasExtendOverlay(overlayOptions: CanvasImageWorkflowExpandOverlayOptions) {
    return useImageCanvasExtendOverlay({
      ...overlayOptions,
      nodes: graph.nodes,
      getImageNodeDisplayPayload: graph.getImageNodeDisplayPayload,
      onBeforePointerCancel: graph.clearSelectedFlowElements,
      onCommit: async (payload) => {
        await task.runCanvasExtendAction(payload.nodeId, {
          frameRect: payload.frameRect,
          originalImageRect: payload.originalImageRect,
          prompt: payload.prompt,
          outputCount: payload.outputCount,
        })
      },
    })
  }

  return {
    runImageDirectAction: task.runImageDirectAction,
    runCanvasExtendAction: task.runCanvasExtendAction,
    createImageTools,
    createImageActionHandlers,
    createPromptGenerationHandlers,
    createCanvasExtendOverlay,
  }
}
