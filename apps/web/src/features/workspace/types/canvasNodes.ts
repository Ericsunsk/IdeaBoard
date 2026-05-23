import {
  DEFAULT_PROMPT_ASPECT_RATIO,
  type PromptAspectRatio,
} from '../config/promptAspectRatio'
import {
  DEFAULT_FUSION_BLEND_POINT,
  normalizeFusionBlendPoint,
  type FusionBlendPoint,
} from '../utils/fusionBlend'

export interface CanvasPoint {
  x: number
  y: number
}

export type WorkflowNodeOutputStatus = 'idle' | 'running' | 'ready' | 'stale'
export const MODEL_POSE_STRENGTH_MIN = 0
export const MODEL_POSE_STRENGTH_MAX = 100
export const MODEL_POSE_STRENGTH_DEFAULT = 70
export const MODEL_REFERENCE_STRENGTH_MIN = 0
export const MODEL_REFERENCE_STRENGTH_MAX = 100
export const MODEL_REFERENCE_STRENGTH_DEFAULT = 80
export const MODEL_ANGLE_YAW_MIN = -180
export const MODEL_ANGLE_YAW_MAX = 180
export const MODEL_ANGLE_YAW_DEFAULT = 0

export const WORKFLOW_NODE_OUTPUT_STATUSES: readonly WorkflowNodeOutputStatus[] = [
  'idle',
  'running',
  'ready',
  'stale',
]
export const MIN_IMAGE_NODE_OUTPUT_COUNT = 1
export const MAX_IMAGE_NODE_OUTPUT_COUNT = 8
export const DEFAULT_IMAGE_NODE_OUTPUT_COUNT = 1
export const IMAGE_OUTPUT_COUNT_PRESET_OPTIONS = [1, 2, 4, 8] as const
export const DEFAULT_PROMPT_NODE_OUTPUT_COUNT = 1
export const DEFAULT_FUSION_NODE_OUTPUT_COUNT = 1
export { DEFAULT_PROMPT_ASPECT_RATIO, type PromptAspectRatio }
export { DEFAULT_FUSION_BLEND_POINT, type FusionBlendPoint }

export function isWorkflowNodeOutputStatus(value: unknown): value is WorkflowNodeOutputStatus {
  return (
    typeof value === 'string' &&
    WORKFLOW_NODE_OUTPUT_STATUSES.includes(value as WorkflowNodeOutputStatus)
  )
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

export function clampImageNodeOutputCount(value: unknown): number {
  const normalized = typeof value === 'string' ? Number(value.trim()) : value
  const count =
    typeof normalized === 'number' && Number.isFinite(normalized)
      ? Math.round(normalized)
      : normalized
  if (typeof count !== 'number' || !Number.isFinite(count)) {
    return DEFAULT_IMAGE_NODE_OUTPUT_COUNT
  }
  return Math.max(MIN_IMAGE_NODE_OUTPUT_COUNT, Math.min(MAX_IMAGE_NODE_OUTPUT_COUNT, count))
}

export function clampFusionBlendPoint(value: unknown): FusionBlendPoint {
  return normalizeFusionBlendPoint(value)
}

export interface PromptNodeItem {
  id: number
  type: 'prompt'
  x: number
  y: number
  width: number
  title?: string
  text: string
  outputCount: number
  aspectRatio: PromptAspectRatio
  inputImages: PromptNodeInputImage[]
}

export interface PromptNodeInputImage extends ImageNodeOutputItem {
  id: string
  sourceType: 'node' | 'external'
  sourceId: string
  sourceNodeId?: string
}
export type FusionNodeInputImage = PromptNodeInputImage

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
    ? Math.max(
        MODEL_REFERENCE_STRENGTH_MIN,
        Math.min(MODEL_REFERENCE_STRENGTH_MAX, Math.round(value)),
      )
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

export interface ModelNodeItem {
  id: number
  type: 'model'
  x: number
  y: number
  width: number
  title?: string
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
  title?: string
  activeCategory: 'top' | 'bottom' | 'set'
  selectedViewpoint: string
  lighting: 'warm_white' | 'afternoon' | 'hard_light'
  outputStatus: WorkflowNodeOutputStatus
  outputImages: ImageNodeOutputItem[]
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
  title?: string
  sourceImageUrl: string
  sourceImageRatio: number
  outputCount?: number
  outputStatus: WorkflowNodeOutputStatus
  outputImages: ImageNodeOutputItem[]
}

export interface FusionNodeItem {
  id: number
  type: 'fusion'
  x: number
  y: number
  width: number
  title?: string
  prompt: string
  outputCount: number
  aspectRatio: PromptAspectRatio
  blendPoint: FusionBlendPoint
  inputImages: FusionNodeInputImage[]
  outputStatus: WorkflowNodeOutputStatus
  outputImages: ImageNodeOutputItem[]
}

export type CanvasExecutionOutputNodeType =
  | 'model'
  | 'viewpoint'
  | 'fusion'

export const EXECUTION_OUTPUT_NODE_TYPES: readonly CanvasExecutionOutputNodeType[] = [
  'model',
  'viewpoint',
  'fusion',
]

export function isCanvasExecutionOutputNodeType(
  value: unknown,
): value is CanvasExecutionOutputNodeType {
  return (
    typeof value === 'string' &&
    EXECUTION_OUTPUT_NODE_TYPES.includes(value as CanvasExecutionOutputNodeType)
  )
}

export type CanvasExecutionOutputNodeItem =
  | ModelNodeItem
  | ViewpointNodeItem
  | FusionNodeItem

export type CanvasImageOutputNodeType = 'image' | CanvasExecutionOutputNodeType

export type CanvasImageOutputNodeItem = ImageNodeItem | CanvasExecutionOutputNodeItem

export type CanvasConnectionNodeType =
  | 'image'
  | 'prompt'
  | 'model'
  | 'viewpoint'
  | 'fusion'

export const CANVAS_CONNECTION_NODE_TYPES: readonly CanvasConnectionNodeType[] = [
  'image',
  'prompt',
  'model',
  'viewpoint',
  'fusion',
] as const

export function isCanvasConnectionNodeType(value: unknown): value is CanvasConnectionNodeType {
  return (
    typeof value === 'string' &&
    CANVAS_CONNECTION_NODE_TYPES.includes(value as CanvasConnectionNodeType)
  )
}

export function getDefaultCanvasNodeTitle(nodeType: CanvasConnectionNodeType): string {
  switch (nodeType) {
    case 'image':
      return 'Image'
    case 'prompt':
      return 'Prompt'
    case 'model':
      return 'Model'
    case 'viewpoint':
      return 'Display'
    case 'fusion':
      return 'Fusion'
  }
}
export type CanvasConnectionPort = 'image' | 'prompt' | 'imageA' | 'imageB'

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
  | ImageNodeItem
  | FusionNodeItem

export interface CanvasViewportState {
  canvasX: number
  canvasY: number
  zoomLevel: number
}

export interface CanvasBoardSnapshot {
  imageNodes: ImageNodeItem[]
  promptNodes: PromptNodeItem[]
  modelNodes: ModelNodeItem[]
  viewpointNodes: ViewpointNodeItem[]
  fusionNodes: FusionNodeItem[]
  connections: CanvasConnection[]
  viewport: CanvasViewportState
}
