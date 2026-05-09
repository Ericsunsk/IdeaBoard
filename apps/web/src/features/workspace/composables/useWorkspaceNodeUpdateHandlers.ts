import {
  clampModelAngleYaw,
  clampModelPoseStrength,
  clampModelReferenceStrength,
  clampEnhanceExtendPercent,
  clampWearOpacity,
  clampWearRotation,
  clampWearScale,
  isColorNodePlaceholderState,
  normalizeColorNodeColors,
  normalizeWearTileMode,
} from '../types/canvasNodes'
import { clampMaskRefineAmount } from '../services/flowMaskProcessing'
import type {
  ColorNodeItem,
  EditNodeItem,
  EnhanceNodeItem,
  FusionNodeItem,
  MaskNodeItem,
  ModelAngleFraming,
  ModelNodeItem,
  PromptNodeItem,
  ViewpointNodeItem,
  WearNodeItem,
} from '../types/canvasNodes'

interface UseWorkspaceNodeUpdateHandlersOptions {
  markWorkflowInputChanged: (nodeType: 'prompt' | 'image', nodeId: number) => void
  markCanvasContentChanged: () => void
  markExecutionInputContentChanged: (
    nodeType: 'model' | 'viewpoint' | 'fusion' | 'enhance' | 'wear' | 'color' | 'edit',
    nodeId: number,
  ) => void
  markMaskNodeChanged: (node: MaskNodeItem) => void
  setEditNodeMode: (node: EditNodeItem, mode: EditNodeItem['mode']) => void
  setEnhanceNodeMode: (node: EnhanceNodeItem, mode: EnhanceNodeItem['mode']) => void
  onMaskCanvasMouseDown: (event: MouseEvent, node: MaskNodeItem) => void
  updateModelAngleYaw: (node: ModelNodeItem, yaw: number) => void
  updateModelAngleFraming: (node: ModelNodeItem, framing: ModelAngleFraming) => void
}

