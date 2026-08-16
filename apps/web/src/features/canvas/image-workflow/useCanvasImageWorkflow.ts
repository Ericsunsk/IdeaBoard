import type { ComputedRef, Ref } from 'vue'
import type { PromptAspectRatio } from '../../canvas-board/domain'
import type { ActiveNodePanel, ActiveNodePanelKind } from '../composables/useFlowActiveNodePanels'
import type { CanvasExtendDragAnchor } from '../composables/useImageCanvasExtendOverlay'
import { useImageCanvasExtendOverlay } from '../composables/useImageCanvasExtendOverlay'
import type { CanvasFeedback } from '../feedback/useCanvasFeedback'
import type { ImageNodeAction } from '../config/imageActionRegistry'
import type { CanvasFlowPaneAdapter } from '../flow-pane'
import {
  useCanvasImageTools,
  type CanvasImageTools,
  type UseCanvasImageToolsOptions,
} from '../image-tools'
import { useFlowExpandInteractionHandlers } from './canvasExtendIntent'
import {
  createImageActionIntentRouter,
  type ImageActionIntentRouterOptions,
} from './imageActionIntentRouter'
import { useFlowImageActionStrengthControl } from './imageActionStrengthControl'
import {
  useImageWorkflowExecution,
  type ImageWorkflowExecution,
  type UseImageWorkflowExecutionOptions,
} from './useImageWorkflowExecution'
import {
  createPromptGenerationIntent,
  type PromptGenerationIntentOptions,
} from './promptGenerationIntent'

type CanvasImageWorkflowExecutionPort = Pick<
  ImageWorkflowExecution,
  | 'runImageInpaintAction'
  | 'runPromptGeneration'
  | 'runFusionPromptGeneration'
  | 'runImageDirectAction'
  | 'runCanvasExtendAction'
  | 'isGenerationAvailable'
  | 'isNodeExecutionActive'
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
  | 'isCanvasInteractionLocked'
> & {
  getMediaInputImages: PromptGenerationIntentOptions['getMediaInputImages']
  updateSnapshotNodeParam: ImageActionIntentRouterOptions['onFlowNodeParamUpdate']
  markNodeExecutionDirty: (nodeId: string, options?: { includeSelf?: boolean }) => void
}

export type CanvasImageWorkflowUiAdapter = {
  activeTool: Ref<string>
  canvasWrapRef: Ref<HTMLElement | null>
  getImageStageElement: (nodeId: string) => HTMLElement | null
  panels: {
    open: (panel: ActiveNodePanel) => void
    clear: () => void
    isActive: (kind: ActiveNodePanelKind, nodeId: string) => boolean
    getActiveNodeId: (kind: ActiveNodePanelKind) => string | null
  }
}

export type CanvasImageWorkflowOptions = {
  graph: CanvasImageWorkflowGraphPort
  execution: Omit<UseImageWorkflowExecutionOptions, 'nodes' | 'currentBoardId' | 'feedback'>
  flowPaneAdapter: CanvasFlowPaneAdapter
  openImageUrlPreview: UseCanvasImageToolsOptions['openImageUrlPreview']
  ui: CanvasImageWorkflowUiAdapter
  feedback: {
    imageTools: CanvasFeedback['imageTools']
    taskExecution: CanvasFeedback['taskExecution']
  }
}

export type CanvasImageWorkflow = {
  viewModel: {
    tools: CanvasImageTools['viewModel']
    canvasExtend: {
      nodeId: ComputedRef<string | null>
      overlayStyle: ComputedRef<Record<string, string> | null>
      handleRadius: number
    }
  }
  commands: {
    isGenerationAvailable: CanvasImageWorkflowExecutionPort['isGenerationAvailable']
    tools: CanvasImageTools['commands']
    interactions: {
      onFlowImageContextMenu: (nodeId: string, event: MouseEvent) => void
      onFlowFusionContextMenu: (nodeId: string, event: MouseEvent) => void
      onFlowImageDirectAction: (
        nodeId: string,
        action: ImageNodeAction,
        prompt?: string,
        outputCount?: number,
        strength?: number,
      ) => void
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
      onFlowImageExpandSubmit: (nodeId: string, prompt: string, outputCount: number) => void
      onFlowExpandHandlePointerDown: (
        nodeId: string,
        anchor: CanvasExtendDragAnchor,
        event: PointerEvent,
      ) => void
      onFlowExpandFramePointerDown: (nodeId: string, event: PointerEvent) => void
      onFlowExpandHandlePointerMove: (event: PointerEvent) => void
      onFlowExpandHandlePointerUp: (event: PointerEvent) => void
    }
    cancelCanvasExtend: () => void
    hideStrengthControl: () => void
  }
}

type CanvasImageWorkflowRuntime = CanvasImageWorkflow & {
  reattachPendingImageResults: CanvasImageWorkflowExecutionPort['reattachPendingImageResults']
}

