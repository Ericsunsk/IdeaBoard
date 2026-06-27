import { DEFAULT_PROMPT_ASPECT_RATIO, type PromptAspectRatio } from './promptAspectRatio'
import { DEFAULT_FUSION_BLEND_POINT, type FusionBlendPoint } from './fusionBlend'

export interface CanvasPoint {
  x: number
  y: number
}

export type WorkflowNodeOutputStatus = 'idle' | 'running' | 'ready' | 'stale'

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
export const CANVAS_BOARD_STATE_SCHEMA_VERSION = 1
export { DEFAULT_PROMPT_ASPECT_RATIO, type PromptAspectRatio }
export { DEFAULT_FUSION_BLEND_POINT, type FusionBlendPoint }

function isWorkflowNodeOutputStatus(value: unknown): value is WorkflowNodeOutputStatus {
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
  pendingTaskId?: string
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

export type CanvasExecutionOutputNodeType = 'fusion'

export type CanvasExecutionOutputNodeItem = FusionNodeItem

export type CanvasConnectionNodeType = 'image' | 'prompt' | 'fusion'

export type CanvasConnectionPort = 'image' | 'prompt'

export interface CanvasConnection {
  id: number
  fromNodeType: CanvasConnectionNodeType
  fromNodeId: number
  fromPort: CanvasConnectionPort
  toNodeType: CanvasConnectionNodeType
  toNodeId: number
  toPort: CanvasConnectionPort
}

export type CanvasNodeItem = PromptNodeItem | ImageNodeItem | FusionNodeItem

export type CanvasNodeItemByKind = {
  image: ImageNodeItem
  prompt: PromptNodeItem
  fusion: FusionNodeItem
}

export interface CanvasViewportState {
  canvasX: number
  canvasY: number
  zoomLevel: number
}

export interface CanvasBoardSnapshot {
  schemaVersion: typeof CANVAS_BOARD_STATE_SCHEMA_VERSION
  imageNodes: ImageNodeItem[]
  promptNodes: PromptNodeItem[]
  fusionNodes: FusionNodeItem[]
  connections: CanvasConnection[]
  viewport: CanvasViewportState
}

export interface CanvasBoardState extends CanvasBoardSnapshot {
  previewSnapshotDataUrl?: string
  previewSnapshotUpdatedAt?: number
  previewSnapshotVersion?: number
  updatedAt: number
}

export function createEmptyCanvasBoardSnapshot(): CanvasBoardSnapshot {
  return {
    schemaVersion: CANVAS_BOARD_STATE_SCHEMA_VERSION,
    imageNodes: [],
    promptNodes: [],
    fusionNodes: [],
    connections: [],
    viewport: {
      canvasX: 0,
      canvasY: 0,
      zoomLevel: 100,
    },
  }
}

export function createEmptyCanvasBoardState(updatedAt = Date.now()): CanvasBoardState {
  return {
    ...createEmptyCanvasBoardSnapshot(),
    updatedAt,
  }
}
