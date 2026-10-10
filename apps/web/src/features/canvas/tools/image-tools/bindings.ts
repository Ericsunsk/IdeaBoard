import type { ComputedRef, Ref } from 'vue'
import type {
  ImageRetouchKind,
  RetouchEffectLayer,
  RetouchNonHslWholeParams,
  RetouchEditorLayers,
  RetouchMode,
} from '../retouch'
import type { RetouchEffectParamKey, RetouchEffectParamMap } from '../retouch/core/model.ts'
import type { ImageMaskDrawMode } from '../inpaint'
import type { ImageStrengthAction } from '../../application/execution'
import type { ImageToolSession } from './session'
import type { ZeroCopyStageHandle } from '../retouch'

type ImageToolAnchorRect = {
  top: number
  left: number
  width: number
  height: number
}

export type ImageToolStageBinding = {
  nodeId: string
  active: boolean
  mode: RetouchMode | null
  /** Zero-copy GPU stage. Null outside adjust mode or before adapter setup. */
  retouchStage: Pick<
    ZeroCopyStageHandle,
    'canvasRef' | 'setCanvasRef' | 'isGpuActive' | 'stageStatus'
  > | null
  session: ImageToolSession
}

import type { ImageActionStrengthControlHandlers } from './model.ts'

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
    layers: RetouchEditorLayers
    selectedLayerId: string
    selectedEffectLayer: RetouchEffectLayer | null
    histogramSource: ImageData | null
    applyingRetouch: boolean
    canApply: boolean
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
  selectEffectLayer: (kind: ImageRetouchKind) => void
  selectEditorLayer: (layerId: string) => void
  setEditorLayerVisible: (layerId: string, visible: boolean) => void
  addEditorLayer: () => void
  copyEditorLayer: () => void
  mergeEditorLayer: () => void
  removeEditorLayer: (layerId: string) => void
  updateEditorParam: <TKey extends RetouchEffectParamKey>(
    key: TKey,
    value: RetouchEffectParamMap[TKey],
  ) => void
  updateEditorRetouchParams: (params: RetouchNonHslWholeParams) => void
  applyRetouch: () => Promise<void>
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
    retouch: {
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
