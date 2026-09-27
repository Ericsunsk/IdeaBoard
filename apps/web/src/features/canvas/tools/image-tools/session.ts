import { computed } from 'vue'
import type { Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import type { ImageNodeItem } from '../../../board'
import type { CanvasInteractionKey } from '../../presentation/screens/useFlowCanvasInteractionState.ts'
import type {
  CanvasFlowViewportControls,
  CanvasVueFlowConnectionAdapter,
} from '../../infrastructure/vue-flow-adapter'
import type { CanvasNodeItem } from '../../../board'
import { useRetouchMode } from '../retouch/useRetouchMode'
import { useRetouchSession } from '../retouch/useRetouchSession'
import { useRetouchCoordinateAdapter } from '../retouch'
import { cancelImageRetouchFinalRender } from '../retouch/rendering/renderer'
import type { RetouchMode } from '../retouch/model.ts'
import { useImageInpaintOverlay } from '../inpaint/useImageInpaintOverlay'
import { useInpaintChromeState } from '../inpaint/useInpaintChromeState'
import type { ImageNodeDisplayPayload } from '../../infrastructure/vue-flow-adapter/flowImageNodeSelectors.ts'
import type { CanvasFeedback } from '../../application/workspace/useCanvasFeedback.ts'
import { createImageToolMaskOverlayState } from './maskOverlayAdapter'
import { createImageToolApplyScopeHost } from './applyScope'
import type { ImageActionStrengthControlSource } from './model.ts'

type ValueRef<T> = {
  readonly value: T
}

type CanvasImageToolsFeedback = CanvasFeedback['imageTools']

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
} & CanvasFlowViewportControls

export type ImageToolSessionCapabilities = {
  board: {
    currentBoardId: ValueRef<string | null | undefined>
    getImageNodeDisplayPayload: UseCanvasImageToolsOptions['getImageNodeDisplayPayload']
    getImageNodeSnapshot: UseCanvasImageToolsOptions['getImageNodeSnapshot']
    getImageStageElement: UseCanvasImageToolsOptions['getImageStageElement']
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
  const applyScope = createImageToolApplyScopeHost({
    currentBoardId: () => options.currentBoardId.value,
    isNodeAlive: (nodeId) => options.getImageNodeSnapshot(nodeId) != null,
  })
  const isImageRetouchMode = computed(() => retouchMode.value === 'adjust')

  const maskChromeState = useInpaintChromeState({
    retouchNodeId,
  })

  function cancelImageRetouchFinalTask(
    nodeId: string | null = retouchNodeId.value ?? applyScope.scopeId.value,
  ) {
    if (!nodeId) return
    cancelImageRetouchFinalRender(nodeId)
    applyScope.cancel(nodeId)
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
    if (mode === 'adjust') {
      // Non-destructive editing (issue #671): re-entering Retouch on a node
      // with a persisted stack reconstructs sliders/curves exactly.
      const snapshot = options.getImageNodeSnapshot(nodeId)
      if (!retouchSession.restoreRetouchStack(snapshot?.retouchStack)) {
        retouchSession.resetRetouchSession()
      }
    }
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
        getImageStageElement: options.getImageStageElement,
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
    applyScope,
    inpaint,
    maskChromeState,
    coordinates,
    enter,
    cancel,
    commitMask,
    exitRetouchMode,
    discardNodeEdits,
  }
}
