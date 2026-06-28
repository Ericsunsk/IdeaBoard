import { computed, provide, readonly, ref, type ComputedRef, type Ref } from 'vue'
import type { Connection, Node } from '@vue-flow/core'
import type { ImageNodeItem } from '../../canvas-board/domain'
import {
  cancelImageAdjustmentFinalRender,
  flattenVisibleImageEditorAdjustmentLayers,
  hasEffectiveImageEditorAdjustmentLayers,
  IMAGE_ADJUSTMENT_FINAL_TASK_ID,
  renderImageAdjustmentFile,
  useImageAdjustmentFilterPreviews,
  useImageEditorMode,
  useImageEditorSession,
  IMAGE_ADJUSTMENT_FINAL_APPLY_CONTEXT_KEY,
  type ImageAdjustmentKind,
  type ImageAdjustmentFinalApplyProgress,
  type ImageEditorAdjustmentLayer,
  type ImageEditorFilterPreview,
  type ImageEditorLayer,
  type ImageEditorMode,
  type ImageEditorParamKey,
  type ImageEditorParamValue,
  type ImageEditorTool,
  type WorkspaceImageAdjustmentFinalApplyContext,
} from '../image-editor'
import type { ImageNodeDisplayPayload } from '../utils/flowImageNodeSelectors'
import type { CanvasInteractionKey } from './useFlowCanvasInteractionState'
import {
  useImageMaskEditorOverlay,
  type ImageMaskBrushShape,
  type ImageMaskDrawMode,
  type ImageMaskEditorCommittedData,
} from '../mask-editor'

type ValueRef<T> = {
  readonly value: T
}

type WorkspaceImageToolRuntimeMessage = {
  warning: (message: string) => void
  success: (message: string) => void
}

type UseWorkspaceImageToolRuntimeOptions = {
  nodes: Ref<Node[]>
  activeTool: Ref<string>
  currentBoardId: ValueRef<string | null | undefined>
  getImageNodeDisplayPayload: (nodeId: string) => ImageNodeDisplayPayload | null
  getImageNodeSnapshot: (nodeId: string) => ImageNodeItem | null
  getImageStageElement: (nodeId: string) => HTMLElement | null
  createImageNodeFromFile: (
    point: { x: number; y: number },
    file: File,
  ) => Promise<string | null>
  connectImageNodes: (connection: Connection) => void
  clearSelectedFlowElements: () => void
  startCanvasInteraction: (key: CanvasInteractionKey) => void
  stopCanvasInteraction: (key: CanvasInteractionKey) => void
  message: WorkspaceImageToolRuntimeMessage
}

