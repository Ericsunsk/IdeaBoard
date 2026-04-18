import type {
  CanvasBoardSnapshot,
  CanvasImageItem,
  CanvasConnection,
  CanvasViewportState,
  ColorNodeItem,
  EnhanceNodeItem,
  EditNodeItem,
  FaceSwapNodeItem,
  FusionNodeItem,
  ImageNodeItem,
  ImageNodeOutputItem,
  MannequinNodeItem,
  MaskNodeItem,
  ModelNodeItem,
  PaletteNodeItem,
  PromptNodeItem,
  GenerateNodeItem,
  TryOnNodeItem,
  ViewpointNodeItem,
} from '../types/canvasNodes'
import { resolveWorkflowNodeOutputStatus } from '../types/canvasNodes'

export type CanvasBoardStateMessage = CanvasImageItem
export type CanvasBoardStatePromptNode = PromptNodeItem
export type CanvasBoardStateModelNode = ModelNodeItem
export type CanvasBoardStateMannequinNode = MannequinNodeItem
export type CanvasBoardStateViewpointNode = ViewpointNodeItem
export type CanvasBoardStateMaskNode = MaskNodeItem
export type CanvasBoardStateImageNodeOutput = ImageNodeOutputItem
export type CanvasBoardStateImageNode = ImageNodeItem
export type CanvasBoardStateColorNode = ColorNodeItem
export type CanvasBoardStateEditNode = EditNodeItem
export type CanvasBoardStateFusionNode = FusionNodeItem
export type CanvasBoardStateEnhanceNode = EnhanceNodeItem
export type CanvasBoardStateTryOnNode = TryOnNodeItem
export type CanvasBoardStateFaceSwapNode = FaceSwapNodeItem
export type CanvasBoardStatePaletteNode = PaletteNodeItem
export type CanvasBoardStateGenerateNode = GenerateNodeItem
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
  const selectedAngles = Array.isArray(item.selectedAngles)
    ? item.selectedAngles.filter((angle): angle is string => typeof angle === 'string')
    : []
  return {
    id: item.id,
    type: 'model',
    x: item.x,
    y: item.y,
    width: item.width,
    sourceImageUrl: typeof item.sourceImageUrl === 'string' ? item.sourceImageUrl : '',
    sourceImageRatio: isFiniteNumber(item.sourceImageRatio) ? item.sourceImageRatio : 1,
    selectedAngles,
    background: typeof item.background === 'string' ? item.background : 'original',
    outputStatus: resolveWorkflowNodeOutputStatus(item.outputStatus, outputImages),
    outputImages,
  }
}

