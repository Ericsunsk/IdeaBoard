import type {
  CanvasBoardSnapshot,
  CanvasImageItem,
  CanvasConnection,
  CanvasViewportState,
  ColorNodeItem,
  EnhanceNodeItem,
  EditNodeItem,
  FusionNodeItem,
  ImageNodeItem,
  ImageNodeOutputItem,
  MaskNodeItem,
  ModelAngleFraming,
  ModelAngleSpec,
  ModelNodeItem,
  PromptNodeItem,
  WearNodeItem,
  ViewpointNodeItem,
} from '../types/canvasNodes'
import {
  DEFAULT_COLOR_STRENGTH,
  ENHANCE_EXTEND_PERCENT_DEFAULT,
  ENHANCE_EXTEND_PERCENT_MAX,
  ENHANCE_EXTEND_PERCENT_MIN,
  clampWearOpacity,
  clampWearRotation,
  clampWearScale,
  clampModelAngleYaw,
  clampModelReferenceStrength,
  clampModelPoseStrength,
  createDefaultModelAngleSpec,
  normalizeWearTileMode,
  normalizeColorNodeColors,
  resolveWorkflowNodeOutputStatus,
} from '../types/canvasNodes'
import { normalizeViewpointSelectionByCategory } from '../components/canvas/nodes/nodeOptionConfig'

export type CanvasBoardStateMessage = CanvasImageItem
export type CanvasBoardStatePromptNode = PromptNodeItem
export type CanvasBoardStateModelNode = ModelNodeItem
export type CanvasBoardStateViewpointNode = ViewpointNodeItem
export type CanvasBoardStateMaskNode = MaskNodeItem
export type CanvasBoardStateImageNodeOutput = ImageNodeOutputItem
export type CanvasBoardStateImageNode = ImageNodeItem
export type CanvasBoardStateColorNode = ColorNodeItem
export type CanvasBoardStateEditNode = EditNodeItem
export type CanvasBoardStateFusionNode = FusionNodeItem
export type CanvasBoardStateEnhanceNode = EnhanceNodeItem
export type CanvasBoardStateWearNode = WearNodeItem
export type CanvasBoardStateConnection = CanvasConnection
export type CanvasBoardStateViewport = CanvasViewportState

export interface CanvasBoardState extends CanvasBoardSnapshot {
  updatedAt: number
}

const STORAGE_KEY = 'feng.canvas.boardStates.v1'

function isFiniteNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value)
}

function isObject(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object'
}

function clampEnhanceExtendPercent(value: unknown): number {
  return isFiniteNumber(value)
    ? Math.max(ENHANCE_EXTEND_PERCENT_MIN, Math.min(ENHANCE_EXTEND_PERCENT_MAX, Math.round(value)))
    : ENHANCE_EXTEND_PERCENT_DEFAULT
}

function isModelAngleFraming(value: unknown): value is ModelAngleFraming {
  return value === 'full' || value === 'half' || value === 'portrait'
}

function toValidModelAngleSpec(spec: unknown): ModelAngleSpec | null {
  if (!isObject(spec)) return null
  return {
    yaw: clampModelAngleYaw(spec.yaw),
    framing: isModelAngleFraming(spec.framing) ? spec.framing : 'full',
  }
}

function readModelAngleSpec(item: Record<string, unknown>): ModelAngleSpec {
  const angleSpec = toValidModelAngleSpec(item.angleSpec)
  if (angleSpec) return angleSpec
  return createDefaultModelAngleSpec()
}

function readRawStateMap(): Record<string, CanvasBoardState> {
  if (typeof window === 'undefined') return {}
  const raw = localStorage.getItem(STORAGE_KEY)
  if (!raw) return {}
  try {
    const parsed = JSON.parse(raw)
    if (!isObject(parsed)) return {}
    const result: Record<string, CanvasBoardState> = {}
    Object.entries(parsed).forEach(([boardId, stateLike]) => {
      const state = toValidBoardState(stateLike)
      if (!state) return
      result[boardId] = state
    })
    return result
  } catch {
    return {}
  }
}

function writeRawStateMap(stateMap: Record<string, CanvasBoardState>) {
  if (typeof window === 'undefined') return
  localStorage.setItem(STORAGE_KEY, JSON.stringify(stateMap))
}

