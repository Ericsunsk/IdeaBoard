export interface CanvasPoint {
  x: number
  y: number
}

export interface CanvasImageItem {
  id: number
  type: 'image'
  url: string
  x: number
  y: number
  width?: number
  height_ratio?: number
}

export interface PromptNodeItem {
  id: number
  type: 'prompt'
  x: number
  y: number
  width: number
  text: string
}

export interface ModelNodeItem {
  id: number
  type: 'model'
  x: number
  y: number
  width: number
  sourceImageUrl: string
  sourceImageRatio: number
  selectedAngles: string[]
  background: string
}

export interface MaskNodeItem {
  id: number
  type: 'mask'
  x: number
  y: number
  width: number
  sourceImageUrl: string
  sourceImageRatio: number
  maskDataUrl: string
  brushSize: number
  drawMode: 'paint' | 'erase'
  refineAmount: number
  invertMask: boolean
}

export interface ImageNodeOutputItem {
  url: string
  ratio: number
}

export interface ImageNodeItem {
  id: number
  type: 'image-node'
  x: number
  y: number
  width: number
  sourceImageUrl: string
  sourceImageRatio: number
  outputImages: ImageNodeOutputItem[]
}

export interface ColorNodeItem {
  id: number
  type: 'color'
  x: number
  y: number
  width: number
  targetColor: string
  strength: number
}

export type CanvasNodeItem =
  | PromptNodeItem
  | ModelNodeItem
  | MaskNodeItem
  | ImageNodeItem
  | ColorNodeItem

export interface CanvasViewportState {
  canvasX: number
  canvasY: number
  zoomLevel: number
}

export interface CanvasBoardSnapshot {
  messages: CanvasImageItem[]
  imageNodes: ImageNodeItem[]
  promptNodes: PromptNodeItem[]
  modelNodes: ModelNodeItem[]
  maskNodes: MaskNodeItem[]
  colorNodes: ColorNodeItem[]
  viewport: CanvasViewportState
}
