import type { ComputedRef, Ref } from 'vue'
import type { PromptAspectRatio } from '../../../board/ports/canvas'
import type {
  ActiveNodePanel,
  ActiveNodePanelKind,
} from '../../ui/screens/controllers'
import type { OutpaintDragAnchor } from '../../tools/outpaint'
import { useImageOutpaintOverlay } from '../../tools/outpaint'
import type { CanvasFeedback } from '../workspace/useCanvasFeedback.ts'
import type { ImageNodeAction } from '../../core/nodes/imageActionRegistry.ts'
import type { CanvasVueFlowAdapter } from '../../session/flow-adapter'
import {
  useCanvasImageTools,
  type CanvasImageTools,
  type UseCanvasImageToolsOptions,
} from '../../tools/image-tools'
import { useFlowOutpaintInteractionHandlers } from '../../tools/outpaint'
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
import { createGenerationIntent, type GenerationIntentOptions } from './generationIntent'

type CanvasImageWorkflowExecutionPort = Pick<
  ImageWorkflowExecution,
  | 'runImageInpaintAction'
  | 'runGeneration'
  | 'runFusionGeneration'
  | 'runImageDirectAction'
  | 'runOutpaintAction'
  | 'isGenerationAvailable'
  | 'isNodeExecutionActive'
  | 'hydratePendingImageLeases'
  | 'reattachPendingImageResults'
>

type CanvasImageWorkflowGraphPort = Pick<
  UseCanvasImageToolsOptions,
  | 'nodes'
  | 'currentBoardId'
  | 'getImageNodeDisplayPayload'
  | 'getImageNodeSnapshot'
  | 'updateSnapshotNode'
  | 'createPendingRetouchNode'
  | 'applyImageFileToImageNode'
  | 'removeNode'
  | 'connectionAdapter'
  | 'uploadImageToBoard'
  | 'clearSelectedFlowElements'
  | 'startCanvasInteraction'
  | 'stopCanvasInteraction'
  | 'isCanvasInteractionLocked'
> & {
  getMediaInputImages: GenerationIntentOptions['getMediaInputImages']
  updateSnapshotNodeParam: ImageActionIntentRouterOptions['params']['updateNodeParam']
  markNodeExecutionDirty: (nodeId: string, options?: { shouldIncludeSelf?: boolean }) => void
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
  flowPaneAdapter: CanvasVueFlowAdapter
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
    outpaint: {
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
      onFlowImageOutpaintSubmit: (nodeId: string, prompt: string) => void
      onFlowOutpaintHandlePointerDown: (
        nodeId: string,
        anchor: OutpaintDragAnchor,
        event: PointerEvent,
      ) => void
      onFlowOutpaintFramePointerDown: (nodeId: string, event: PointerEvent) => void
      onFlowOutpaintHandlePointerMove: (event: PointerEvent) => void
      onFlowOutpaintHandlePointerUp: (event: PointerEvent) => void
    }
    cancelOutpaint: () => void
    hideStrengthControl: () => void
  }
}