function toValidMessage(item: unknown): CanvasBoardStateMessage | null {
  if (!isObject(item)) return null
  if (item.type !== 'image') return null
  if (!isFiniteNumber(item.id)) return null
  if (typeof item.url !== 'string' || !item.url) return null
  if (!isFiniteNumber(item.x) || !isFiniteNumber(item.y)) return null
  const width = isFiniteNumber(item.width) ? item.width : undefined
  const heightRatio = isFiniteNumber(item.height_ratio) ? item.height_ratio : undefined
  return {
    id: item.id,
    type: 'image',
    url: item.url,
    x: item.x,
    y: item.y,
    width,
    height_ratio: heightRatio,
  }
}

function toValidPromptNode(item: unknown): CanvasBoardStatePromptNode | null {
  if (!isObject(item)) return null
  if (item.type !== 'prompt') return null
  if (!isFiniteNumber(item.id)) return null
  if (!isFiniteNumber(item.x) || !isFiniteNumber(item.y)) return null
  if (!isFiniteNumber(item.width)) return null
  return {
    id: item.id,
    type: 'prompt',
    x: item.x,
    y: item.y,
    width: item.width,
    text: typeof item.text === 'string' ? item.text : '',
  }
}

function toValidImageNodeOutput(item: unknown): CanvasBoardStateImageNodeOutput | null {
  if (!isObject(item)) return null
  if (typeof item.url !== 'string' || !item.url) return null
  const ratio = isFiniteNumber(item.ratio) && item.ratio > 0 ? item.ratio : 1
  return {
    url: item.url,
    ratio,
  }
}

function readOutputImages(item: Record<string, unknown>): CanvasBoardStateImageNodeOutput[] {
  const rawOutputs = Array.isArray(item.outputImages) ? item.outputImages : []
  return rawOutputs
    .map(toValidImageNodeOutput)
    .filter((output): output is CanvasBoardStateImageNodeOutput => !!output)
}

function toValidImageNode(item: unknown): CanvasBoardStateImageNode | null {
  if (!isObject(item)) return null
  if (item.type !== 'image-node') return null
  if (!isFiniteNumber(item.id)) return null
  if (!isFiniteNumber(item.x) || !isFiniteNumber(item.y) || !isFiniteNumber(item.width)) return null
  return {
    id: item.id,
    type: 'image-node',
    x: item.x,
    y: item.y,
    width: item.width,
    sourceImageUrl: typeof item.sourceImageUrl === 'string' ? item.sourceImageUrl : '',
    sourceImageRatio: isFiniteNumber(item.sourceImageRatio) ? item.sourceImageRatio : 1,
    outputImages: readOutputImages(item),
  }
}

function toValidModelNode(item: unknown): CanvasBoardStateModelNode | null {
  if (!isObject(item)) return null
  if (item.type !== 'model') return null
  if (!isFiniteNumber(item.id)) return null
  if (!isFiniteNumber(item.x) || !isFiniteNumber(item.y) || !isFiniteNumber(item.width)) return null
  const outputImages = readOutputImages(item)
  return {
    id: item.id,
    type: 'model',
    x: item.x,
    y: item.y,
    width: item.width,
    angleSpec: readModelAngleSpec(item),
    modelStrength: clampModelReferenceStrength(item.modelStrength),
    poseStrength: clampModelPoseStrength(item.poseStrength),
    background: typeof item.background === 'string' ? item.background : 'original',
    outputStatus: resolveWorkflowNodeOutputStatus(item.outputStatus, outputImages),
    outputImages,
  }
}

function toValidViewpointNode(item: unknown): CanvasBoardStateViewpointNode | null {
  if (!isObject(item)) return null
  if (item.type !== 'viewpoint') return null
  if (!isFiniteNumber(item.id)) return null
  if (!isFiniteNumber(item.x) || !isFiniteNumber(item.y) || !isFiniteNumber(item.width)) return null
  const outputImages = readOutputImages(item)
  const activeCategory =
    item.activeCategory === 'bottom' || item.activeCategory === 'set'
      ? item.activeCategory
      : 'top'
  const selectedViewpoint = normalizeViewpointSelectionByCategory(
    activeCategory,
    typeof item.selectedViewpoint === 'string' ? item.selectedViewpoint : '',
  )
  const lighting =
    item.lighting === 'afternoon' || item.lighting === 'hard_light'
      ? item.lighting
      : 'warm_white'
  return {
    id: item.id,
    type: 'viewpoint',
    x: item.x,
    y: item.y,
    width: item.width,
    activeCategory,
    selectedViewpoint,
    lighting,
    outputStatus: resolveWorkflowNodeOutputStatus(item.outputStatus, outputImages),
    outputImages,
  }
}

