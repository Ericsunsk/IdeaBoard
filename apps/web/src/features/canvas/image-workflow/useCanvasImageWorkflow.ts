import { useImageCanvasExtendOverlay } from '../composables/useImageCanvasExtendOverlay'
import {
  useCanvasImageTools,
  type CanvasImageTools,
  type UseCanvasImageToolsOptions,
} from '../image-tools'
import type { CanvasFlowPaneAdapter } from '../flow-pane'
import type { CanvasFeedback } from '../feedback/useCanvasFeedback'
import type { ComputedRef } from 'vue'
import type { PromptAspectRatio } from '../../canvas-board/domain'
import type { ImageNodeAction } from '../config/imageActionRegistry'
import type { CanvasExtendDragAnchor } from '../composables/useImageCanvasExtendOverlay'
import {
  createImageActionIntentRouter,
  type ImageActionIntentRouterOptions,
} from './imageActionIntentRouter'
import {
  useImageWorkflowExecution,
  type ImageWorkflowExecution,
  type UseImageWorkflowExecutionOptions,
} from './useImageWorkflowExecution'
import {
  createPromptGenerationIntent,
  type PromptGenerationIntentOptions,
} from './promptGenerationIntent'

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
  | 'connectionAdapter'
  | 'uploadImageToBoard'
  | 'openImageUrlPreview'
  | 'updateOutputCount'
  | 'runInpaintAction'
  | 'runPromptGeneration'
  | 'isGenerationAvailable'
  | 'isDirectActionRunning'
  | 'clearSelectedFlowElements'
  | 'startCanvasInteraction'
  | 'stopCanvasInteraction'
  | 'feedback'
>

export type CanvasImageWorkflowActionOptions = Omit<
  ImageActionIntentRouterOptions,
  'runImageDirectAction' | 'runFusionPromptGeneration' | 'onFlowNodeParamUpdate' | 'onWarning'
>

export type CanvasImageWorkflowExpandOverlayOptions = Omit<
  Parameters<typeof useImageCanvasExtendOverlay>[0],
  'nodes' | 'getImageNodeDisplayPayload' | 'onBeforePointerCancel' | 'onCommit'
>

type CanvasImageWorkflowExecutionPort = Pick<
  ImageWorkflowExecution,
  | 'runImageInpaintAction'
  | 'runImagePromptGeneration'
  | 'runImagePromptGenerationFromSource'
  | 'runTextPromptGeneration'
  | 'runFusionPromptGeneration'
  | 'runImageDirectAction'
  | 'runCanvasExtendAction'
  | 'isGenerationAvailable'
  | 'isNodeDirectActionRunning'
  | 'reattachPendingImageResults'
>

type CanvasImageWorkflowGraphPort = Pick<
  UseCanvasImageToolsOptions,
  | 'nodes'
  | 'currentBoardId'
  | 'getImageNodeDisplayPayload'
  | 'getImageNodeSnapshot'
  | 'createImageNodeFromFile'
  | 'connectionAdapter'
  | 'uploadImageToBoard'
  | 'clearSelectedFlowElements'
  | 'startCanvasInteraction'
  | 'stopCanvasInteraction'
> & {
  getMediaInputImages: PromptGenerationIntentOptions['getMediaInputImages']
  resolvePromptInputImageNodeIds: PromptGenerationIntentOptions['resolvePromptInputImageNodeIds']
  updateSnapshotNodeParam: ImageActionIntentRouterOptions['onFlowNodeParamUpdate']
  markNodeExecutionDirty: (nodeId: string, options?: { includeSelf?: boolean }) => void
}

export type CanvasImageWorkflowOptions = {
  graph: CanvasImageWorkflowGraphPort
  execution: Omit<UseImageWorkflowExecutionOptions, 'nodes' | 'currentBoardId' | 'feedback'>
  flowPaneAdapter: CanvasFlowPaneAdapter
  openImageUrlPreview: UseCanvasImageToolsOptions['openImageUrlPreview']
  feedback: {
    imageTools: CanvasFeedback['imageTools']
    taskExecution: CanvasFeedback['taskExecution']
  }
}

