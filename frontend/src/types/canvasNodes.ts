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

export interface EditNodeItem {
  id: number
  type: 'edit'
  x: number
  y: number
  width: number
  mode: 'partial-edit' | 'pattern-inpaint'
}

export interface FusionNodeItem {
  id: number
  type: 'fusion'
  x: number
  y: number
  width: number
  mode:
    | 'style-fusion'
    | 'pattern-fusion-creative'
    | 'pattern-fusion-style'
    | 'craft-fusion-creative'
    | 'craft-fusion-style'
}

export interface EnhanceNodeItem {
  id: number
  type: 'enhance'
  x: number
  y: number
  width: number
  mode: 'pattern-upscale' | 'hd-upscale' | 'deblur' | 'canvas-extend'
  upscaleMode: 'base' | 'enhancement'
  upscaleFactor: 2 | 3 | 4
  outputFormat: 'jpg' | 'png'
  outputQuality: number
}

export interface TryOnNodeItem {
  id: number
  type: 'tryon'
  x: number
  y: number
  width: number
  mode: 'fabric-on-body' | 'pattern-on-body'
  tileMode: 'seamless' | 'single'
  rotation: number
  scale: number
  opacity: number
}

export interface PaletteNodeItem {
  id: number
  type: 'palette'
  x: number
  y: number
  width: number
  mode: 'women' | 'men' | 'kids' | 'reference'
  colors: string[]
}

export interface GenerateNodeItem {
  id: number
  type: 'generate'
  x: number
  y: number
  width: number
  mode: 'pattern-gen' | 'style-clone' | 'seamless-tile-classic' | 'seamless-tile-cartoon'
  styleModel: string
  ratio: '1:1' | '3:4' | '4:3' | '2:3' | '3:2'
  count: 1 | 2 | 4
  quality: '1k' | '2k' | '4k'
}

export type CanvasConnectionNodeType =
  | 'image'
  | 'prompt'
  | 'mask'
  | 'color'
  | 'model'
  | 'edit'
  | 'fusion'
  | 'enhance'
  | 'tryon'
  | 'palette'
  | 'generate'
export type CanvasConnectionPort = 'image' | 'mask' | 'prompt' | 'imageA' | 'imageB' | 'reference'

export interface CanvasConnection {
  id: number
  fromNodeType: CanvasConnectionNodeType
  fromNodeId: number
  fromPort: CanvasConnectionPort
  toNodeType: CanvasConnectionNodeType
  toNodeId: number
  toPort: CanvasConnectionPort
}

export type CanvasNodeItem =
  | PromptNodeItem
  | ModelNodeItem
  | MaskNodeItem
  | ImageNodeItem
  | ColorNodeItem
  | EditNodeItem
  | FusionNodeItem
  | EnhanceNodeItem
  | TryOnNodeItem
  | PaletteNodeItem
  | GenerateNodeItem

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
  editNodes: EditNodeItem[]
  fusionNodes: FusionNodeItem[]
  enhanceNodes: EnhanceNodeItem[]
  tryOnNodes: TryOnNodeItem[]
  paletteNodes: PaletteNodeItem[]
  generateNodes: GenerateNodeItem[]
  connections: CanvasConnection[]
  viewport: CanvasViewportState
}
