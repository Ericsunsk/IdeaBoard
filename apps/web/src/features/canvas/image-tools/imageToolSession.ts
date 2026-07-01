import { computed, ref } from 'vue'
import type { Connection, Node } from '@vue-flow/core'
import type { ComponentPublicInstance, Ref } from 'vue'
import type { ImageNodeItem } from '../../canvas-board/domain'
import type { CanvasInteractionKey } from '../composables/useFlowCanvasInteractionState'
import type { ImageStrengthAction } from '../execution'
import {
  cancelImageAdjustmentFinalRender,
  type ImageAdjustmentFinalApplyProgress,
  type ImageEditorMode,
  useImageEditorMode,
  useImageEditorSession,
  useImageEditorViewportController,
} from '../image-editor'
import { useImageMaskEditorOverlay, useMaskEditorChromeState } from '../mask-editor'
import type { ImageNodeDisplayPayload } from '../utils/flowImageNodeSelectors'
import type { FlowSnapshotParamPatch } from '../utils/flowSnapshotParamMutations'
import { createImageToolMaskOverlayState } from './maskOverlayAdapter'

type ValueRef<T> = {
  readonly value: T
}

type CanvasImageToolsMessage = {
  warning: (message: string) => void
  success: (message: string) => void
}

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
  createImageNodeFromFile: (point: { x: number; y: number }, file: File) => Promise<string | null>
  connectImageNodes: (connection: Connection) => void
  uploadImageToBoard: (file: File) => Promise<{ url: string; ratio: number }>
  openImageUrlPreview: (url: string, ratio: number) => void | Promise<void>
  clearActiveNodePanel: () => void
  hideStrengthControl: () => void
  updateOutputCount: (
    nodeId: string,
    payload: Extract<FlowSnapshotParamPatch, { key: 'outputCount' }>,
  ) => void
  runInpaintAction: (
    nodeId: string,
    options: {
      prompt: string
      outputCount: number
      maskDataUrl: string
      referenceImageUrls?: string[]
    },
  ) => Promise<void> | void
  runPromptGeneration: (nodeId: string, prompt: string, outputCount: number) => Promise<void> | void
  isImageActionPanelVisible: (nodeId: string) => boolean
  isImagePromptComposerVisible: (nodeId: string) => boolean
  isDirectActionRunning: (nodeId: string) => boolean
  activeStrengthAction: ValueRef<ImageStrengthAction | null>
  getActionStrength: (action: ImageStrengthAction) => number
  setStrengthRangeRef: (
    action: ImageStrengthAction,
    el: Element | ComponentPublicInstance | null,
  ) => void
  onStrengthRangePointerDown: (action: ImageStrengthAction, event: PointerEvent) => void
  onStrengthInput: (action: ImageStrengthAction, event: Event) => void
  onStrengthChange: (action: ImageStrengthAction, event: Event) => void
  onStrengthPointerUp: (action: ImageStrengthAction, event: PointerEvent) => void
  onStrengthClick: (action: ImageStrengthAction, event: MouseEvent) => void
  clearSelectedFlowElements: () => void
  startCanvasInteraction: (key: CanvasInteractionKey) => void
  stopCanvasInteraction: (key: CanvasInteractionKey) => void
  message: CanvasImageToolsMessage
}

export type ImageToolSession = ReturnType<typeof createImageToolSession>

export function createImageToolSession(options: UseCanvasImageToolsOptions) {
  const imageEditor = useImageEditorMode({
    nodes: options.nodes,
    activeTool: options.activeTool,
    getImageNodeDisplayPayload: options.getImageNodeDisplayPayload,
    getImageStageElement: options.getImageStageElement,
    onBeforePointerCancel: options.clearSelectedFlowElements,
    onBeforeExit: (state) => cancelImageAdjustmentFinalTask(state.nodeId),
  })
  const {
    imageEditorMode,
    imageEditorNodeId,
    enterImageEditorMode: enterImageEditorModeBase,
    exitImageEditorMode,
  } = imageEditor

  const maskEditor = useImageMaskEditorOverlay({
    inpaintEditorState: createImageToolMaskOverlayState(imageEditor.imageEditorState),
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
    getImageNodeSnapshot: options.getImageNodeSnapshot,
    uploadImageToBoard: options.uploadImageToBoard,
    onWarning: options.message.warning,
    onReferenceImagePreview: (image) => {
      void options.openImageUrlPreview(image.url, image.ratio)
    },
  })

  const viewport = useImageEditorViewportController({
    nodes: options.nodes,
    canvasWrapRef: options.canvasWrapRef,
    imageEditorNodeId,
    getImageStageElement: options.getImageStageElement,
    screenToFlowCoordinate: options.screenToFlowCoordinate,
    getViewport: options.getViewport,
    setViewport: options.setViewport,
    setCenter: options.setCenter,
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

  function enter(nodeId: string, mode: ImageEditorMode): boolean {
    const entered = enterImageEditorModeBase(nodeId, mode)
    if (!entered) return false
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
    options,
    imageEditorMode,
    imageEditorNodeId,
    imageEditorSession,
    isImageAdjustmentMode,
    isApplyingImageEditorAdjustments,
    imageAdjustmentFinalApplyProgress,
    activeImageAdjustmentFinalScopeId,
    maskEditor,
    maskChromeState,
    viewport,
    enter,
    cancel,
    commitMask,
    exitImageEditorMode,
    cancelImageAdjustmentFinalTask,
    discardNodeEdits,
  }
}
