import type {
  CanvasBoardSnapshot,
  CanvasImageItem,
  CanvasViewportState,
  ColorNodeItem,
  ImageNodeItem,
  ImageNodeOutputItem,
  MaskNodeItem,
  ModelNodeItem,
  PromptNodeItem,
} from '../types/canvasNodes'

export type CanvasBoardStateMessage = CanvasImageItem
export type CanvasBoardStatePromptNode = PromptNodeItem
export type CanvasBoardStateModelNode = ModelNodeItem
export type CanvasBoardStateMaskNode = MaskNodeItem
export type CanvasBoardStateImageNodeOutput = ImageNodeOutputItem
export type CanvasBoardStateImageNode = ImageNodeItem
export type CanvasBoardStateColorNode = ColorNodeItem
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

function toValidImageNode(item: unknown): CanvasBoardStateImageNode | null {
  if (!isObject(item)) return null
  if (item.type !== 'image-node') return null
  if (!isFiniteNumber(item.id)) return null
  if (!isFiniteNumber(item.x) || !isFiniteNumber(item.y) || !isFiniteNumber(item.width)) return null
  const rawOutputs = Array.isArray(item.outputImages) ? item.outputImages : []
  return {
    id: item.id,
    type: 'image-node',
    x: item.x,
    y: item.y,
    width: item.width,
    sourceImageUrl: typeof item.sourceImageUrl === 'string' ? item.sourceImageUrl : '',
    sourceImageRatio: isFiniteNumber(item.sourceImageRatio) ? item.sourceImageRatio : 1,
    outputImages: rawOutputs.map(toValidImageNodeOutput).filter((output): output is CanvasBoardStateImageNodeOutput => !!output),
  }
}

function toValidModelNode(item: unknown): CanvasBoardStateModelNode | null {
  if (!isObject(item)) return null
  if (item.type !== 'model') return null
  if (!isFiniteNumber(item.id)) return null
  if (!isFiniteNumber(item.x) || !isFiniteNumber(item.y) || !isFiniteNumber(item.width)) return null
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
  const rawMaskNodes = Array.isArray(item.maskNodes) ? item.maskNodes : []
  const rawColorNodes = Array.isArray(item.colorNodes) ? item.colorNodes : []
  return {
    messages: rawMessages.map(toValidMessage).filter((m): m is CanvasBoardStateMessage => !!m),
    imageNodes: rawImageNodes.map(toValidImageNode).filter((n): n is CanvasBoardStateImageNode => !!n),
    promptNodes: rawPromptNodes.map(toValidPromptNode).filter((n): n is CanvasBoardStatePromptNode => !!n),
    modelNodes: rawModelNodes.map(toValidModelNode).filter((n): n is CanvasBoardStateModelNode => !!n),
    maskNodes: rawMaskNodes.map(toValidMaskNode).filter((n): n is CanvasBoardStateMaskNode => !!n),
    colorNodes: rawColorNodes.map(toValidColorNode).filter((n): n is CanvasBoardStateColorNode => !!n),
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