type WorkspaceImageToolRuntime = {
  imageEditorMode: ComputedRef<ImageEditorMode | null>
  flowImageEditorNodeId: ComputedRef<string | null>
  flowMaskEditorNodeId: ComputedRef<string | null>
  flowMaskEditorOverlayStyle: ComputedRef<Record<string, string> | null>
  flowMaskEditorDrawMode: Ref<ImageMaskDrawMode>
  flowMaskEditorBrushSize: Ref<number>
  flowMaskEditorBrushHardness: Ref<number>
  flowMaskEditorBrushShape: Ref<ImageMaskBrushShape>
  canUndoFlowMaskEditor: Ref<boolean>
  canRedoFlowMaskEditor: Ref<boolean>
  clearFlowMaskEditorMask: () => void
  invertFlowMaskEditorMask: () => void
  featherFlowMaskEditorMask: (strength?: number) => void
  undoFlowMaskEditorMask: () => void
  redoFlowMaskEditorMask: () => void
  clearFlowMaskEditorCommittedData: (nodeId: string) => void
  setFlowMaskEditorCanvasRef: (nodeId: string, element: Element | null) => void
  onFlowMaskEditorCanvasPointerDown: (event: PointerEvent) => void
  setFlowMaskEditorDrawMode: (mode: ImageMaskDrawMode) => void
  setFlowMaskEditorBrushSize: (value: number) => void
  setFlowMaskEditorBrushHardness: (value: number) => void
  setFlowMaskEditorBrushShape: (value: ImageMaskBrushShape) => void
  getFlowMaskEditorCommittedData: (nodeId: string) => ImageMaskEditorCommittedData | null
  activeImageEditorTool: Ref<ImageEditorTool>
  imageEditorLayers: Ref<ImageEditorLayer[]>
  selectedImageEditorLayerId: Ref<string>
  selectedImageEditorAdjustmentLayer: ComputedRef<ImageEditorAdjustmentLayer | null>
  selectImageEditorTool: (tool: ImageEditorTool) => void
  selectImageAdjustmentLayer: (kind: ImageAdjustmentKind) => void
  selectImageEditorLayer: (layerId: string) => void
  setImageEditorLayerVisible: (layerId: string, visible: boolean) => void
  addImageEditorLayer: () => void
  copyImageEditorLayer: () => void
  mergeImageEditorLayer: () => void
  removeImageEditorLayer: (layerId: string) => void
  updateSelectedImageEditorLayerParam: (
    key: ImageEditorParamKey,
    value: ImageEditorParamValue,
  ) => void
  isApplyingImageEditorAdjustments: Ref<boolean>
  imageAdjustmentFinalApplyProgress: Ref<ImageAdjustmentFinalApplyProgress | null>
  imageAdjustmentFilterPreviews: ComputedRef<ImageEditorFilterPreview[]>
  onApplyImageEditorAdjustments: () => Promise<void>
  cancelImageEditorAdjustmentFinalApply: () => void
  cancelFlowMaskEditorMode: () => void
  enterFlowImageEditorMode: (nodeId: string, mode: ImageEditorMode) => boolean
  commitFlowMaskEditorMode: () => boolean
}

