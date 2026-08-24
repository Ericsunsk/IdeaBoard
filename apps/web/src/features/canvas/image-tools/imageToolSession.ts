import { computed, ref } from 'vue'
import type { Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import type { ImageNodeItem } from '../../canvas-board/domain'
import type { CanvasInteractionKey } from '../workspace-ui/useFlowCanvasInteractionState'
import type { CanvasVueFlowConnectionAdapter } from '../vue-flow-adapter'
import type { CanvasGraphNodeIdForKind } from '../graph-runtime'
import { useImageEditorMode } from '../image-editor/useImageEditorMode'
import { useImageEditorSession } from '../image-editor/useImageEditorSession'
import { useImageEditorCoordinateAdapter } from '../image-editor'
import type { ImageAdjustmentFinalApplyProgress } from '../image-editor/runtime/imageAdjustmentFinalApplyContext'
import { cancelImageAdjustmentFinalRender } from '../image-editor/services/imageAdjustmentRenderer'
import type { ImageEditorMode } from '../image-editor/types'
import { useImageMaskEditorOverlay } from '../mask-editor/useImageMaskEditorOverlay'
import { useMaskEditorChromeState } from '../mask-editor/useMaskEditorChromeState'
import type { ImageNodeDisplayPayload } from '../utils/flowImageNodeSelectors'
import type { CanvasFeedback } from '../feedback/useCanvasFeedback'
import { createImageToolMaskOverlayState } from './maskOverlayAdapter'
import type { ImageActionStrengthControlSource } from './imageToolBindings'

type ValueRef<T> = {
  readonly value: T
}

type CanvasImageToolsFeedback = CanvasFeedback['imageTools']

export type FlowViewport = { x: number; y: number; zoom: number }

export type UseCanvasImageToolsOptions = {
  nodes: Ref<Node[]>
  activeTool: Ref<string>
  canvasWrapRef: Ref<HTMLElement | null>
  isCanvasInteractionLocked: ValueRef<boolean>
  currentBoardId: ValueRef<string | null | undefined>
  getImageNodeDisplayPayload: (nodeId: string) => ImageNodeDisplayPayload | null
  getImageNodeSnapshot: (nodeId: string) => ImageNodeItem | null
  getImageStageElement: (nodeId: string) => HTMLElement | null
  screenToFlowCoordinate: (point: { x: number; y: number }) => { x: number; y: number }
  getViewport: () => FlowViewport
  setViewport: (
    viewport: FlowViewport,
    options?: { duration?: number; interpolate?: 'smooth' },
  ) => Promise<unknown> | unknown
  setCenter: (
    x: number,
    y: number,
    options?: { zoom?: number; duration?: number; interpolate?: 'smooth' },
  ) => Promise<unknown> | unknown
  createImageNodeFromFile: (
    point: { x: number; y: number },
    file: File,
  ) => Promise<CanvasGraphNodeIdForKind<'image'> | null>
  connectionAdapter: Pick<CanvasVueFlowConnectionAdapter, 'connectIntent'>
  uploadImageToBoard: (file: File) => Promise<{ url: string; ratio: number }>
  openImageUrlPreview: (url: string, ratio: number) => void | Promise<void>
  clearActiveNodePanel: () => void
  hideStrengthControl: () => void
  runInpaintAction: (
    nodeId: string,
    options: {
      prompt: string
      maskDataUrl: string
    },
  ) => Promise<void> | void
  isImageActionPanelVisible: (nodeId: string) => boolean
  isImagePromptComposerVisible: (nodeId: string) => boolean
  isGenerationAvailable: () => boolean
  isNodeExecutionActive: (nodeId: string) => boolean
  strengthControl: ImageActionStrengthControlSource
  clearSelectedFlowElements: () => void
  startCanvasInteraction: (key: CanvasInteractionKey) => void
  stopCanvasInteraction: (key: CanvasInteractionKey) => void
  feedback: CanvasImageToolsFeedback
}

export type ImageToolSessionCapabilities = {
  board: {
    currentBoardId: ValueRef<string | null | undefined>
    getImageNodeDisplayPayload: UseCanvasImageToolsOptions['getImageNodeDisplayPayload']
    getImageNodeSnapshot: UseCanvasImageToolsOptions['getImageNodeSnapshot']
    createImageNodeFromFile: UseCanvasImageToolsOptions['createImageNodeFromFile']
    connectIntent: CanvasVueFlowConnectionAdapter['connectIntent']
  }
  execution: {
    isNodeExecutionActive: UseCanvasImageToolsOptions['isNodeExecutionActive']
    runInpaintAction: UseCanvasImageToolsOptions['runInpaintAction']
  }
  ui: {
    clearActiveNodePanel: UseCanvasImageToolsOptions['clearActiveNodePanel']
    hideStrengthControl: UseCanvasImageToolsOptions['hideStrengthControl']
    isImageActionPanelVisible: UseCanvasImageToolsOptions['isImageActionPanelVisible']
    isImagePromptComposerVisible: UseCanvasImageToolsOptions['isImagePromptComposerVisible']
    strengthControl: UseCanvasImageToolsOptions['strengthControl']
  }
  feedback: Pick<CanvasImageToolsFeedback, 'warning' | 'succeeded'>
}

export type ImageToolSession = Omit<ReturnType<typeof createImageToolSession>, 'options'>

export function createImageToolSession(options: UseCanvasImageToolsOptions) {
  const imageEditor = useImageEditorMode({
    nodes: options.nodes,
    activeTool: options.activeTool,
    getImageNodeDisplayPayload: options.getImageNodeDisplayPayload,
    onBeforePointerCancel: options.clearSelectedFlowElements,
    onBeforeExit: (state) => confirmImageEditorExit(state),
  })
  const {
    imageEditorMode,
    imageEditorNodeId,
    enterImageEditorMode: enterImageEditorModeBase,
    exitImageEditorMode,
  } = imageEditor

  const coordinates = useImageEditorCoordinateAdapter({
    nodes: options.nodes,
    canvasWrapRef: options.canvasWrapRef,
    imageEditorState: imageEditor.imageEditorState,
    getImageNodeDisplayPayload: options.getImageNodeDisplayPayload,
    getImageStageElement: options.getImageStageElement,
    screenToFlowCoordinate: options.screenToFlowCoordinate,
    getViewport: options.getViewport,
    setViewport: options.setViewport,
    setCenter: options.setCenter,
  })

  const maskEditor = useImageMaskEditorOverlay({
    inpaintEditorState: createImageToolMaskOverlayState(imageEditor.imageEditorState),
    imageRect: coordinates.imageEditorImageRect,
    startCanvasInteraction: options.startCanvasInteraction,
    stopCanvasInteraction: options.stopCanvasInteraction,
  })

  const imageEditorSession = useImageEditorSession()
  const isApplyingImageEditorAdjustments = ref(false)
  const imageAdjustmentFinalApplyProgress = ref<ImageAdjustmentFinalApplyProgress | null>(null)
  const activeImageAdjustmentFinalScopeId = ref<string | null>(null)
  const isImageAdjustmentMode = computed(() => imageEditorMode.value === 'adjust')

  const maskChromeState = useMaskEditorChromeState({
    imageEditorNodeId,
  })

  function cancelImageAdjustmentFinalTask(nodeId: string | null = imageEditorNodeId.value) {
    if (!nodeId) return
    cancelImageAdjustmentFinalRender(nodeId)
    if (activeImageAdjustmentFinalScopeId.value === nodeId) {
      activeImageAdjustmentFinalScopeId.value = null
      isApplyingImageEditorAdjustments.value = false
      imageAdjustmentFinalApplyProgress.value = null
    }
  }

  function confirmImageEditorExit(state: { nodeId: string; mode: ImageEditorMode }) {
    cancelImageAdjustmentFinalTask(state.nodeId)
    return true
  }

  function enter(nodeId: string, mode: ImageEditorMode): boolean {
    const entered = enterImageEditorModeBase(nodeId, mode)
    if (!entered) return false
    if (!coordinates.imageEditorImageRect.value) {
      exitImageEditorMode({ runBeforeExit: false })
      return false
    }
    if (mode === 'adjust') imageEditorSession.resetImageEditorSession()
    return true
  }

  function cancel() {
    exitImageEditorMode()
  }

  function commitMask() {
    const didCommit = maskEditor.commitMaskDraft()
    if (!didCommit) return false
    cancelImageAdjustmentFinalTask()
    exitImageEditorMode({ runBeforeExit: false })
    return true
  }

  function discardNodeEdits(nodeId: string) {
    maskEditor.clearCommittedMaskData(nodeId)
  }

  return {
    capabilities: {
      board: {
        currentBoardId: options.currentBoardId,
        getImageNodeDisplayPayload: options.getImageNodeDisplayPayload,
        getImageNodeSnapshot: options.getImageNodeSnapshot,
        createImageNodeFromFile: options.createImageNodeFromFile,
        connectIntent: options.connectionAdapter.connectIntent,
      },
      execution: {
        isNodeExecutionActive: options.isNodeExecutionActive,
        runInpaintAction: options.runInpaintAction,
      },
      ui: {
        clearActiveNodePanel: options.clearActiveNodePanel,
        hideStrengthControl: options.hideStrengthControl,
        isImageActionPanelVisible: options.isImageActionPanelVisible,
        isImagePromptComposerVisible: options.isImagePromptComposerVisible,
        strengthControl: options.strengthControl,
      },
      feedback: {
        warning: options.feedback.warning,
        succeeded: options.feedback.succeeded,
      },
    } satisfies ImageToolSessionCapabilities,
    imageEditorMode,
    imageEditorNodeId,
    imageEditorSession,
    isImageAdjustmentMode,
    isApplyingImageEditorAdjustments,
    imageAdjustmentFinalApplyProgress,
    activeImageAdjustmentFinalScopeId,
    maskEditor,
    maskChromeState,
    coordinates,
    enter,
    cancel,
    commitMask,
    exitImageEditorMode,
    cancelImageAdjustmentFinalTask,
    discardNodeEdits,
  }
}
