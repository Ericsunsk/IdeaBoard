import { DEFAULT_COLOR_HEX } from '../components/nodes/colorPopoverUtils'

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
export const MODEL_POSE_STRENGTH_MIN = 0
export const MODEL_POSE_STRENGTH_MAX = 100
export const MODEL_POSE_STRENGTH_DEFAULT = 70
export const MODEL_REFERENCE_STRENGTH_MIN = 0
export const MODEL_REFERENCE_STRENGTH_MAX = 100
export const MODEL_REFERENCE_STRENGTH_DEFAULT = 80
export const MODEL_ANGLE_YAW_MIN = -180
export const MODEL_ANGLE_YAW_MAX = 180
export const MODEL_ANGLE_YAW_DEFAULT = 0
export const ENHANCE_EXTEND_PERCENT_MIN = 0
export const ENHANCE_EXTEND_PERCENT_MAX = 50
export const ENHANCE_EXTEND_PERCENT_DEFAULT = 15
export const WEAR_ROTATION_MIN = 0
export const WEAR_ROTATION_MAX = 360
export const WEAR_ROTATION_DEFAULT = 0
export const WEAR_SCALE_MIN = 10
export const WEAR_SCALE_MAX = 300
export const WEAR_SCALE_DEFAULT = 88
export const WEAR_OPACITY_MIN = 0
export const WEAR_OPACITY_MAX = 100
export const WEAR_OPACITY_DEFAULT = 82
export const WEAR_TILE_MODE_DEFAULT = 'single'

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

export type ModelAngleFraming = 'full' | 'half' | 'portrait'

export interface ModelAngleSpec {
  yaw: number
  framing: ModelAngleFraming
}

export function clampModelPoseStrength(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value)
    ? Math.max(MODEL_POSE_STRENGTH_MIN, Math.min(MODEL_POSE_STRENGTH_MAX, Math.round(value)))
    : MODEL_POSE_STRENGTH_DEFAULT
}

export function clampModelReferenceStrength(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value)
    ? Math.max(MODEL_REFERENCE_STRENGTH_MIN, Math.min(MODEL_REFERENCE_STRENGTH_MAX, Math.round(value)))
    : MODEL_REFERENCE_STRENGTH_DEFAULT
}

export function clampModelAngleYaw(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value)
    ? Math.max(MODEL_ANGLE_YAW_MIN, Math.min(MODEL_ANGLE_YAW_MAX, Math.round(value)))
    : MODEL_ANGLE_YAW_DEFAULT
}

export function createDefaultModelAngleSpec(): ModelAngleSpec {
  return {
    yaw: MODEL_ANGLE_YAW_DEFAULT,
    framing: 'full',
  }
}

export function normalizeWearTileMode(value: unknown): WearNodeItem['tileMode'] {
  return value === 'seamless' ? 'seamless' : WEAR_TILE_MODE_DEFAULT
}

export function clampWearRotation(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value)
    ? Math.max(WEAR_ROTATION_MIN, Math.min(WEAR_ROTATION_MAX, Math.round(value)))
    : WEAR_ROTATION_DEFAULT
}

export function clampWearScale(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value)
    ? Math.max(WEAR_SCALE_MIN, Math.min(WEAR_SCALE_MAX, Math.round(value)))
    : WEAR_SCALE_DEFAULT
}

export function clampWearOpacity(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value)
    ? Math.max(WEAR_OPACITY_MIN, Math.min(WEAR_OPACITY_MAX, Math.round(value)))
    : WEAR_OPACITY_DEFAULT
}

export interface ModelNodeItem {
  id: number
  type: 'model'
  x: number
  y: number
  width: number
  angleSpec: ModelAngleSpec
  modelStrength: number
  poseStrength: number
  background: string
  outputStatus: WorkflowNodeOutputStatus
  outputImages: ImageNodeOutputItem[]
}

export interface ViewpointNodeItem {
  id: number
  type: 'viewpoint'
  x: number
  y: number
  width: number
  activeCategory: 'top' | 'bottom' | 'set'
  selectedViewpoint: string
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
  mode: 'partial-edit' | 'bg-swap' | 'face-swap'
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
  mode: 'hd-upscale' | 'canvas-extend'
  upscaleMode: 'base' | 'enhancement'
  upscaleFactor: 2 | 3 | 4
  outputFormat: 'jpg' | 'png'
  outputQuality: number
  extendTop: number
  extendRight: number
  extendBottom: number
  extendLeft: number
  outputStatus: WorkflowNodeOutputStatus
  outputImages: ImageNodeOutputItem[]
}

export interface WearNodeItem {
  id: number
  type: 'wear'
  x: number
  y: number
  width: number
  tileMode: 'seamless' | 'single'
  rotation: number
  scale: number
  opacity: number
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

export type CanvasExecutionOutputNodeType =
  | 'model'
  | 'viewpoint'
  | 'color'
  | 'edit'
  | 'fusion'
  | 'enhance'
  | 'wear'

export const EXECUTION_OUTPUT_NODE_TYPES: readonly CanvasExecutionOutputNodeType[] = [
  'model',
  'viewpoint',
  'color',
  'edit',
  'fusion',
  'enhance',
  'wear',
]

export function isCanvasExecutionOutputNodeType(value: unknown): value is CanvasExecutionOutputNodeType {
  return typeof value === 'string' && EXECUTION_OUTPUT_NODE_TYPES.includes(value as CanvasExecutionOutputNodeType)
}

export type CanvasExecutionOutputNodeItem =
  | ModelNodeItem
  | ViewpointNodeItem
  | ColorNodeItem
  | EditNodeItem
  | FusionNodeItem
  | EnhanceNodeItem
  | WearNodeItem

export type CanvasImageOutputNodeType = 'image' | CanvasExecutionOutputNodeType

export type CanvasImageOutputNodeItem = ImageNodeItem | CanvasExecutionOutputNodeItem

export type CanvasConnectionNodeType =
  | 'image'
  | 'prompt'
  | 'mask'
  | 'color'
  | 'model'
  | 'viewpoint'
  | 'edit'
  | 'fusion'
  | 'enhance'
  | 'wear'
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
  | ViewpointNodeItem
  | MaskNodeItem
  | ImageNodeItem
  | ColorNodeItem
  | EditNodeItem
  | FusionNodeItem
  | EnhanceNodeItem
  | WearNodeItem

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
  viewpointNodes: ViewpointNodeItem[]
  maskNodes: MaskNodeItem[]
  colorNodes: ColorNodeItem[]
  editNodes: EditNodeItem[]
  fusionNodes: FusionNodeItem[]
  enhanceNodes: EnhanceNodeItem[]
  wearNodes: WearNodeItem[]
  connections: CanvasConnection[]
  viewport: CanvasViewportState
}
