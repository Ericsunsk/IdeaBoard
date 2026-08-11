import type { ComponentPublicInstance, ComputedRef, Ref } from 'vue'
import type {
  ImageAdjustmentKind,
  ImageEditorAdjustmentLayer,
  ImageEditorAdjustmentWholeParams,
  ImageEditorLayer,
  ImageEditorMode,
  ImageEditorParamKey,
  ImageEditorParamValue,
  ImageEditorRenderedPreview,
  ImageEditorTool,
} from '../image-editor/types'
import type { ImageAdjustmentFinalApplyProgress } from '../image-editor/runtime/imageAdjustmentFinalApplyContext'
import type { ImageEditorRasterToolSettings } from '../image-editor/utils/imageEditorRasterTools'
import type {
  ImageMaskBrushShape,
  ImageMaskDrawMode,
  MaskEditorReferenceImage,
} from '../mask-editor/types'
import type { ImageStrengthAction } from '../execution'
import type { ImageToolSession } from './imageToolSession'

export type ImageToolAnchorRect = {
  top: number
  left: number
  width: number
  height: number
}

export type ImageToolStageBinding = {
  nodeId: string
  active: boolean
  mode: ImageEditorMode | null
  renderedPreview: ImageEditorRenderedPreview | null
  session: ImageToolSession
}

type ImageActionStrengthControlHandlers = {
  getValue: (action: ImageStrengthAction) => number
  setRangeRef: (action: ImageStrengthAction, el: Element | ComponentPublicInstance | null) => void
  setTriggerRef: (action: ImageStrengthAction, el: Element | ComponentPublicInstance | null) => void
  onRangePointerDown: (action: ImageStrengthAction, event: PointerEvent) => void
  onInput: (action: ImageStrengthAction, event: Event) => void
  onChange: (action: ImageStrengthAction, event: Event) => void
  onPointerUp: (action: ImageStrengthAction, event: PointerEvent) => void
  onClick: (action: ImageStrengthAction, event: MouseEvent) => void
}

export type ImageActionStrengthControlSource = ImageActionStrengthControlHandlers & {
  activeAction: { readonly value: ImageStrengthAction | null }
}

export type ImageActionStrengthControl = ImageActionStrengthControlHandlers & {
  visible: boolean
  activeAction: ImageStrengthAction | null
}

export type ImageActionBinding = {
  actionMenuVisible: boolean
  promptComposerVisible: boolean
  promptPlaceholder: string
  directGenerating: boolean
  strengthControl: ImageActionStrengthControl
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
    rasterToolSettings: ImageEditorRasterToolSettings
    canUseRasterTool: boolean
    canUndoRasterStroke: boolean
    canRedoRasterStroke: boolean
    hasRasterEdits: boolean
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
  updateEditorRasterToolSettings: (patch: Partial<ImageEditorRasterToolSettings>) => void
  undoEditorRasterStroke: () => void
  redoEditorRasterStroke: () => void
  applyAdjustments: () => Promise<void>
}

export type CanvasImageTools = {
  viewModel: {
    active: {
      nodeId: ComputedRef<string | null>
      maskNodeId: ComputedRef<string | null>
      mode: ComputedRef<ImageEditorMode | null>
    }
    locks: {
      isChromeHidden: ComputedRef<boolean>
      isCanvasLocked: ComputedRef<boolean>
    }
    chrome: ComputedRef<ImageToolChromeModel>
    adjustment: {
      isApplying: Ref<boolean>
      finalApplyProgress: Ref<ImageAdjustmentFinalApplyProgress | null>
    }
    nodeBindings: {
      stageFor: (nodeId: string) => ImageToolStageBinding
      actionFor: (nodeId: string) => ImageActionBinding
    }
  }
  commands: {
    lifecycle: {
      enter: (nodeId: string, mode: ImageEditorMode) => boolean
      cancel: () => void
      commit: () => boolean
      discardNodeEdits: (nodeId: string) => void
    }
    editor: {
      apply: () => Promise<void>
      onCanvasWheel: (event: WheelEvent) => void
      focusAndShowChrome: (nodeId: string) => void | Promise<void>
      scheduleChromeAnchorUpdate: () => void
    }
    prompt: {
      runPromptAction: (nodeId: string, prompt: string, outputCount: number) => Promise<void> | void
    }
    chrome: ImageToolChromeCommands
  }
}

export type ImageToolPresentationBinding = {
  viewModel: Pick<CanvasImageTools['viewModel'], 'chrome'>
  commands: Pick<CanvasImageTools['commands'], 'chrome'>
}
