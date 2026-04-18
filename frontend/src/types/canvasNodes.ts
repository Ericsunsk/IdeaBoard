import { DEFAULT_COLOR_HEX } from '../components/canvas/nodes/colorPopoverUtils'

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

export type WorkflowNodeOutputStatus = 'idle' | 'ready' | 'stale'
export const DEFAULT_COLOR_STRENGTH = 70
export const MAX_COLOR_NODE_SWATCHES = 8
export const DEFAULT_COLOR_NODE_COLOR = DEFAULT_COLOR_HEX

export const WORKFLOW_NODE_OUTPUT_STATUSES: readonly WorkflowNodeOutputStatus[] = ['idle', 'ready', 'stale']

export function isWorkflowNodeOutputStatus(value: unknown): value is WorkflowNodeOutputStatus {
  return typeof value === 'string' && WORKFLOW_NODE_OUTPUT_STATUSES.includes(value as WorkflowNodeOutputStatus)
}

export function resolveWorkflowNodeOutputStatus(
  outputStatus: unknown,
  outputImages: ArrayLike<unknown> | null | undefined,
): WorkflowNodeOutputStatus {
  if (isWorkflowNodeOutputStatus(outputStatus)) {
    return outputStatus
  }
  return (outputImages?.length ?? 0) > 0 ? 'ready' : 'idle'
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
  outputStatus: WorkflowNodeOutputStatus
  outputImages: ImageNodeOutputItem[]
}

export interface MannequinNodeItem {
  id: number
  type: 'mannequin'
  x: number
  y: number
  width: number
  sourceImageUrl: string
  sourceImageRatio: number
  outputStatus: WorkflowNodeOutputStatus
  outputImages: ImageNodeOutputItem[]
}

export interface ViewpointNodeItem {
  id: number
  type: 'viewpoint'
  x: number
  y: number
  width: number
  sourceImages: ImageNodeOutputItem[]
  activeCategory: 'top' | 'bottom' | 'set'
  selectedViewpoints: string[]
  lighting: 'warm_white' | 'afternoon' | 'hard_light'
  outputStatus: WorkflowNodeOutputStatus
  outputImages: ImageNodeOutputItem[]
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
  colors: string[]
  strength: number
  outputStatus: WorkflowNodeOutputStatus
  outputImages: ImageNodeOutputItem[]
}

export interface EditNodeItem {
  id: number
  type: 'edit'
  x: number
  y: number
  width: number
  mode: 'partial-edit' | 'pattern-inpaint' | 'bg-swap'
  outputStatus: WorkflowNodeOutputStatus
  outputImages: ImageNodeOutputItem[]
}

export interface FusionNodeItem {
  id: number
  type: 'fusion'
  x: number
  y: number
  width: number
  blendRatio: number
  outputStatus: WorkflowNodeOutputStatus
  outputImages: ImageNodeOutputItem[]
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
  outputStatus: WorkflowNodeOutputStatus
  outputImages: ImageNodeOutputItem[]
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
  outputStatus: WorkflowNodeOutputStatus
  outputImages: ImageNodeOutputItem[]
}

export interface FaceSwapNodeItem {
  id: number
  type: 'faceswap'
  x: number
  y: number
  width: number
  outputStatus: WorkflowNodeOutputStatus
  outputImages: ImageNodeOutputItem[]
}

export function normalizeColorNodeColors(colors: readonly string[] | null | undefined): string[] {
  return Array.from(new Set(
    (colors ?? [])
      .map(color => color.trim().toUpperCase())
      .filter(color => /^#?[0-9A-F]{6}$/.test(color))
      .map(color => (color.startsWith('#') ? color : `#${color}`)),
  )).slice(0, MAX_COLOR_NODE_SWATCHES)
}

export function hasColorNodeColors(node: { colors?: readonly string[] | null }) {
  return (node.colors?.length ?? 0) > 0
}

export function isColorNodePlaceholderState(node: { colors?: readonly string[] | null }) {
  return (node.colors?.length ?? 0) === 1 && normalizeColorNodeColors(node.colors)[0] === DEFAULT_COLOR_NODE_COLOR
}

export function isColorSingleColorMode(node: { colors?: readonly string[] | null }) {
  return (node.colors?.length ?? 0) === 1
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
  outputStatus: WorkflowNodeOutputStatus
  outputImages: ImageNodeOutputItem[]
}

export type CanvasExecutionOutputNodeType =
  | 'model'
  | 'mannequin'
  | 'viewpoint'
  | 'color'
  | 'edit'
  | 'fusion'
  | 'enhance'
  | 'tryon'
  | 'faceswap'
  | 'generate'

export const EXECUTION_OUTPUT_NODE_TYPES: readonly CanvasExecutionOutputNodeType[] = [
  'model',
  'mannequin',
  'viewpoint',
  'color',
  'edit',
  'fusion',
  'enhance',
  'tryon',
  'faceswap',
  'generate',
]

export function isCanvasExecutionOutputNodeType(value: unknown): value is CanvasExecutionOutputNodeType {
  return typeof value === 'string' && EXECUTION_OUTPUT_NODE_TYPES.includes(value as CanvasExecutionOutputNodeType)
}

export type CanvasExecutionOutputNodeItem =
  | ModelNodeItem
  | MannequinNodeItem
  | ViewpointNodeItem
  | ColorNodeItem
  | EditNodeItem
  | FusionNodeItem
  | EnhanceNodeItem
  | TryOnNodeItem
  | FaceSwapNodeItem
  | GenerateNodeItem

export type CanvasImageOutputNodeType = 'image' | CanvasExecutionOutputNodeType

export type CanvasImageOutputNodeItem = ImageNodeItem | CanvasExecutionOutputNodeItem

export type CanvasConnectionNodeType =
  | 'image'
  | 'prompt'
  | 'mask'
  | 'color'
  | 'model'
  | 'mannequin'
  | 'viewpoint'
  | 'edit'
  | 'fusion'
  | 'enhance'
  | 'tryon'
  | 'faceswap'
  | 'generate'
export type CanvasConnectionPort = 'image' | 'mask' | 'prompt' | 'imageA' | 'imageB'

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
  | MannequinNodeItem
  | ViewpointNodeItem
  | MaskNodeItem
  | ImageNodeItem
  | ColorNodeItem
  | EditNodeItem
  | FusionNodeItem
  | EnhanceNodeItem
  | TryOnNodeItem
  | FaceSwapNodeItem
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
  mannequinNodes: MannequinNodeItem[]
  viewpointNodes: ViewpointNodeItem[]
  maskNodes: MaskNodeItem[]
  colorNodes: ColorNodeItem[]
  editNodes: EditNodeItem[]
  fusionNodes: FusionNodeItem[]
  enhanceNodes: EnhanceNodeItem[]
  tryOnNodes: TryOnNodeItem[]
  faceSwapNodes: FaceSwapNodeItem[]
  generateNodes: GenerateNodeItem[]
  connections: CanvasConnection[]
  viewport: CanvasViewportState
}
