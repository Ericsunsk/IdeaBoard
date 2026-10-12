import { DEFAULT_PROMPT_ASPECT_RATIO, type PromptAspectRatio } from './promptAspectRatio'
import { DEFAULT_FUSION_BLEND_POINT, type FusionBlendPoint } from './fusionBlend'
import {
  assertRequestedOutputCount,
  MAX_REQUESTED_OUTPUT_COUNT,
  MIN_REQUESTED_OUTPUT_COUNT,
  REQUESTED_OUTPUT_COUNT_OPTIONS,
  type OutputResolution,
} from '../../../../core/contracts/generationContract'
import type { MediaRef } from '../../../../core/media/mediaRef'

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
export const BOARD_STATE_SCHEMA_VERSION = 4
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
  title?: string | undefined
  text: string
  outputCount: number
  outputResolution?: OutputResolution | undefined
  aspectRatio: PromptAspectRatio
  inputImages: PromptNodeInputImage[]
}

export interface PromptNodeInputImage extends ImageNodeOutputItem {
  id: string
  sourceType: 'node' | 'external'
  sourceId: string
  sourceNodeId?: string | undefined
}
export type FusionNodeInputImage = PromptNodeInputImage

export interface ImageNodeOutputItem {
  /** Runtime display URL. Durable board state is `mediaRef`. */
  url: string
  ratio: number
  mediaRef?: MediaRef | undefined
  mediaAssetId?: number | undefined
}

/** @externalContract Persisted Board v4 recipe steps. */
export type ImageRetouchRecipeStep =
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

export interface ImageRetouchRecipe {
  version: 1
  sourceAssetId: string
  sourceMimeType?: string | undefined
  plan: ImageRetouchRecipeStep[]
}

export interface BoardStateMediaRef {
  mediaAssetId: number
  refType: 'board' | 'board_node'
  refId: string
  role?: string | undefined
}

/**
 * Durable intent to materialize an owned Library asset into a Canvas image.
 * The source remains owned by Library until materialization succeeds, so this
 * fact never carries provider URLs or a temporary media reference.
 *
 * `libraryAssetId` accepts the canonical workspace opaque id (`lasset_...`).
 */
export type LibraryImportFact =
  | { state: 'pending'; libraryAssetId: string }
  | { state: 'failed'; libraryAssetId: string; failureMessage: string }

export interface ImageNodeItem {
  id: number
  type: 'image-node'
  x: number
  y: number
  width: number
  title?: string | undefined
  /** Runtime display URL. Durable board state is `sourceMediaRef`. */
  sourceImageUrl: string
  sourceMediaRef?: MediaRef | undefined
  sourceMediaAssetId?: number | undefined
  retouchRecipe?: ImageRetouchRecipe | undefined
  sourceImageRatio: number
  outputCount?: number | undefined
  outputResolution?: OutputResolution | undefined
  pendingTaskId?: string | undefined
  pendingResultIndex?: number | undefined
  pendingClientActionId?: string | undefined
  libraryImport?: LibraryImportFact | undefined
  outputStatus: WorkflowNodeOutputStatus
  outputImages: ImageNodeOutputItem[]
}

export interface FusionNodeItem {
  id: number
  type: 'fusion'
  x: number
  y: number
  width: number
  title?: string | undefined
  prompt: string
  outputCount: number
  outputResolution?: OutputResolution | undefined
  aspectRatio: PromptAspectRatio
  blendPoint: FusionBlendPoint
  inputImages: FusionNodeInputImage[]
  outputStatus: WorkflowNodeOutputStatus
  outputImages: ImageNodeOutputItem[]
}

export type CanvasExecutionOutputNodeKind = 'fusion'

export type CanvasExecutionOutputNodeItem = FusionNodeItem

export type CanvasConnectionNodeKind = 'image' | 'prompt' | 'fusion'

export type CanvasConnectionPort = 'image' | 'prompt'
export type CanvasGroupId = `group-${number}`
export type CanvasGroupNodeId = `${CanvasConnectionNodeKind}-${number}`

export interface CanvasConnectionEndpoint {
  fromNodeType: CanvasConnectionNodeKind
  fromNodeId: number
  fromPort: CanvasConnectionPort
  toNodeType: CanvasConnectionNodeKind
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

export interface BoardSnapshot {
  schemaVersion: typeof BOARD_STATE_SCHEMA_VERSION
  imageNodes: ImageNodeItem[]
  promptNodes: PromptNodeItem[]
  fusionNodes: FusionNodeItem[]
  connections: CanvasConnection[]
  groups: CanvasGroup[]
  mediaRefs?: BoardStateMediaRef[] | undefined
}

export interface BoardState extends BoardSnapshot {
  previewSnapshotDataUrl?: string | undefined
  previewSnapshotUpdatedAt?: number | undefined
  previewSnapshotVersion?: number | undefined
  updatedAt: number
}

export function createEmptyBoardSnapshot(): BoardSnapshot {
  return {
    schemaVersion: BOARD_STATE_SCHEMA_VERSION,
    imageNodes: [],
    promptNodes: [],
    fusionNodes: [],
    connections: [],
    groups: [],
  }
}

export function createEmptyBoardState(updatedAt = Date.now()): BoardState {
  return {
    ...createEmptyBoardSnapshot(),
    updatedAt,
  }
}