function toValidMaskNode(item: unknown): CanvasBoardStateMaskNode | null {
  if (!isObject(item)) return null
  if (item.type !== 'mask') return null
  if (!isFiniteNumber(item.id)) return null
  if (!isFiniteNumber(item.x) || !isFiniteNumber(item.y) || !isFiniteNumber(item.width)) return null
  const brushSize = isFiniteNumber(item.brushSize) ? item.brushSize : 18
  const drawMode = item.drawMode === 'erase' ? 'erase' : 'paint'
  const refineAmount = isFiniteNumber(item.refineAmount) ? Math.max(-100, Math.min(100, Math.round(item.refineAmount))) : 0
  return {
    id: item.id,
    type: 'mask',
    x: item.x,
    y: item.y,
    width: item.width,
    sourceImageUrl: typeof item.sourceImageUrl === 'string' ? item.sourceImageUrl : '',
    sourceImageRatio: isFiniteNumber(item.sourceImageRatio) ? item.sourceImageRatio : 1,
    maskDataUrl: typeof item.maskDataUrl === 'string' ? item.maskDataUrl : '',
    brushSize: Math.max(2, Math.min(64, brushSize)),
    drawMode,
    refineAmount,
    invertMask: !!item.invertMask,
  }
}

function toValidColorNode(item: unknown): CanvasBoardStateColorNode | null {
  if (!isObject(item)) return null
  if (item.type !== 'color') return null
  if (!isFiniteNumber(item.id)) return null
  if (!isFiniteNumber(item.x) || !isFiniteNumber(item.y) || !isFiniteNumber(item.width)) return null
  const outputImages = readOutputImages(item)
  const rawColors = Array.isArray(item.colors) ? item.colors : []
  const colors = normalizeColorNodeColors(rawColors.filter((color): color is string => typeof color === 'string'))
  const strength = isFiniteNumber(item.strength)
    ? Math.max(0, Math.min(100, Math.round(item.strength)))
    : DEFAULT_COLOR_STRENGTH
  return {
    id: item.id,
    type: 'color',
    x: item.x,
    y: item.y,
    width: item.width,
    colors,
    strength,
    outputStatus: resolveWorkflowNodeOutputStatus(item.outputStatus, outputImages),
    outputImages,
  }
}

function toValidConnection(item: unknown): CanvasBoardStateConnection | null {
  if (!isObject(item)) return null
  if (!isFiniteNumber(item.id)) return null
  const rawFromNodeType =
    item.fromNodeType === 'image' ||
    item.fromNodeType === 'prompt' ||
    item.fromNodeType === 'mask' ||
    item.fromNodeType === 'color' ||
    item.fromNodeType === 'model' ||
    item.fromNodeType === 'viewpoint' ||
    item.fromNodeType === 'edit' ||
    item.fromNodeType === 'fusion' ||
    item.fromNodeType === 'enhance' ||
    item.fromNodeType === 'wear'
      ? item.fromNodeType
      : null
  const rawToNodeType =
    item.toNodeType === 'image' ||
    item.toNodeType === 'prompt' ||
    item.toNodeType === 'mask' ||
    item.toNodeType === 'color' ||
    item.toNodeType === 'model' ||
    item.toNodeType === 'viewpoint' ||
    item.toNodeType === 'edit' ||
    item.toNodeType === 'fusion' ||
    item.toNodeType === 'enhance' ||
    item.toNodeType === 'wear'
      ? item.toNodeType
      : null
  const fromPort =
    item.fromPort === 'image' || item.fromPort === 'mask' || item.fromPort === 'prompt' || item.fromPort === 'imageA' || item.fromPort === 'imageB'
      ? item.fromPort
      : null
  const toPort =
    item.toPort === 'image' || item.toPort === 'mask' || item.toPort === 'prompt' || item.toPort === 'imageA' || item.toPort === 'imageB'
      ? item.toPort
      : null
  if (!rawFromNodeType || !rawToNodeType || !fromPort || !toPort) return null
  if (!isFiniteNumber(item.fromNodeId) || !isFiniteNumber(item.toNodeId)) return null
  return {
    id: item.id,
    fromNodeType: rawFromNodeType,
    fromNodeId: item.fromNodeId,
    fromPort,
    toNodeType: rawToNodeType,
    toNodeId: item.toNodeId,
    toPort,
  }
}

