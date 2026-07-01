import type { ComponentPublicInstance, ComputedRef, Ref } from 'vue'
import type {
  ImageAdjustmentKind,
  ImageAdjustmentFinalApplyProgress,
  ImageEditorAdjustmentLayer,
  ImageEditorAdjustmentWholeParams,
  ImageEditorLayer,
  ImageEditorMode,
  ImageEditorParamKey,
  ImageEditorParamValue,
  ImageEditorRenderedPreview,
  ImageEditorTool,
} from '../image-editor'
import type {
  ImageMaskBrushShape,
  ImageMaskDrawMode,
  MaskEditorReferenceImage,
} from '../mask-editor'
import type { ImageStrengthAction } from '../execution'

export type ImageToolAnchorRect = {
  top: number
  left: number
  width: number
  height: number
}

export type ImageToolStageBinding = {
  mode: ImageEditorMode | null
  renderedPreview: ImageEditorRenderedPreview | null
  maskOverlayStyle: Record<string, string> | null
  maskCanvasWidth: number
  maskCanvasHeight: number
  maskLayerOpacity: number
  imageLayerVisible: boolean
  maskBrushSize: number
  maskBrushShape: ImageMaskBrushShape
  setMaskCanvasRef: (element: Element | null) => void
  onMaskCanvasPointerDown: (event: PointerEvent) => void
}

export type ImageActionBinding = {
  actionMenuVisible: boolean
  promptComposerVisible: boolean
  promptPlaceholder: string
  directGenerating: boolean
  showStrengthControl: boolean
  activeStrengthAction: ImageStrengthAction | null
  getStrengthValue: (action: ImageStrengthAction) => number
  setStrengthRangeRef: (
    action: ImageStrengthAction,
    el: Element | ComponentPublicInstance | null,
  ) => void
  onStrengthRangePointerDown: (action: ImageStrengthAction, event: PointerEvent) => void
  onStrengthInput: (action: ImageStrengthAction, event: Event) => void
  onStrengthChange: (action: ImageStrengthAction, event: Event) => void
  onStrengthPointerUp: (action: ImageStrengthAction, event: PointerEvent) => void
  onStrengthClick: (action: ImageStrengthAction, event: MouseEvent) => void
}

export type ImageToolChromeModel = {
  nodeId: string | null
  mode: ImageEditorMode | null
  anchorRect: ImageToolAnchorRect | null
  isReady: boolean
  mask: {
    drawMode: ImageMaskDrawMode
    brushShape: ImageMaskBrushShape
    brushSize: number
    brushHardness: number
    layerOpacity: number
    outputCount: number
    referenceImages: MaskEditorReferenceImage[]
    directGenerating: boolean
    canUndo: boolean
    canRedo: boolean
  }
  editor: {
    activeTool: ImageEditorTool
    layers: ImageEditorLayer[]
    selectedLayerId: string
    selectedAdjustmentLayer: ImageEditorAdjustmentLayer | null
    histogramSource: ImageData | null
    applyingAdjustments: boolean
  }
}

export type ImageToolChromeCommands = {
  setMaskDrawMode: (mode: ImageMaskDrawMode) => void
  setMaskBrushShape: (shape: ImageMaskBrushShape) => void
  setMaskBrushSize: (size: number) => void
  setMaskBrushHardness: (hardness: number) => void
  setMaskLayerOpacity: (opacity: number) => void
  clearMask: () => void
  invertMask: () => void
  featherMask: (strength?: number) => void
  undoMask: () => void
  redoMask: () => void
  updateMaskOutputCount: (count: number) => void
  openMaskReferenceImageInput: () => void
  removeMaskReferenceImage: (imageId: string) => void
  previewMaskReferenceImage: (image: MaskEditorReferenceImage) => void
  submitMaskPrompt: (prompt: string, outputCount: number) => void
  selectEditorTool: (tool: ImageEditorTool) => void
  selectAdjustmentLayer: (kind: ImageAdjustmentKind) => void
  selectEditorLayer: (layerId: string) => void
  setEditorLayerVisible: (layerId: string, visible: boolean) => void
  addEditorLayer: () => void
  copyEditorLayer: () => void
  mergeEditorLayer: () => void
  removeEditorLayer: (layerId: string) => void
  updateEditorParam: (key: ImageEditorParamKey, value: ImageEditorParamValue) => void
  updateEditorAdjustmentParams: (params: ImageEditorAdjustmentWholeParams) => void
  applyAdjustments: () => Promise<void>
}

export type CanvasImageTools = {
  activeNodeId: ComputedRef<string | null>
  activeMaskNodeId: ComputedRef<string | null>
  activeMode: ComputedRef<ImageEditorMode | null>
  chromeModel: ComputedRef<ImageToolChromeModel>
  chromeCommands: ImageToolChromeCommands
  isChromeHidden: ComputedRef<boolean>
  isCanvasLocked: ComputedRef<boolean>
  isApplyingAdjustments: Ref<boolean>
  adjustmentFinalApplyProgress: Ref<ImageAdjustmentFinalApplyProgress | null>
  editorLayers: Ref<ImageEditorLayer[]>
  enter: (nodeId: string, mode: ImageEditorMode) => boolean
  cancel: () => void
  commit: () => boolean
  apply: () => Promise<void>
  onCanvasWheel: (event: WheelEvent) => void
  focusAndShowChrome: (nodeId: string) => void | Promise<void>
  scheduleChromeAnchorUpdate: () => void
  stageBindingFor: (nodeId: string) => ImageToolStageBinding
  actionBindingFor: (nodeId: string) => ImageActionBinding
  discardNodeEdits: (nodeId: string) => void
  runPromptAction: (nodeId: string, prompt: string, outputCount: number) => Promise<void> | void
}