export type CanvasImageWorkflow = {
  isGenerationAvailable: CanvasImageWorkflowExecutionPort['isGenerationAvailable']
  runImageDirectAction: CanvasImageWorkflowExecutionPort['runImageDirectAction']
  runCanvasExtendAction: CanvasImageWorkflowExecutionPort['runCanvasExtendAction']
  createImageTools: (options: CanvasImageWorkflowToolsOptions) => CanvasImageTools
  createImageActionHandlers: (options: CanvasImageWorkflowActionOptions) => {
    onFlowImageContextMenu: (nodeId: string, event: MouseEvent) => void
    onFlowFusionContextMenu: (nodeId: string, event: MouseEvent) => void
    onFlowImageDirectAction: (nodeId: string, action: ImageNodeAction) => void
    onFlowImagePromptGenerate: (nodeId: string, prompt: string, outputCount: number) => void
    onFlowFusionPromptGenerate: (
      nodeId: string,
      prompt: string,
      outputCount: number,
      aspectRatio: PromptAspectRatio,
    ) => void
    onFlowPromptGenerate: (
      promptNodeId: string,
      prompt: string,
      outputCount: number,
      aspectRatio: PromptAspectRatio,
    ) => void
  }
  createCanvasExtendOverlay: (options: CanvasImageWorkflowExpandOverlayOptions) => {
    isExpandMode: ComputedRef<boolean>
    expandNodeId: ComputedRef<string | null>
    expandOverlayStyle: ComputedRef<Record<string, string> | null>
    enterExpandMode: (nodeId: string) => boolean
    cancelExpandMode: () => void
    beginHandleResize: (event: PointerEvent, anchor: CanvasExtendDragAnchor) => void
    beginFrameDrag: (event: PointerEvent) => void
    onHandlePointerMove: (event: PointerEvent) => void
    onHandlePointerUp: (event: PointerEvent) => void
    commitExpandMode: (prompt?: string, outputCount?: number) => Promise<void>
    handleRadius: number
  }
}

type CanvasImageWorkflowRuntime = CanvasImageWorkflow & {
  reattachPendingImageResults: CanvasImageWorkflowExecutionPort['reattachPendingImageResults']
}

export function useCanvasImageWorkflow(
  options: CanvasImageWorkflowOptions,
): CanvasImageWorkflowRuntime {
  const { graph } = options
  const execution = useImageWorkflowExecution({
    ...options.execution,
    nodes: graph.nodes,
    currentBoardId: graph.currentBoardId,
    feedback: options.feedback.taskExecution,
  })

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
      connectionAdapter: graph.connectionAdapter,
      uploadImageToBoard: graph.uploadImageToBoard,
      openImageUrlPreview: options.openImageUrlPreview,
      updateOutputCount: markImageWorkflowNodeDirty,
      runInpaintAction: execution.runImageInpaintAction,
      runPromptGeneration: execution.runImagePromptGeneration,
      isGenerationAvailable: execution.isGenerationAvailable,
      isDirectActionRunning: execution.isNodeDirectActionRunning,
      clearSelectedFlowElements: graph.clearSelectedFlowElements,
      startCanvasInteraction: graph.startCanvasInteraction,
      stopCanvasInteraction: graph.stopCanvasInteraction,
      feedback: options.feedback.imageTools,
    })
  }

  function createImageActionHandlers(actionOptions: CanvasImageWorkflowActionOptions) {
    const imageActions = createImageActionIntentRouter({
      ...actionOptions,
      runImageDirectAction: execution.runImageDirectAction,
      runFusionPromptGeneration: execution.runFusionPromptGeneration,
      onFlowNodeParamUpdate: markImageWorkflowNodeDirty,
      onWarning: options.feedback.imageTools.actionUnavailable,
    })
    const promptGeneration = createPromptGenerationIntent({
      cancelFlowExpandMode: actionOptions.cancelFlowExpandMode,
      cancelFlowMaskEditorMode: actionOptions.cancelFlowMaskEditorMode,
      getMediaInputImages: graph.getMediaInputImages,
      resolvePromptInputImageNodeIds: graph.resolvePromptInputImageNodeIds,
      runTextPromptGeneration: execution.runTextPromptGeneration,
      runImagePromptGeneration: execution.runImagePromptGeneration,
      runImagePromptGenerationFromSource: execution.runImagePromptGenerationFromSource,
    })
    return { ...imageActions, ...promptGeneration }
  }

  function createCanvasExtendOverlay(overlayOptions: CanvasImageWorkflowExpandOverlayOptions) {
    return useImageCanvasExtendOverlay({
      ...overlayOptions,
      nodes: graph.nodes,
      getImageNodeDisplayPayload: graph.getImageNodeDisplayPayload,
      startCanvasInteraction: graph.startCanvasInteraction,
      stopCanvasInteraction: graph.stopCanvasInteraction,
      onBeforePointerCancel: graph.clearSelectedFlowElements,
      onCommit: async (payload) => {
        await execution.runCanvasExtendAction(payload.nodeId, {
          frameRect: payload.frameRect,
          originalImageRect: payload.originalImageRect,
          prompt: payload.prompt,
          outputCount: payload.outputCount,
        })
      },
    })
  }

  return {
    isGenerationAvailable: execution.isGenerationAvailable,
    runImageDirectAction: execution.runImageDirectAction,
    runCanvasExtendAction: execution.runCanvasExtendAction,
    reattachPendingImageResults: execution.reattachPendingImageResults,
    createImageTools,
    createImageActionHandlers,
    createCanvasExtendOverlay,
  }
}