type CanvasImageWorkflowRuntime = CanvasImageWorkflow & {
  hydratePendingImageLeases: () => number
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
    graph.markNodeExecutionDirty(nodeId, { shouldIncludeSelf: true })
  }

  const { strengthControl, outpaint, imageTools } = useWorkflowImageEditing(options, execution)

  const imageActions = createImageActionIntentRouter({
    state: {
      flowRetouchNodeId: imageTools.viewModel.active.nodeId,
    },
    strength: {
      consumeSuppressedClick: strengthControl.consumeSuppressedStrengthClick,
      getValue: strengthControl.getActionStrength,
    },
    ui: {
      openPanel: ui.panels.open,
      clearPanel: ui.panels.clear,
      hideStrengthControl: strengthControl.hideStrengthControl,
      warn: options.feedback.imageTools.actionUnavailable,
    },
    selection: {
      selectNode: options.flowPaneAdapter.selectOnlyNode,
    },
    modes: {
      cancelOutpaint: outpaint.cancelOutpaintMode,
      cancelInpaint: imageTools.commands.lifecycle.cancel,
      enterOutpaint: outpaint.enterOutpaintMode,
      enterRetouch: imageTools.commands.lifecycle.enter,
      focusRetouch: imageTools.commands.editor.focusAndShowChrome,
    },
    execution: {
      runImageDirectAction: execution.runImageDirectAction,
      runFusionGeneration: execution.runFusionGeneration,
    },
    params: {
      updateNodeParam: markImageWorkflowNodeDirty,
    },
  })
  const generation = createGenerationIntent({
    cancelFlowOutpaintMode: outpaint.cancelOutpaintMode,
    cancelFlowInpaintMode: imageTools.commands.lifecycle.cancel,
    getMediaInputImages: graph.getMediaInputImages,
    runGeneration: execution.runGeneration,
  })
  const outpaintInteractions = useFlowOutpaintInteractionHandlers({
    flowOutpaintNodeId: outpaint.outpaintNodeId,
    clearActiveNodePanel: ui.panels.clear,
    hideStrengthControl: strengthControl.hideStrengthControl,
    commitFlowOutpaintMode: outpaint.commitOutpaintMode,
    beginFlowOutpaintHandleResize: outpaint.beginHandleResize,
    beginFlowOutpaintFrameDrag: outpaint.beginFrameDrag,
  })

  return {
    viewModel: {
      tools: imageTools.viewModel,
      outpaint: {
        nodeId: outpaint.outpaintNodeId,
        overlayStyle: outpaint.outpaintOverlayStyle,
        handleRadius: outpaint.handleRadius,
      },
    },
    commands: {
      isGenerationAvailable: execution.isGenerationAvailable,
      tools: imageTools.commands,
      interactions: {
        ...imageActions,
        ...generation,
        ...outpaintInteractions,
        onFlowOutpaintHandlePointerMove: outpaint.onHandlePointerMove,
        onFlowOutpaintHandlePointerUp: outpaint.onHandlePointerUp,
      },
      cancelOutpaint: outpaint.cancelOutpaintMode,
      hideStrengthControl: strengthControl.hideStrengthControl,
    },
    hydratePendingImageLeases: execution.hydratePendingImageLeases,
    reattachPendingImageResults: execution.reattachPendingImageResults,
  }
}

function useWorkflowImageEditing(
  options: CanvasImageWorkflowOptions,
  execution: ImageWorkflowExecution,
) {
  const { graph, ui } = options
  const strengthControl = useFlowImageActionStrengthControl({
    onCommit: () => undefined,
    onCancel: ui.panels.clear,
  })

  const outpaint = useImageOutpaintOverlay({
    activeTool: ui.activeTool,
    getImageStageElement: ui.getImageStageElement,
    nodes: graph.nodes,
    getImageNodeDisplayPayload: graph.getImageNodeDisplayPayload,
    startCanvasInteraction: graph.startCanvasInteraction,
    stopCanvasInteraction: graph.stopCanvasInteraction,
    onBeforePointerCancel: graph.clearSelectedFlowElements,
    onCommit: async (payload) => {
      await execution.runOutpaintAction(payload.nodeId, {
        frameRect: payload.frameRect,
        originalImageRect: payload.originalImageRect,
        prompt: payload.prompt,
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
    updateSnapshotNode: graph.updateSnapshotNode,
    getImageStageElement: ui.getImageStageElement,
    screenToFlowCoordinate: options.flowPaneAdapter.screenToFlowCoordinate,
    getViewport: options.flowPaneAdapter.getViewport,
    setViewport: options.flowPaneAdapter.setViewport,
    setCenter: options.flowPaneAdapter.setCenter,
    createPendingRetouchNode: graph.createPendingRetouchNode,
    applyImageFileToImageNode: graph.applyImageFileToImageNode,
    removeNode: graph.removeNode,
    connectionAdapter: graph.connectionAdapter,
    uploadImageToBoard: graph.uploadImageToBoard,
    openImageUrlPreview: options.openImageUrlPreview,
    clearActiveNodePanel: ui.panels.clear,
    hideStrengthControl: strengthControl.hideStrengthControl,
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

  return { strengthControl, outpaint, imageTools }
}