function toValidEditNode(item: unknown): CanvasBoardStateEditNode | null {
  if (!isObject(item)) return null
  if (item.type !== 'edit') return null
  if (!isFiniteNumber(item.id)) return null
  if (!isFiniteNumber(item.x) || !isFiniteNumber(item.y) || !isFiniteNumber(item.width)) return null
  const outputImages = readOutputImages(item)
  const mode =
    item.mode === 'bg-swap' || item.mode === 'face-swap'
      ? item.mode
      : 'partial-edit'
  return {
    id: item.id,
    type: 'edit',
    x: item.x,
    y: item.y,
    width: item.width,
    mode,
    outputStatus: resolveWorkflowNodeOutputStatus(item.outputStatus, outputImages),
    outputImages,
  }
}

function toValidFusionNode(item: unknown): CanvasBoardStateFusionNode | null {
  if (!isObject(item)) return null
  if (item.type !== 'fusion') return null
  if (!isFiniteNumber(item.id)) return null
  if (!isFiniteNumber(item.x) || !isFiniteNumber(item.y) || !isFiniteNumber(item.width)) return null
  const outputImages = readOutputImages(item)
  const blendRatio = isFiniteNumber(item.blendRatio)
    ? Math.max(0, Math.min(100, Math.round(item.blendRatio)))
    : 50
  return {
    id: item.id,
    type: 'fusion',
    x: item.x,
    y: item.y,
    width: item.width,
    blendRatio,
    outputStatus: resolveWorkflowNodeOutputStatus(item.outputStatus, outputImages),
    outputImages,
  }
}

function toValidEnhanceNode(item: unknown): CanvasBoardStateEnhanceNode | null {
  if (!isObject(item)) return null
  if (item.type !== 'enhance') return null
  if (!isFiniteNumber(item.id)) return null
  if (!isFiniteNumber(item.x) || !isFiniteNumber(item.y) || !isFiniteNumber(item.width)) return null
  const outputImages = readOutputImages(item)
  const mode =
    item.mode === 'hd-upscale' ||
    item.mode === 'canvas-extend'
      ? item.mode
      : 'hd-upscale'
  const upscaleMode = item.upscaleMode === 'enhancement' ? 'enhancement' : 'base'
  const upscaleFactor = item.upscaleFactor === 3 || item.upscaleFactor === 4 ? item.upscaleFactor : 2
  const outputFormat = item.outputFormat === 'png' ? 'png' : 'jpg'
  const outputQuality = isFiniteNumber(item.outputQuality)
    ? Math.max(30, Math.min(100, Math.round(item.outputQuality)))
    : 95
  const extendTop = clampEnhanceExtendPercent(item.extendTop)
  const extendRight = clampEnhanceExtendPercent(item.extendRight)
  const extendBottom = clampEnhanceExtendPercent(item.extendBottom)
  const extendLeft = clampEnhanceExtendPercent(item.extendLeft)
  return {
    id: item.id,
    type: 'enhance',
    x: item.x,
    y: item.y,
    width: item.width,
    mode,
    upscaleMode,
    upscaleFactor,
    outputFormat,
    outputQuality,
    extendTop,
    extendRight,
    extendBottom,
    extendLeft,
    outputStatus: resolveWorkflowNodeOutputStatus(item.outputStatus, outputImages),
    outputImages,
  }
}

function toValidWearNode(item: unknown): CanvasBoardStateWearNode | null {
  if (!isObject(item)) return null
  if (item.type !== 'wear') return null
  if (!isFiniteNumber(item.id)) return null
  if (!isFiniteNumber(item.x) || !isFiniteNumber(item.y) || !isFiniteNumber(item.width)) return null
  const outputImages = readOutputImages(item)
  return {
    id: item.id,
    type: 'wear',
    x: item.x,
    y: item.y,
    width: item.width,
    tileMode: normalizeWearTileMode(item.tileMode),
    rotation: clampWearRotation(item.rotation),
    scale: clampWearScale(item.scale),
    opacity: clampWearOpacity(item.opacity),
    outputStatus: resolveWorkflowNodeOutputStatus(item.outputStatus, outputImages),
    outputImages,
  }
}

function toValidViewport(item: unknown): CanvasBoardStateViewport {
  if (!isObject(item)) {
    return { canvasX: 0, canvasY: 0, zoomLevel: 100 }
  }
  return {
    canvasX: isFiniteNumber(item.canvasX) ? item.canvasX : 0,
    canvasY: isFiniteNumber(item.canvasY) ? item.canvasY : 0,
    zoomLevel: isFiniteNumber(item.zoomLevel) ? item.zoomLevel : 100,
  }
}

