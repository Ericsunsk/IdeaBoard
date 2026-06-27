import { computed, ref, type ComputedRef, type Ref } from 'vue'
import type { Connection, Node } from '@vue-flow/core'
import type { ImageNodeItem } from '../../canvas-board/domain'
import {
  flattenVisibleImageEditorAdjustmentLayers,
  renderImageAdjustmentFile,
  useImageAdjustmentFilterPreviews,
  useImageEditorMode,
  useImageEditorSession,
  type ImageAdjustmentKind,
  type ImageEditorAdjustmentLayer,
  type ImageEditorFilterPreview,
  type ImageEditorLayer,
  type ImageEditorMode,
  type ImageEditorParamKey,
  type ImageEditorParamValue,
  type ImageEditorTool,
} from '../image-editor'
import type { ImageNodeDisplayPayload } from '../utils/flowImageNodeSelectors'
import type { CanvasInteractionKey } from './useFlowCanvasInteractionState'
import {
  useImageMaskEditorOverlay,
  type ImageMaskBrushShape,
  type ImageMaskDrawMode,
  type ImageMaskEditorCommittedData,
} from './useImageMaskEditorOverlay'

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
  imageAdjustmentFilterPreviews: ComputedRef<ImageEditorFilterPreview[]>
  onApplyImageEditorAdjustments: () => Promise<void>
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
  const isImageAdjustmentMode = computed(() => imageEditorMode.value === 'adjust')
  const { imageAdjustmentFilterPreviews } = useImageAdjustmentFilterPreviews({
    isAdjustMode: isImageAdjustmentMode,
    layers: imageEditorSession.imageEditorLayers,
  })

  async function onApplyImageEditorAdjustments() {
    if (isApplyingImageEditorAdjustments.value) return

    const nodeId = flowImageEditorNodeId.value
    if (!nodeId || imageEditorMode.value !== 'adjust') return

    const boardId = options.currentBoardId.value
    const source = options.getImageNodeDisplayPayload(nodeId)
    const sourceNode = options.getImageNodeSnapshot(nodeId)
    if (!source || !sourceNode) {
      options.message.warning('当前图片不可调整')
      return
    }

    const visibleAdjustmentLayers = flattenVisibleImageEditorAdjustmentLayers(
      imageEditorSession.imageEditorLayers.value,
    )
    if (visibleAdjustmentLayers.length === 0) {
      options.message.warning('没有可应用的调整图层')
      return
    }

    isApplyingImageEditorAdjustments.value = true
    try {
      const renderedFile = await renderImageAdjustmentFile(
        { url: source.url, title: sourceNode.title || 'Image' },
        visibleAdjustmentLayers,
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
      options.message.success('已应用调整并派生新节点')
    } catch (error) {
      options.message.warning(error instanceof Error ? error.message : '应用调整失败')
    } finally {
      isApplyingImageEditorAdjustments.value = false
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
    imageAdjustmentFilterPreviews,
    onApplyImageEditorAdjustments,
    cancelFlowMaskEditorMode,
    enterFlowImageEditorMode,
    commitFlowMaskEditorMode,
  }
}