function toValidMannequinNode(item: unknown): CanvasBoardStateMannequinNode | null {
  if (!isObject(item)) return null
  if (item.type !== 'mannequin') return null
  if (!isFiniteNumber(item.id)) return null
  if (!isFiniteNumber(item.x) || !isFiniteNumber(item.y) || !isFiniteNumber(item.width)) return null
  const outputImages = readOutputImages(item)
  return {
    id: item.id,
    type: 'mannequin',
    x: item.x,
    y: item.y,
    width: item.width,
    sourceImageUrl: typeof item.sourceImageUrl === 'string' ? item.sourceImageUrl : '',
    sourceImageRatio: isFiniteNumber(item.sourceImageRatio) ? item.sourceImageRatio : 1,
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
  const rawSourceImages = Array.isArray(item.sourceImages) ? item.sourceImages : []
  const sourceImages = rawSourceImages
    .map(toValidImageNodeOutput)
    .filter((image): image is CanvasBoardStateImageNodeOutput => !!image)
    .slice(0, 3)
  const activeCategory =
    item.activeCategory === 'bottom' || item.activeCategory === 'set'
      ? item.activeCategory
      : 'top'
  const selectedViewpoints = Array.isArray(item.selectedViewpoints)
    ? item.selectedViewpoints
        .filter((value): value is string => typeof value === 'string')
        .slice(0, 4)
    : []
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
    sourceImages,
    activeCategory,
    selectedViewpoints,
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
  const rawColor = typeof item.targetColor === 'string' ? item.targetColor.trim() : ''
  const targetColor = /^#[0-9a-fA-F]{6}$/.test(rawColor) ? rawColor.toUpperCase() : '#A3E635'
  const strength = isFiniteNumber(item.strength) ? Math.max(0, Math.min(100, Math.round(item.strength))) : 70
  return {
    id: item.id,
    type: 'color',
    x: item.x,
    y: item.y,
    width: item.width,
    targetColor,
    strength,
    outputStatus: resolveWorkflowNodeOutputStatus(item.outputStatus, outputImages),
    outputImages,
  }
}

function toValidConnection(item: unknown): CanvasBoardStateConnection | null {
  if (!isObject(item)) return null
  if (!isFiniteNumber(item.id)) return null
  const fromNodeType =
    item.fromNodeType === 'image' ||
    item.fromNodeType === 'prompt' ||
    item.fromNodeType === 'mask' ||
    item.fromNodeType === 'color' ||
    item.fromNodeType === 'model' ||
    item.fromNodeType === 'mannequin' ||
    item.fromNodeType === 'viewpoint' ||
    item.fromNodeType === 'edit' ||
    item.fromNodeType === 'fusion' ||
    item.fromNodeType === 'enhance' ||
    item.fromNodeType === 'tryon' ||
    item.fromNodeType === 'faceswap' ||
    item.fromNodeType === 'palette' ||
    item.fromNodeType === 'generate'
      ? item.fromNodeType
      : null
  const toNodeType =
    item.toNodeType === 'image' ||
    item.toNodeType === 'prompt' ||
    item.toNodeType === 'mask' ||
    item.toNodeType === 'color' ||
    item.toNodeType === 'model' ||
    item.toNodeType === 'mannequin' ||
    item.toNodeType === 'viewpoint' ||
    item.toNodeType === 'edit' ||
    item.toNodeType === 'fusion' ||
    item.toNodeType === 'enhance' ||
    item.toNodeType === 'tryon' ||
    item.toNodeType === 'faceswap' ||
    item.toNodeType === 'palette' ||
    item.toNodeType === 'generate'
      ? item.toNodeType
      : null
  const fromPort =
    item.fromPort === 'image' || item.fromPort === 'mask' || item.fromPort === 'prompt' || item.fromPort === 'imageA' || item.fromPort === 'imageB' || item.fromPort === 'reference'
      ? item.fromPort
      : null
  const toPort =
    item.toPort === 'image' || item.toPort === 'mask' || item.toPort === 'prompt' || item.toPort === 'imageA' || item.toPort === 'imageB' || item.toPort === 'reference'
      ? item.toPort
      : null
  if (!fromNodeType || !toNodeType || !fromPort || !toPort) return null
  if (!isFiniteNumber(item.fromNodeId) || !isFiniteNumber(item.toNodeId)) return null
  return {
    id: item.id,
    fromNodeType,
    fromNodeId: item.fromNodeId,
    fromPort,
    toNodeType,
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
    item.mode === 'pattern-inpaint' ||
    item.mode === 'bg-swap'
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
  const mode =
    item.mode === 'style-fusion' ||
    item.mode === 'pattern-fusion-creative' ||
    item.mode === 'pattern-fusion-style' ||
    item.mode === 'craft-fusion-creative' ||
    item.mode === 'craft-fusion-style'
      ? item.mode
      : 'style-fusion'
  return {
    id: item.id,
    type: 'fusion',
    x: item.x,
    y: item.y,
    width: item.width,
    mode,
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
    item.mode === 'pattern-upscale' ||
    item.mode === 'hd-upscale' ||
    item.mode === 'deblur' ||
    item.mode === 'canvas-extend'
      ? item.mode
      : 'pattern-upscale'
  const upscaleMode = item.upscaleMode === 'enhancement' ? 'enhancement' : 'base'
  const upscaleFactor = item.upscaleFactor === 3 || item.upscaleFactor === 4 ? item.upscaleFactor : 2
  const outputFormat = item.outputFormat === 'png' ? 'png' : 'jpg'
  const outputQuality = isFiniteNumber(item.outputQuality)
    ? Math.max(30, Math.min(100, Math.round(item.outputQuality)))
    : 95
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
    outputStatus: resolveWorkflowNodeOutputStatus(item.outputStatus, outputImages),
    outputImages,
  }
}

function toValidTryOnNode(item: unknown): CanvasBoardStateTryOnNode | null {
  if (!isObject(item)) return null
  if (item.type !== 'tryon') return null
  if (!isFiniteNumber(item.id)) return null
  if (!isFiniteNumber(item.x) || !isFiniteNumber(item.y) || !isFiniteNumber(item.width)) return null
  const outputImages = readOutputImages(item)
  const mode = item.mode === 'pattern-on-body' ? 'pattern-on-body' : 'fabric-on-body'
  const tileMode = item.tileMode === 'single' ? 'single' : 'seamless'
  const rotation = isFiniteNumber(item.rotation) ? Math.max(0, Math.min(360, Math.round(item.rotation))) : 0
  const scale = isFiniteNumber(item.scale) ? Math.max(10, Math.min(300, Math.round(item.scale))) : 100
  const opacity = isFiniteNumber(item.opacity) ? Math.max(0, Math.min(100, Math.round(item.opacity))) : 100
  return {
    id: item.id,
    type: 'tryon',
    x: item.x,
    y: item.y,
    width: item.width,
    mode,
    tileMode,
    rotation,
    scale,
    opacity,
    outputStatus: resolveWorkflowNodeOutputStatus(item.outputStatus, outputImages),
    outputImages,
  }
}

function toValidFaceSwapNode(item: unknown): CanvasBoardStateFaceSwapNode | null {
  if (!isObject(item)) return null
  if (item.type !== 'faceswap') return null
  if (!isFiniteNumber(item.id)) return null
  if (!isFiniteNumber(item.x) || !isFiniteNumber(item.y) || !isFiniteNumber(item.width)) return null
  const outputImages = readOutputImages(item)
  return {
    id: item.id,
    type: 'faceswap',
    x: item.x,
    y: item.y,
    width: item.width,
    outputStatus: resolveWorkflowNodeOutputStatus(item.outputStatus, outputImages),
    outputImages,
  }
}

function toValidPaletteNode(item: unknown): CanvasBoardStatePaletteNode | null {
  if (!isObject(item)) return null
  if (item.type !== 'palette') return null
  if (!isFiniteNumber(item.id)) return null
  if (!isFiniteNumber(item.x) || !isFiniteNumber(item.y) || !isFiniteNumber(item.width)) return null
  const outputImages = readOutputImages(item)
  const mode =
    item.mode === 'men' ||
    item.mode === 'kids' ||
    item.mode === 'reference'
      ? item.mode
      : 'women'
  const rawColors = Array.isArray(item.colors) ? item.colors : []
  const colors = rawColors
    .filter((color): color is string => typeof color === 'string')
    .map(color => color.trim().toUpperCase())
    .filter(color => /^#[0-9A-F]{6}$/.test(color))
    .slice(0, 8)
  return {
    id: item.id,
    type: 'palette',
    x: item.x,
    y: item.y,
    width: item.width,
    mode,
    colors,
    outputStatus: resolveWorkflowNodeOutputStatus(item.outputStatus, outputImages),
    outputImages,
  }
}

function toValidGenerateNode(item: unknown): CanvasBoardStateGenerateNode | null {
  if (!isObject(item)) return null
  if (item.type !== 'generate') return null
  if (!isFiniteNumber(item.id)) return null
  if (!isFiniteNumber(item.x) || !isFiniteNumber(item.y) || !isFiniteNumber(item.width)) return null
  const outputImages = readOutputImages(item)
  const mode =
    item.mode === 'style-clone' ||
    item.mode === 'seamless-tile-classic' ||
    item.mode === 'seamless-tile-cartoon'
      ? item.mode
      : 'pattern-gen'
  const ratio =
    item.ratio === '3:4' ||
    item.ratio === '4:3' ||
    item.ratio === '2:3' ||
    item.ratio === '3:2'
      ? item.ratio
      : '1:1'
  const count = item.count === 2 || item.count === 4 ? item.count : 1
  const quality = item.quality === '2k' || item.quality === '4k' ? item.quality : '1k'
  return {
    id: item.id,
    type: 'generate',
    x: item.x,
    y: item.y,
    width: item.width,
    mode,
    styleModel: typeof item.styleModel === 'string' ? item.styleModel : '',
    ratio,
    count,
    quality,
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
  const rawMannequinNodes = Array.isArray(item.mannequinNodes) ? item.mannequinNodes : []
  const rawViewpointNodes = Array.isArray(item.viewpointNodes) ? item.viewpointNodes : []
  const rawMaskNodes = Array.isArray(item.maskNodes) ? item.maskNodes : []
  const rawColorNodes = Array.isArray(item.colorNodes) ? item.colorNodes : []
  const rawEditNodes = Array.isArray(item.editNodes) ? item.editNodes : []
  const rawFusionNodes = Array.isArray(item.fusionNodes) ? item.fusionNodes : []
  const rawEnhanceNodes = Array.isArray(item.enhanceNodes) ? item.enhanceNodes : []
  const rawTryOnNodes = Array.isArray(item.tryOnNodes) ? item.tryOnNodes : []
  const rawFaceSwapNodes = Array.isArray(item.faceSwapNodes) ? item.faceSwapNodes : []
  const rawPaletteNodes = Array.isArray(item.paletteNodes) ? item.paletteNodes : []
  const rawGenerateNodes = Array.isArray(item.generateNodes) ? item.generateNodes : []
  const rawConnections = Array.isArray(item.connections) ? item.connections : []
  return {
    messages: rawMessages.map(toValidMessage).filter((m): m is CanvasBoardStateMessage => !!m),
    imageNodes: rawImageNodes.map(toValidImageNode).filter((n): n is CanvasBoardStateImageNode => !!n),
    promptNodes: rawPromptNodes.map(toValidPromptNode).filter((n): n is CanvasBoardStatePromptNode => !!n),
    modelNodes: rawModelNodes.map(toValidModelNode).filter((n): n is CanvasBoardStateModelNode => !!n),
    mannequinNodes: rawMannequinNodes.map(toValidMannequinNode).filter((n): n is CanvasBoardStateMannequinNode => !!n),
    viewpointNodes: rawViewpointNodes.map(toValidViewpointNode).filter((n): n is CanvasBoardStateViewpointNode => !!n),
    maskNodes: rawMaskNodes.map(toValidMaskNode).filter((n): n is CanvasBoardStateMaskNode => !!n),
    colorNodes: rawColorNodes.map(toValidColorNode).filter((n): n is CanvasBoardStateColorNode => !!n),
    editNodes: rawEditNodes.map(toValidEditNode).filter((n): n is CanvasBoardStateEditNode => !!n),
    fusionNodes: rawFusionNodes.map(toValidFusionNode).filter((n): n is CanvasBoardStateFusionNode => !!n),
    enhanceNodes: rawEnhanceNodes.map(toValidEnhanceNode).filter((n): n is CanvasBoardStateEnhanceNode => !!n),
    tryOnNodes: rawTryOnNodes.map(toValidTryOnNode).filter((n): n is CanvasBoardStateTryOnNode => !!n),
    faceSwapNodes: rawFaceSwapNodes.map(toValidFaceSwapNode).filter((n): n is CanvasBoardStateFaceSwapNode => !!n),
    paletteNodes: rawPaletteNodes.map(toValidPaletteNode).filter((n): n is CanvasBoardStatePaletteNode => !!n),
    generateNodes: rawGenerateNodes.map(toValidGenerateNode).filter((n): n is CanvasBoardStateGenerateNode => !!n),
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
