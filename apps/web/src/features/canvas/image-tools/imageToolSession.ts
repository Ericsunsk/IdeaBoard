import { computed, ref } from 'vue'
import type { Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import type { ImageNodeItem } from '../../canvas-board/domain'
import type { CanvasInteractionKey } from '../workspace-ui/useFlowCanvasInteractionState'
import type { CanvasVueFlowConnectionAdapter } from '../vue-flow-adapter'
import type { CanvasNodeItem } from '../../canvas-board/domain'
import { useRetouchMode } from '../retouch/useRetouchMode'
import { useRetouchSession } from '../retouch/useRetouchSession'
import { useRetouchCoordinateAdapter } from '../retouch'
import { cancelImageRetouchFinalRender } from '../retouch/services/imageRetouchRenderer'
import type { RetouchMode } from '../retouch/types'
import { useImageInpaintOverlay } from '../inpaint/useImageInpaintOverlay'
import { useInpaintChromeState } from '../inpaint/useInpaintChromeState'
import type { ImageNodeDisplayPayload } from '../utils/flowImageNodeSelectors'
import type { CanvasFeedback } from '../feedback/useCanvasFeedback'
import { createImageToolMaskOverlayState } from './maskOverlayAdapter'
import type { ImageActionStrengthControlSource } from './imageToolTypes'

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
  updateSnapshotNode?:
    ((nodeId: string, patcher: (previous: CanvasNodeItem) => CanvasNodeItem) => void) | undefined
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
  createPendingRetouchNode: (sourceNode: ImageNodeItem) => string | null
  applyImageFileToImageNode: (nodeId: string, file: File) => Promise<boolean>
  removeNode: (nodeId: string) => void
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
    updateSnapshotNode?: UseCanvasImageToolsOptions['updateSnapshotNode']
    createPendingRetouchNode: UseCanvasImageToolsOptions['createPendingRetouchNode']
    applyImageFileToImageNode: UseCanvasImageToolsOptions['applyImageFileToImageNode']
    removeNode: UseCanvasImageToolsOptions['removeNode']
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
  const retouch = useRetouchMode({
    nodes: options.nodes,
    activeTool: options.activeTool,
    getImageNodeDisplayPayload: options.getImageNodeDisplayPayload,
    onBeforePointerCancel: options.clearSelectedFlowElements,
    onBeforeExit: (state) => confirmRetouchExit(state),
  })
  const {
    retouchMode,
    retouchNodeId,
    enterRetouchMode: enterRetouchModeBase,
    exitRetouchMode,
  } = retouch

  const coordinates = useRetouchCoordinateAdapter({
    nodes: options.nodes,
    canvasWrapRef: options.canvasWrapRef,
    retouchState: retouch.retouchState,
    getImageNodeDisplayPayload: options.getImageNodeDisplayPayload,
    getImageStageElement: options.getImageStageElement,
    screenToFlowCoordinate: options.screenToFlowCoordinate,
    getViewport: options.getViewport,
    setViewport: options.setViewport,
    setCenter: options.setCenter,
  })

  const inpaint = useImageInpaintOverlay({
    inpaintEditorState: createImageToolMaskOverlayState(retouch.retouchState),
    imageRect: coordinates.retouchImageRect,
    startCanvasInteraction: options.startCanvasInteraction,
    stopCanvasInteraction: options.stopCanvasInteraction,
  })

  const retouchSession = useRetouchSession()
  const isApplyingRetouch = ref(false)
  const activeImageRetouchFinalScopeId = ref<string | null>(null)
  const activeImageRetouchPendingNodeId = ref<string | null>(null)
  let imageRetouchFinalApplyCleanup: (() => void) | null = null
  const isImageRetouchMode = computed(() => retouchMode.value === 'adjust')

  const maskChromeState = useInpaintChromeState({
    retouchNodeId,
  })

  function cancelImageRetouchFinalTask(
    nodeId: string | null = retouchNodeId.value ?? activeImageRetouchFinalScopeId.value,
  ) {
    if (!nodeId) return
    cancelImageRetouchFinalRender(nodeId)
    if (activeImageRetouchFinalScopeId.value === nodeId) {
      imageRetouchFinalApplyCleanup?.()
      imageRetouchFinalApplyCleanup = null
      activeImageRetouchFinalScopeId.value = null
      activeImageRetouchPendingNodeId.value = null
      isApplyingRetouch.value = false
    }
  }

  function registerImageRetouchFinalApplyCleanup(cleanup: (() => void) | null) {
    imageRetouchFinalApplyCleanup?.()
    imageRetouchFinalApplyCleanup = cleanup
  }

  function confirmRetouchExit(state: { nodeId: string; mode: RetouchMode }) {
    cancelImageRetouchFinalTask(state.nodeId)
    return true
  }

  function enter(nodeId: string, mode: RetouchMode): boolean {
    const entered = enterRetouchModeBase(nodeId, mode)
    if (!entered) return false
    if (!coordinates.retouchImageRect.value) {
      exitRetouchMode({ runBeforeExit: false })
      return false
    }
    if (mode === 'adjust') retouchSession.resetRetouchSession()
    return true
  }

  function cancel() {
    exitRetouchMode()
  }

  function commitMask() {
    const didCommit = inpaint.commitMaskDraft()
    if (!didCommit) return false
    cancelImageRetouchFinalTask()
    exitRetouchMode({ runBeforeExit: false })
    return true
  }

  function discardNodeEdits(nodeId: string) {
    inpaint.clearCommittedMaskData(nodeId)
  }

  return {
    capabilities: {
      board: {
        currentBoardId: options.currentBoardId,
        getImageNodeDisplayPayload: options.getImageNodeDisplayPayload,
        getImageNodeSnapshot: options.getImageNodeSnapshot,
        updateSnapshotNode: options.updateSnapshotNode,
        createPendingRetouchNode: options.createPendingRetouchNode,
        applyImageFileToImageNode: options.applyImageFileToImageNode,
        removeNode: options.removeNode,
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
    retouchMode,
    retouchNodeId,
    retouchSession,
    isImageRetouchMode,
    isApplyingRetouch,
    activeImageRetouchPendingNodeId,
    activeImageRetouchFinalScopeId,
    inpaint,
    maskChromeState,
    coordinates,
    enter,
    cancel,
    commitMask,
    exitRetouchMode,
    cancelImageRetouchFinalTask,
    registerImageRetouchFinalApplyCleanup,
    discardNodeEdits,
  }
}
