import type { ComponentPublicInstance, ComputedRef, Ref } from 'vue'
import type {
  ImageAdjustmentKind,
  RetouchAdjustmentLayer,
  RetouchAdjustmentWholeParams,
  RetouchLayer,
  RetouchMode,
  RetouchParamKey,
  RetouchParamValue,
  RetouchRenderedPreview,
} from '../retouch/types'
import type { ImageMaskDrawMode } from '../inpaint/types'
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
  mode: RetouchMode | null
  renderedPreview: RetouchRenderedPreview | null
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
  mode: RetouchMode | null
  anchorRect: ImageToolAnchorRect | null
  isReady: boolean
  mask: {
    drawMode: ImageMaskDrawMode
    brushSize: number
    layerOpacity: number
    directGenerating: boolean
    canUndo: boolean
    canRedo: boolean
  }
  editor: {
    layers: RetouchLayer[]
    selectedLayerId: string
    selectedAdjustmentLayer: RetouchAdjustmentLayer | null
    histogramSource: ImageData | null
    applyingAdjustments: boolean
  }
}

export type ImageToolChromeCommands = {
  setMaskDrawMode: (mode: ImageMaskDrawMode) => void
  setMaskBrushSize: (size: number) => void
  setMaskLayerOpacity: (opacity: number) => void
  clearMask: () => void
  undoMask: () => void
  redoMask: () => void
  submitMaskPrompt: (prompt: string) => void
  selectAdjustmentLayer: (kind: ImageAdjustmentKind) => void
  selectEditorLayer: (layerId: string) => void
  setEditorLayerVisible: (layerId: string, visible: boolean) => void
  addEditorLayer: () => void
  copyEditorLayer: () => void
  mergeEditorLayer: () => void
  removeEditorLayer: (layerId: string) => void
  updateEditorParam: (key: RetouchParamKey, value: RetouchParamValue) => void
  updateEditorAdjustmentParams: (params: RetouchAdjustmentWholeParams) => void
  applyAdjustments: () => Promise<void>
}

export type CanvasImageTools = {
  viewModel: {
    active: {
      nodeId: ComputedRef<string | null>
      maskNodeId: ComputedRef<string | null>
      mode: ComputedRef<RetouchMode | null>
    }
    locks: {
      isChromeHidden: ComputedRef<boolean>
      isCanvasLocked: ComputedRef<boolean>
    }
    chrome: ComputedRef<ImageToolChromeModel>
    adjustment: {
      isApplying: Ref<boolean>
    }
    nodeBindings: {
      stageFor: (nodeId: string) => ImageToolStageBinding
      actionFor: (nodeId: string) => ImageActionBinding
    }
  }
  commands: {
    lifecycle: {
      enter: (nodeId: string, mode: RetouchMode) => boolean
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
    chrome: ImageToolChromeCommands
  }
}

export type ImageToolPresentationBinding = {
  viewModel: Pick<CanvasImageTools['viewModel'], 'chrome'>
  commands: Pick<CanvasImageTools['commands'], 'chrome'>
}