export function useCanvasImageWorkflow(
  options: CanvasImageWorkflowOptions,
): CanvasImageWorkflowRuntime {
  const { graph, ui } = options
  const execution = useImageWorkflowExecution({
    ...options.execution,
    nodes: graph.nodes,
    feedback: options.feedback.taskExecution,
  })

  function markImageWorkflowNodeDirty(
    nodeId: string,
    payload: Parameters<CanvasImageWorkflowGraphPort['updateSnapshotNodeParam']>[1],
  ) {
    graph.updateSnapshotNodeParam(nodeId, payload)
    graph.markNodeExecutionDirty(nodeId, { includeSelf: true })
  }

  const strengthControl = useFlowImageActionStrengthControl({
    onCommit: () => undefined,
    onCancel: ui.panels.clear,
  })

  const canvasExtend = useImageCanvasExtendOverlay({
    activeTool: ui.activeTool,
    getImageStageElement: ui.getImageStageElement,
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

  const imageTools = useCanvasImageTools({
    nodes: graph.nodes,
    activeTool: ui.activeTool,
    canvasWrapRef: ui.canvasWrapRef,
    isCanvasInteractionLocked: graph.isCanvasInteractionLocked,
    currentBoardId: graph.currentBoardId,
    getImageNodeDisplayPayload: graph.getImageNodeDisplayPayload,
    getImageNodeSnapshot: graph.getImageNodeSnapshot,
    getImageStageElement: ui.getImageStageElement,
    screenToFlowCoordinate: options.flowPaneAdapter.screenToFlowCoordinate,
    getViewport: options.flowPaneAdapter.getViewport,
    setViewport: options.flowPaneAdapter.setViewport,
    setCenter: options.flowPaneAdapter.setCenter,
    createImageNodeFromFile: graph.createImageNodeFromFile,
    connectionAdapter: graph.connectionAdapter,
    uploadImageToBoard: graph.uploadImageToBoard,
    openImageUrlPreview: options.openImageUrlPreview,
    clearActiveNodePanel: ui.panels.clear,
    hideStrengthControl: strengthControl.hideStrengthControl,
    updateOutputCount: markImageWorkflowNodeDirty,
    runInpaintAction: execution.runImageInpaintAction,
    isImageActionPanelVisible: (nodeId) => ui.panels.isActive('image-tools', nodeId),
    isImagePromptComposerVisible: (nodeId) => ui.panels.isActive('image-tools', nodeId),
    isGenerationAvailable: execution.isGenerationAvailable,
    isNodeExecutionActive: execution.isNodeExecutionActive,
    strengthControl: {
      activeAction: strengthControl.activeStrengthAction,
      getValue: strengthControl.getActionStrength,
      setRangeRef: strengthControl.setStrengthRangeRef,
      setTriggerRef: strengthControl.setStrengthTriggerRef,
      onRangePointerDown: strengthControl.onStrengthRangePointerDown,
      onInput: strengthControl.onStrengthInput,
      onChange: strengthControl.onStrengthChange,
      onPointerUp: strengthControl.onStrengthPointerUp,
      onClick: strengthControl.onStrengthClick,
    },
    clearSelectedFlowElements: graph.clearSelectedFlowElements,
    startCanvasInteraction: graph.startCanvasInteraction,
    stopCanvasInteraction: graph.stopCanvasInteraction,
    feedback: options.feedback.imageTools,
  })

  const imageActions = createImageActionIntentRouter({
    flowImageEditorNodeId: imageTools.viewModel.active.nodeId,
    consumeSuppressedStrengthClick: strengthControl.consumeSuppressedStrengthClick,
    openActiveNodePanel: ui.panels.open,
    clearActiveNodePanel: ui.panels.clear,
    hideStrengthControl: strengthControl.hideStrengthControl,
    selectFlowNodeForContextMenu: options.flowPaneAdapter.selectOnlyNode,
    cancelFlowExpandMode: canvasExtend.cancelExpandMode,
    cancelFlowMaskEditorMode: imageTools.commands.lifecycle.cancel,
    enterFlowExpandMode: canvasExtend.enterExpandMode,
    enterFlowImageEditorMode: imageTools.commands.lifecycle.enter,
    focusImageEditorAndShowChrome: imageTools.commands.editor.focusAndShowChrome,
    runImageDirectAction: execution.runImageDirectAction,
    runFusionPromptGeneration: execution.runFusionPromptGeneration,
    onFlowNodeParamUpdate: markImageWorkflowNodeDirty,
    onWarning: options.feedback.imageTools.actionUnavailable,
    getActionStrength: strengthControl.getActionStrength,
  })
  const promptGeneration = createPromptGenerationIntent({
    cancelFlowExpandMode: canvasExtend.cancelExpandMode,
    cancelFlowMaskEditorMode: imageTools.commands.lifecycle.cancel,
    getMediaInputImages: graph.getMediaInputImages,
    runPromptGeneration: execution.runPromptGeneration,
  })
  const canvasExtendInteractions = useFlowExpandInteractionHandlers({
    flowExpandNodeId: canvasExtend.expandNodeId,
    clearActiveNodePanel: ui.panels.clear,
    hideStrengthControl: strengthControl.hideStrengthControl,
    commitFlowExpandMode: canvasExtend.commitExpandMode,
    beginFlowExpandHandleResize: canvasExtend.beginHandleResize,
    beginFlowExpandFrameDrag: canvasExtend.beginFrameDrag,
  })

  return {
    viewModel: {
      tools: imageTools.viewModel,
      canvasExtend: {
        nodeId: canvasExtend.expandNodeId,
        overlayStyle: canvasExtend.expandOverlayStyle,
        handleRadius: canvasExtend.handleRadius,
      },
    },
    commands: {
      isGenerationAvailable: execution.isGenerationAvailable,
      tools: imageTools.commands,
      interactions: {
        ...imageActions,
        ...promptGeneration,
        ...canvasExtendInteractions,
        onFlowExpandHandlePointerMove: canvasExtend.onHandlePointerMove,
        onFlowExpandHandlePointerUp: canvasExtend.onHandlePointerUp,
      },
      cancelCanvasExtend: canvasExtend.cancelExpandMode,
      hideStrengthControl: strengthControl.hideStrengthControl,
    },
    reattachPendingImageResults: execution.reattachPendingImageResults,
  }
}