export function useWorkspaceNodeUpdateHandlers(options: UseWorkspaceNodeUpdateHandlersOptions) {
  function onPromptNodeTextUpdate(node: PromptNodeItem, value: string) {
    if (node.text === value) return
    node.text = value
    options.markWorkflowInputChanged('prompt', node.id)
    options.markCanvasContentChanged()
  }

  function onModelNodeSelectBackground(node: ModelNodeItem, key: string) {
    if (node.background === key) return
    node.background = key
    options.markExecutionInputContentChanged('model', node.id)
  }

  function onModelNodeUpdateAngleYaw(node: ModelNodeItem, value: number) {
    const previousYaw = node.angleSpec.yaw
    options.updateModelAngleYaw(node, clampModelAngleYaw(value))
    if (node.angleSpec.yaw === previousYaw) return
    options.markExecutionInputContentChanged('model', node.id)
  }

  function onModelNodeUpdateAngleFraming(node: ModelNodeItem, value: ModelAngleFraming) {
    const previousFraming = node.angleSpec.framing
    options.updateModelAngleFraming(node, value)
    if (node.angleSpec.framing === previousFraming) return
    options.markExecutionInputContentChanged('model', node.id)
  }

  function onModelNodeUpdateModelStrength(node: ModelNodeItem, value: number) {
    const nextValue = clampModelReferenceStrength(value)
    if (node.modelStrength === nextValue) return
    node.modelStrength = nextValue
    options.markExecutionInputContentChanged('model', node.id)
  }

  function onModelNodeUpdatePoseStrength(node: ModelNodeItem, value: number) {
    const nextValue = clampModelPoseStrength(value)
    if (node.poseStrength === nextValue) return
    node.poseStrength = nextValue
    options.markExecutionInputContentChanged('model', node.id)
  }

  function onViewpointNodeUpdateLighting(node: ViewpointNodeItem, value: ViewpointNodeItem['lighting']) {
    if (node.lighting === value) return
    node.lighting = value
    options.markExecutionInputContentChanged('viewpoint', node.id)
  }

  function onMaskNodeCanvasMouseDownBridge(event: MouseEvent, node: MaskNodeItem) {
    options.onMaskCanvasMouseDown(event, node)
  }

  function onMaskNodeSetDrawMode(node: MaskNodeItem, value: 'paint' | 'erase') {
    node.drawMode = value
  }

  function onMaskNodeUpdateBrushSize(node: MaskNodeItem, value: number) {
    node.brushSize = value
  }

  function onMaskNodeUpdateRefineAmount(node: MaskNodeItem, value: number) {
    const nextValue = clampMaskRefineAmount(value)
    if (node.refineAmount === nextValue) return
    node.refineAmount = nextValue
    options.markMaskNodeChanged(node)
  }

  function onMaskNodeToggleInvert(node: MaskNodeItem) {
    node.invertMask = !node.invertMask
    options.markMaskNodeChanged(node)
  }

  function onEditNodeUpdateMode(node: EditNodeItem, value: EditNodeItem['mode']) {
    options.setEditNodeMode(node, value)
  }

  function onFusionNodeUpdateBlendRatio(node: FusionNodeItem, value: number) {
    const nextValue = Math.max(0, Math.min(100, Math.round(value)))
    if (node.blendRatio === nextValue) return
    node.blendRatio = nextValue
    options.markExecutionInputContentChanged('fusion', node.id)
  }

  function onEnhanceNodeUpdateMode(node: EnhanceNodeItem, value: EnhanceNodeItem['mode']) {
    options.setEnhanceNodeMode(node, value)
  }

  function onEnhanceNodeUpdateUpscaleMode(node: EnhanceNodeItem, value: EnhanceNodeItem['upscaleMode']) {
    if (node.upscaleMode === value) return
    node.upscaleMode = value
    options.markExecutionInputContentChanged('enhance', node.id)
  }

  function onEnhanceNodeUpdateUpscaleFactor(node: EnhanceNodeItem, value: EnhanceNodeItem['upscaleFactor']) {
    if (node.upscaleFactor === value) return
    node.upscaleFactor = value
    options.markExecutionInputContentChanged('enhance', node.id)
  }

  function onEnhanceNodeUpdateOutputFormat(node: EnhanceNodeItem, value: EnhanceNodeItem['outputFormat']) {
    if (node.outputFormat === value) return
    node.outputFormat = value
    options.markExecutionInputContentChanged('enhance', node.id)
  }

  function onEnhanceNodeUpdateOutputQuality(node: EnhanceNodeItem, value: number) {
    const nextValue = Math.max(30, Math.min(100, Math.round(value)))
    if (node.outputQuality === nextValue) return
    node.outputQuality = nextValue
    options.markExecutionInputContentChanged('enhance', node.id)
  }

  function onEnhanceNodeUpdateExtendTop(node: EnhanceNodeItem, value: number) {
    const nextValue = clampEnhanceExtendPercent(value)
    if (node.extendTop === nextValue) return
    node.extendTop = nextValue
    options.markExecutionInputContentChanged('enhance', node.id)
  }

  function onEnhanceNodeUpdateExtendRight(node: EnhanceNodeItem, value: number) {
    const nextValue = clampEnhanceExtendPercent(value)
    if (node.extendRight === nextValue) return
    node.extendRight = nextValue
    options.markExecutionInputContentChanged('enhance', node.id)
  }

  function onEnhanceNodeUpdateExtendBottom(node: EnhanceNodeItem, value: number) {
    const nextValue = clampEnhanceExtendPercent(value)
    if (node.extendBottom === nextValue) return
    node.extendBottom = nextValue
    options.markExecutionInputContentChanged('enhance', node.id)
  }

  function onEnhanceNodeUpdateExtendLeft(node: EnhanceNodeItem, value: number) {
    const nextValue = clampEnhanceExtendPercent(value)
    if (node.extendLeft === nextValue) return
    node.extendLeft = nextValue
    options.markExecutionInputContentChanged('enhance', node.id)
  }

  function onWearNodeUpdateTileMode(node: WearNodeItem, value: WearNodeItem['tileMode']) {
    const nextValue = normalizeWearTileMode(value)
    if (node.tileMode === nextValue) return
    node.tileMode = nextValue
    options.markExecutionInputContentChanged('wear', node.id)
  }

  function onWearNodeUpdateRotation(node: WearNodeItem, value: number) {
    const nextValue = clampWearRotation(value)
    if (node.rotation === nextValue) return
    node.rotation = nextValue
    options.markExecutionInputContentChanged('wear', node.id)
  }

  function onWearNodeUpdateScale(node: WearNodeItem, value: number) {
    const nextValue = clampWearScale(value)
    if (node.scale === nextValue) return
    node.scale = nextValue
    options.markExecutionInputContentChanged('wear', node.id)
  }

  function onWearNodeUpdateOpacity(node: WearNodeItem, value: number) {
    const nextValue = clampWearOpacity(value)
    if (node.opacity === nextValue) return
    node.opacity = nextValue
    options.markExecutionInputContentChanged('wear', node.id)
  }

  function onColorNodeApplyPreset(node: ColorNodeItem, value: string[]) {
    const baseColors = isColorNodePlaceholderState(node) ? [] : node.colors
    const nextColors = normalizeColorNodeColors([...baseColors, ...value])
    if (nextColors.join(',') === node.colors.join(',')) return
    node.colors = nextColors
    options.markExecutionInputContentChanged('color', node.id)
  }

  function onColorNodeStrengthInput(node: ColorNodeItem, value: number) {
    const nextValue = Math.max(0, Math.min(100, Math.round(value)))
    if (node.strength === nextValue) return
    node.strength = nextValue
    options.markExecutionInputContentChanged('color', node.id)
  }

  return {
    onPromptNodeTextUpdate,
    onModelNodeSelectBackground,
    onModelNodeUpdateAngleYaw,
    onModelNodeUpdateAngleFraming,
    onModelNodeUpdateModelStrength,
    onModelNodeUpdatePoseStrength,
    onViewpointNodeUpdateLighting,
    onMaskNodeCanvasMouseDownBridge,
    onMaskNodeSetDrawMode,
    onMaskNodeUpdateBrushSize,
    onMaskNodeUpdateRefineAmount,
    onMaskNodeToggleInvert,
    onEditNodeUpdateMode,
    onFusionNodeUpdateBlendRatio,
    onEnhanceNodeUpdateMode,
    onEnhanceNodeUpdateUpscaleMode,
    onEnhanceNodeUpdateUpscaleFactor,
    onEnhanceNodeUpdateOutputFormat,
    onEnhanceNodeUpdateOutputQuality,
    onEnhanceNodeUpdateExtendTop,
    onEnhanceNodeUpdateExtendRight,
    onEnhanceNodeUpdateExtendBottom,
    onEnhanceNodeUpdateExtendLeft,
    onWearNodeUpdateTileMode,
    onWearNodeUpdateRotation,
    onWearNodeUpdateScale,
    onWearNodeUpdateOpacity,
    onColorNodeApplyPreset,
    onColorNodeStrengthInput,
  }
}