export function useWorkspaceImageToolRuntime(
  options: UseWorkspaceImageToolRuntimeOptions,
): WorkspaceImageToolRuntime {
  const imageEditor = useImageEditorMode({
    nodes: options.nodes,
    activeTool: options.activeTool,
    getImageNodeDisplayPayload: options.getImageNodeDisplayPayload,
    getImageStageElement: options.getImageStageElement,
    onBeforePointerCancel: options.clearSelectedFlowElements,
    onBeforeExit: (state) => cancelImageAdjustmentFinalTask(state.nodeId),
  })

  const maskEditor = useImageMaskEditorOverlay({
    imageEditorState: imageEditor.imageEditorState,
    startCanvasInteraction: options.startCanvasInteraction,
    stopCanvasInteraction: options.stopCanvasInteraction,
  })

  const {
    imageEditorMode,
    imageEditorNodeId: flowImageEditorNodeId,
    enterImageEditorMode: enterFlowImageEditorModeBase,
    exitImageEditorMode,
  } = imageEditor

  const imageEditorSession = useImageEditorSession()
  const isApplyingImageEditorAdjustments = ref(false)
  const imageAdjustmentFinalApplyProgress = ref<ImageAdjustmentFinalApplyProgress | null>(null)
  const activeImageAdjustmentFinalTaskId = ref<string | null>(null)
  const isImageAdjustmentMode = computed(() => imageEditorMode.value === 'adjust')
  const { imageAdjustmentFilterPreviews } = useImageAdjustmentFilterPreviews({
    isAdjustMode: isImageAdjustmentMode,
    layers: imageEditorSession.imageEditorLayers,
    sourceUrl: () =>
      flowImageEditorNodeId.value
        ? options.getImageNodeDisplayPayload(flowImageEditorNodeId.value)?.url
        : null,
  })

  function getImageAdjustmentFinalTaskId(nodeId: string) {
    return `${IMAGE_ADJUSTMENT_FINAL_TASK_ID}:${nodeId}`
  }

  function cancelImageAdjustmentFinalTask(nodeId: string | null = flowImageEditorNodeId.value) {
    if (!nodeId) return
    const taskId = getImageAdjustmentFinalTaskId(nodeId)
    cancelImageAdjustmentFinalRender(taskId)
    if (activeImageAdjustmentFinalTaskId.value === taskId) {
      activeImageAdjustmentFinalTaskId.value = null
      isApplyingImageEditorAdjustments.value = false
      imageAdjustmentFinalApplyProgress.value = null
    }
  }

  provide<WorkspaceImageAdjustmentFinalApplyContext>(
    IMAGE_ADJUSTMENT_FINAL_APPLY_CONTEXT_KEY,
    {
      progress: readonly(imageAdjustmentFinalApplyProgress),
      cancel: () => cancelImageAdjustmentFinalTask(),
    },
  )

  async function onApplyImageEditorAdjustments() {
    if (isApplyingImageEditorAdjustments.value) return

    const nodeId = flowImageEditorNodeId.value
    if (!nodeId || imageEditorMode.value !== 'adjust') return

    const boardId = options.currentBoardId.value
    const source = options.getImageNodeDisplayPayload(nodeId)
    const sourceNode = options.getImageNodeSnapshot(nodeId)
    if (!source || !sourceNode) {
      options.message.warning('当前图片不可编辑')
      return
    }

    const visibleAdjustmentLayers = flattenVisibleImageEditorAdjustmentLayers(
      imageEditorSession.imageEditorLayers.value,
    )
    if (!hasEffectiveImageEditorAdjustmentLayers(imageEditorSession.imageEditorLayers.value)) {
      options.message.warning('没有可应用的编辑图层')
      return
    }

    isApplyingImageEditorAdjustments.value = true
    imageAdjustmentFinalApplyProgress.value = { phase: 'queued' }
    const taskId = getImageAdjustmentFinalTaskId(nodeId)
    activeImageAdjustmentFinalTaskId.value = taskId
    try {
      const renderedFile = await renderImageAdjustmentFile(
        { url: source.url, title: sourceNode.title || 'Image' },
        visibleAdjustmentLayers,
        {
          taskId,
          latestOnly: true,
          onProgress: (progress) => {
            if (activeImageAdjustmentFinalTaskId.value !== taskId) return
            imageAdjustmentFinalApplyProgress.value = {
              phase: progress.phase,
              ...(progress.loaded === undefined ? {} : { loaded: progress.loaded }),
              ...(progress.total === undefined ? {} : { total: progress.total }),
            }
          },
        },
      )
      if (options.currentBoardId.value !== boardId || flowImageEditorNodeId.value !== nodeId)
        return
      const newNodeId = await options.createImageNodeFromFile(
        {
          x: Number(sourceNode.x ?? 0) + Math.max(240, Number(sourceNode.width ?? 300) + 64),
          y: Number(sourceNode.y ?? 0) + 18,
        },
        renderedFile,
      )
      if (!newNodeId) return
      options.connectImageNodes({
        source: nodeId,
        sourceHandle: 'out:image',
        target: newNodeId,
        targetHandle: 'in:image',
      })
      exitImageEditorMode()
      options.message.success('已应用编辑并派生新节点')
    } catch (error) {
      if (
        isImageAdjustmentFinalApplyCancelled(error) ||
        options.currentBoardId.value !== boardId ||
        flowImageEditorNodeId.value !== nodeId ||
        activeImageAdjustmentFinalTaskId.value !== taskId
      ) {
        return
      }
      options.message.warning(error instanceof Error ? error.message : '应用编辑失败')
    } finally {
      if (activeImageAdjustmentFinalTaskId.value === taskId) {
        activeImageAdjustmentFinalTaskId.value = null
        isApplyingImageEditorAdjustments.value = false
        imageAdjustmentFinalApplyProgress.value = null
      }
    }
  }

  function cancelFlowMaskEditorMode() {
    exitImageEditorMode()
  }

  function enterFlowImageEditorMode(nodeId: string, mode: ImageEditorMode): boolean {
    const entered = enterFlowImageEditorModeBase(nodeId, mode)
    if (!entered) return false
    if (mode === 'adjust') imageEditorSession.resetImageEditorSession()
    return true
  }

  function commitFlowMaskEditorMode() {
    const didCommit = maskEditor.commitMaskDraft()
    if (!didCommit) return false
    cancelImageAdjustmentFinalTask()
    exitImageEditorMode({ runBeforeExit: false })
    return true
  }

  return {
    imageEditorMode,
    flowImageEditorNodeId,
    flowMaskEditorNodeId: maskEditor.maskEditorNodeId,
    flowMaskEditorOverlayStyle: maskEditor.maskEditorOverlayStyle,
    flowMaskEditorDrawMode: maskEditor.drawMode,
    flowMaskEditorBrushSize: maskEditor.brushSize,
    flowMaskEditorBrushHardness: maskEditor.brushHardness,
    flowMaskEditorBrushShape: maskEditor.brushShape,
    canUndoFlowMaskEditor: maskEditor.canUndoMask,
    canRedoFlowMaskEditor: maskEditor.canRedoMask,
    clearFlowMaskEditorMask: maskEditor.clearMask,
    invertFlowMaskEditorMask: maskEditor.invertMask,
    featherFlowMaskEditorMask: maskEditor.featherMask,
    undoFlowMaskEditorMask: maskEditor.undoMask,
    redoFlowMaskEditorMask: maskEditor.redoMask,
    clearFlowMaskEditorCommittedData: maskEditor.clearCommittedMaskData,
    setFlowMaskEditorCanvasRef: maskEditor.setMaskCanvasRef,
    onFlowMaskEditorCanvasPointerDown: maskEditor.onCanvasPointerDown,
    setFlowMaskEditorDrawMode: maskEditor.setDrawMode,
    setFlowMaskEditorBrushSize: maskEditor.setBrushSize,
    setFlowMaskEditorBrushHardness: maskEditor.setBrushHardness,
    setFlowMaskEditorBrushShape: maskEditor.setBrushShape,
    getFlowMaskEditorCommittedData: maskEditor.getCommittedMaskData,
    activeImageEditorTool: imageEditorSession.activeImageEditorTool,
    imageEditorLayers: imageEditorSession.imageEditorLayers,
    selectedImageEditorLayerId: imageEditorSession.selectedImageEditorLayerId,
    selectedImageEditorAdjustmentLayer: imageEditorSession.selectedImageEditorAdjustmentLayer,
    selectImageEditorTool: imageEditorSession.selectImageEditorTool,
    selectImageAdjustmentLayer: imageEditorSession.selectImageAdjustmentLayer,
    selectImageEditorLayer: imageEditorSession.selectImageEditorLayer,
    setImageEditorLayerVisible: imageEditorSession.setImageEditorLayerVisible,
    addImageEditorLayer: imageEditorSession.addImageEditorLayer,
    copyImageEditorLayer: imageEditorSession.copyImageEditorLayer,
    mergeImageEditorLayer: imageEditorSession.mergeImageEditorLayer,
    removeImageEditorLayer: imageEditorSession.removeImageEditorLayer,
    updateSelectedImageEditorLayerParam: imageEditorSession.updateSelectedImageEditorLayerParam,
    isApplyingImageEditorAdjustments,
    imageAdjustmentFinalApplyProgress,
    imageAdjustmentFilterPreviews,
    onApplyImageEditorAdjustments,
    cancelImageEditorAdjustmentFinalApply: () => cancelImageAdjustmentFinalTask(),
    cancelFlowMaskEditorMode,
    enterFlowImageEditorMode,
    commitFlowMaskEditorMode,
  }
}

function isImageAdjustmentFinalApplyCancelled(error: unknown) {
  if (!(error instanceof Error)) return false
  const message = error.message.toLowerCase()
  return message === 'cancelled' || message.includes('已取消')
}