function toValidBoardState(item: unknown): CanvasBoardState | null {
  if (!isObject(item)) return null
  const rawMessages = Array.isArray(item.messages) ? item.messages : []
  const rawImageNodes = Array.isArray(item.imageNodes) ? item.imageNodes : []
  const rawPromptNodes = Array.isArray(item.promptNodes) ? item.promptNodes : []
  const rawModelNodes = Array.isArray(item.modelNodes) ? item.modelNodes : []
  const rawViewpointNodes = Array.isArray(item.viewpointNodes) ? item.viewpointNodes : []
  const rawMaskNodes = Array.isArray(item.maskNodes) ? item.maskNodes : []
  const rawColorNodes = Array.isArray(item.colorNodes) ? item.colorNodes : []
  const rawEditNodes = Array.isArray(item.editNodes) ? item.editNodes : []
  const rawFusionNodes = Array.isArray(item.fusionNodes) ? item.fusionNodes : []
  const rawEnhanceNodes = Array.isArray(item.enhanceNodes) ? item.enhanceNodes : []
  const rawWearNodes = Array.isArray(item.wearNodes) ? item.wearNodes : []
  const rawConnections = Array.isArray(item.connections) ? item.connections : []
  return {
    messages: rawMessages.map(toValidMessage).filter((m): m is CanvasBoardStateMessage => !!m),
    imageNodes: rawImageNodes.map(toValidImageNode).filter((n): n is CanvasBoardStateImageNode => !!n),
    promptNodes: rawPromptNodes.map(toValidPromptNode).filter((n): n is CanvasBoardStatePromptNode => !!n),
    modelNodes: rawModelNodes.map(toValidModelNode).filter((n): n is CanvasBoardStateModelNode => !!n),
    viewpointNodes: rawViewpointNodes.map(toValidViewpointNode).filter((n): n is CanvasBoardStateViewpointNode => !!n),
    maskNodes: rawMaskNodes.map(toValidMaskNode).filter((n): n is CanvasBoardStateMaskNode => !!n),
    colorNodes: rawColorNodes.map(toValidColorNode).filter((n): n is CanvasBoardStateColorNode => !!n),
    editNodes: rawEditNodes.map(toValidEditNode).filter((n): n is CanvasBoardStateEditNode => !!n),
    fusionNodes: rawFusionNodes.map(toValidFusionNode).filter((n): n is CanvasBoardStateFusionNode => !!n),
    enhanceNodes: rawEnhanceNodes.map(toValidEnhanceNode).filter((n): n is CanvasBoardStateEnhanceNode => !!n),
    wearNodes: rawWearNodes.map(toValidWearNode).filter((n): n is CanvasBoardStateWearNode => !!n),
    connections: rawConnections.map(toValidConnection).filter((n): n is CanvasBoardStateConnection => !!n),
    viewport: toValidViewport(item.viewport),
    updatedAt: isFiniteNumber(item.updatedAt) ? item.updatedAt : Date.now(),
  }
}

function normalizeBoardId(boardId: string): string {
  return boardId.trim()
}

export function getCanvasBoardState(boardId: string): CanvasBoardState | null {
  const id = normalizeBoardId(boardId)
  if (!id) return null
  const stateMap = readRawStateMap()
  return stateMap[id] || null
}

export function setCanvasBoardState(
  boardId: string,
  state: CanvasBoardSnapshot,
): CanvasBoardState | null {
  const id = normalizeBoardId(boardId)
  if (!id) return null
  const validState = toValidBoardState({
    ...state,
    updatedAt: Date.now(),
  })
  if (!validState) return null
  const stateMap = readRawStateMap()
  stateMap[id] = validState
  writeRawStateMap(stateMap)
  return validState
}

export function deleteCanvasBoardState(boardId: string): boolean {
  const id = normalizeBoardId(boardId)
  if (!id) return false
  const stateMap = readRawStateMap()
  if (!stateMap[id]) return false
  delete stateMap[id]
  writeRawStateMap(stateMap)
  return true
}

export function duplicateCanvasBoardState(sourceBoardId: string, targetBoardId: string): boolean {
  const sourceId = normalizeBoardId(sourceBoardId)
  const targetId = normalizeBoardId(targetBoardId)
  if (!sourceId || !targetId) return false
  const stateMap = readRawStateMap()
  const sourceState = stateMap[sourceId]
  if (!sourceState) return false
  const copiedState = toValidBoardState({
    ...sourceState,
    updatedAt: Date.now(),
  })
  if (!copiedState) return false
  stateMap[targetId] = copiedState
  writeRawStateMap(stateMap)
  return true
}
