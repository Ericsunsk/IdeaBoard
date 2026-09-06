import { DEFAULT_PROMPT_ASPECT_RATIO, type PromptAspectRatio } from './promptAspectRatio'
import { DEFAULT_FUSION_BLEND_POINT, type FusionBlendPoint } from './fusionBlend'
import {
  assertRequestedOutputCount,
  MAX_REQUESTED_OUTPUT_COUNT,
  MIN_REQUESTED_OUTPUT_COUNT,
  REQUESTED_OUTPUT_COUNT_OPTIONS,
  type OutputResolution,
} from '../../../shared/domain/generationContract'
import type { MediaRef } from '../../../shared/media/MediaRef'

export interface CanvasPoint {
  x: number
  y: number
}

export type WorkflowNodeOutputStatus = 'idle' | 'running' | 'ready' | 'stale'

const WORKFLOW_NODE_OUTPUT_STATUSES: readonly WorkflowNodeOutputStatus[] = [
  'idle',
  'running',
  'ready',
  'stale',
]
export const MIN_IMAGE_NODE_OUTPUT_COUNT = MIN_REQUESTED_OUTPUT_COUNT
export const MAX_IMAGE_NODE_OUTPUT_COUNT = MAX_REQUESTED_OUTPUT_COUNT
export const DEFAULT_IMAGE_NODE_OUTPUT_COUNT = 1
export const IMAGE_OUTPUT_COUNT_PRESET_OPTIONS = REQUESTED_OUTPUT_COUNT_OPTIONS
export const DEFAULT_PROMPT_NODE_OUTPUT_COUNT = 1
export const DEFAULT_FUSION_NODE_OUTPUT_COUNT = 1
export const CANVAS_BOARD_STATE_SCHEMA_VERSION = 3
export { DEFAULT_PROMPT_ASPECT_RATIO, type PromptAspectRatio }
export type { OutputResolution }
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

export function requireImageNodeOutputCount(value: unknown): number {
  return assertRequestedOutputCount(value)
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
  outputResolution?: OutputResolution
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
  /** Runtime display URL. Durable board state is `mediaRef`. */
  url: string
  ratio: number
  mediaRef?: MediaRef
  mediaAssetId?: number
}

export type ImageAdjustmentRecipeStep =
  | { kind: 'hsl'; params: { hueShift: number; saturationScale: number; lightnessOffset: number } }
  | { kind: 'threshold'; threshold: number; invert: boolean }
  | {
      kind: 'channel-lut'
      luts: { red: number[]; green: number[]; blue: number[] }
    }
  | {
      kind: 'sharpen'
      params: { amount: number; radius: number; threshold: number; protectColor: boolean }
    }

export interface ImageAdjustmentRecipe {
  version: 1
  sourceMediaAssetId: number
  sourceMimeType?: string
  plan: ImageAdjustmentRecipeStep[]
}

export interface BoardStateMediaRef {
  mediaAssetId: number
  refType: 'board' | 'board_node'
  refId: string
  role?: string
}

/**
 * Durable intent to materialize an owned Library asset into a Canvas image.
 * The source remains owned by Library until materialization succeeds, so this
 * fact never carries provider URLs or a temporary media reference.
 */
export type LibraryImportFact =
  | { state: 'pending'; libraryAssetId: number }
  | { state: 'failed'; libraryAssetId: number; failureMessage: string }

export interface ImageNodeItem {
  id: number
  type: 'image-node'
  x: number
  y: number
  width: number
  title?: string
  /** Runtime display URL. Durable board state is `sourceMediaRef`. */
  sourceImageUrl: string
  sourceMediaRef?: MediaRef
  sourceMediaAssetId?: number
  adjustmentRecipe?: ImageAdjustmentRecipe
  sourceImageRatio: number
  outputCount?: number
  outputResolution?: OutputResolution
  pendingTaskId?: string
  pendingResultIndex?: number
  pendingClientActionId?: string
  libraryImport?: LibraryImportFact
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
  outputResolution?: OutputResolution
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
export type CanvasGroupId = `group-${number}`
export type CanvasGroupNodeId = `${CanvasConnectionNodeType}-${number}`

export interface CanvasConnectionEndpoint {
  fromNodeType: CanvasConnectionNodeType
  fromNodeId: number
  fromPort: CanvasConnectionPort
  toNodeType: CanvasConnectionNodeType
  toNodeId: number
  toPort: CanvasConnectionPort
}

export interface CanvasConnection extends CanvasConnectionEndpoint {
  id: number
}

export interface CanvasGroup {
  id: CanvasGroupId
  nodeIds: CanvasGroupNodeId[]
  x: number
  y: number
  width: number
  height: number
}

export type CanvasNodeItem = PromptNodeItem | ImageNodeItem | FusionNodeItem

export type CanvasNodeItemByKind = {
  image: ImageNodeItem
  prompt: PromptNodeItem
  fusion: FusionNodeItem
}

export interface CanvasBoardSnapshot {
  schemaVersion: typeof CANVAS_BOARD_STATE_SCHEMA_VERSION
  imageNodes: ImageNodeItem[]
  promptNodes: PromptNodeItem[]
  fusionNodes: FusionNodeItem[]
  connections: CanvasConnection[]
  groups: CanvasGroup[]
  mediaRefs?: BoardStateMediaRef[]
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
    groups: [],
  }
}

export function createEmptyCanvasBoardState(updatedAt = Date.now()): CanvasBoardState {
  return {
    ...createEmptyCanvasBoardSnapshot(),
    updatedAt,
  }
}
