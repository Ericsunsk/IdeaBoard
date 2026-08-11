import { useEventListener } from '@vueuse/core'
import { computed, ref } from 'vue'
import type { Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import type { ImageNodeItem } from '../../canvas-board/domain'
import type { CanvasInteractionKey } from '../composables/useFlowCanvasInteractionState'
import type { CanvasFlowPaneConnectionAdapter } from '../flow-pane'
import type { CanvasGraphNodeIdForKind } from '../graph-runtime'
import { useImageEditorMode } from '../image-editor/composables/useImageEditorMode'
import { useImageEditorRasterOverlay } from '../image-editor/composables/useImageEditorRasterOverlay'
import { useImageEditorSession } from '../image-editor/composables/useImageEditorSession'
import { useImageEditorCoordinateAdapter } from '../image-editor'
import type { ImageAdjustmentFinalApplyProgress } from '../image-editor/runtime/imageAdjustmentFinalApplyContext'
import { cancelImageAdjustmentFinalRender } from '../image-editor/services/imageAdjustmentRenderer'
import type { ImageEditorMode } from '../image-editor/types'
import { hasImageEditorRasterEdits } from '../image-editor/utils/imageEditorRasterTools'
import { useImageMaskEditorOverlay } from '../mask-editor/composables/useImageMaskEditorOverlay'
import { useMaskEditorChromeState } from '../mask-editor/composables/useMaskEditorChromeState'
import type { ImageNodeDisplayPayload } from '../utils/flowImageNodeSelectors'
import type { FlowSnapshotParamPatch } from '../utils/flowSnapshotParamMutations'
import type { CanvasFeedback } from '../feedback/useCanvasFeedback'
import { isEditableEventTarget } from '../utils/domEventTargets'
import { createImageToolMaskOverlayState } from './maskOverlayAdapter'
import type { ImageActionStrengthControlSource } from './imageToolBindings'

type ValueRef<T> = {
  readonly value: T
}

type CanvasImageToolsFeedback = CanvasFeedback['imageTools']

const windowTarget = () => (typeof window === 'undefined' ? null : window)

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
  connectionAdapter: Pick<CanvasFlowPaneConnectionAdapter, 'connectIntent'>
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
  isGenerationAvailable: () => boolean
  isNodeExecutionActive: (nodeId: string) => boolean
  strengthControl: ImageActionStrengthControlSource
  clearSelectedFlowElements: () => void
  startCanvasInteraction: (key: CanvasInteractionKey) => void
  stopCanvasInteraction: (key: CanvasInteractionKey) => void
  feedback: CanvasImageToolsFeedback
}

export type ImageToolSession = ReturnType<typeof createImageToolSession>

export function createImageToolSession(options: UseCanvasImageToolsOptions) {
  let requestApplyImageEditorAdjustments: (() => void) | null = null
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
  const imageEditorRasterOverlay = useImageEditorRasterOverlay({
    editorState: imageEditor.imageEditorState,
    imageRect: coordinates.imageEditorImageRect,
    sourceUrl: computed(() =>
      imageEditorNodeId.value
        ? (options.getImageNodeDisplayPayload(imageEditorNodeId.value)?.url ?? null)
        : null,
    ),
    layers: imageEditorSession.imageEditorLayers,
    selectedLayerId: imageEditorSession.selectedImageEditorLayerId,
    activeTool: imageEditorSession.activeImageEditorTool,
    settings: imageEditorSession.imageEditorRasterToolSettings,
    captureHistorySnapshot: imageEditorSession.captureImageEditorRasterHistorySnapshot,
    commitRasterStroke: imageEditorSession.commitImageEditorRasterStroke,
    startCanvasInteraction: options.startCanvasInteraction,
    stopCanvasInteraction: options.stopCanvasInteraction,
  })
  const isApplyingImageEditorAdjustments = ref(false)
  const imageAdjustmentFinalApplyProgress = ref<ImageAdjustmentFinalApplyProgress | null>(null)
  const activeImageAdjustmentFinalScopeId = ref<string | null>(null)
  const isImageAdjustmentMode = computed(() => imageEditorMode.value === 'adjust')

  const maskChromeState = useMaskEditorChromeState({
    imageEditorNodeId,
    getImageNodeSnapshot: options.getImageNodeSnapshot,
    uploadImageToBoard: options.uploadImageToBoard,
    onWarning: options.feedback.warning,
    onReferenceImagePreview: (image) => {
      void options.openImageUrlPreview(image.url, image.ratio)
    },
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

  function setImageEditorApplyRequest(handler: () => void) {
    requestApplyImageEditorAdjustments = handler
  }

  function confirmImageEditorExit(state: { nodeId: string; mode: ImageEditorMode }) {
    if (state.mode !== 'adjust') {
      cancelImageAdjustmentFinalTask(state.nodeId)
      return true
    }

    const hasRasterEdits = hasImageEditorRasterEdits(imageEditorSession.imageEditorLayers.value)
    if (!hasRasterEdits) {
      imageEditorSession.clearImageEditorRasterHistory()
      cancelImageAdjustmentFinalTask(state.nodeId)
      return true
    }

    if (isApplyingImageEditorAdjustments.value) {
      options.feedback.warning('正在应用编辑，请稍候')
      return false
    }

    const shouldApply = window.confirm(
      '有未应用的画笔/橡皮擦编辑。\n\n确定：应用并生成新节点\n取消：继续选择丢弃或返回编辑',
    )
    if (shouldApply) {
      requestApplyImageEditorAdjustments?.()
      return false
    }

    const shouldDiscard = window.confirm(
      '是否丢弃这些未应用的编辑并退出？\n\n确定：丢弃并退出\n取消：返回编辑',
    )
    if (!shouldDiscard) return false

    imageEditorSession.clearImageEditorRasterEdits()
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
    if (imageEditorNodeId.value === nodeId) imageEditorSession.clearImageEditorRasterEdits()
  }

  function onGlobalKeydown(event: KeyboardEvent) {
    if (imageEditorMode.value !== 'adjust') return
    if (isEditableEventTarget(event.target)) return
    const key = event.key.toLowerCase()
    const hasModifier = event.metaKey || event.ctrlKey
    if (!hasModifier) return

    if ((key === 'z' && event.shiftKey) || key === 'y') {
      event.preventDefault()
      imageEditorSession.redoImageEditorRasterStroke()
      return
    }
    if (key === 'z') {
      event.preventDefault()
      imageEditorSession.undoImageEditorRasterStroke()
    }
  }

  useEventListener<KeyboardEvent>(windowTarget, 'keydown', onGlobalKeydown, true)

  return {
    options,
    imageEditorMode,
    imageEditorNodeId,
    imageEditorSession,
    imageEditorRasterOverlay,
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
    setImageEditorApplyRequest,
    discardNodeEdits,
  }
}
