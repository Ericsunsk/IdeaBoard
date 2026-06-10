import * as v from 'valibot'
import {
  DEFAULT_FUSION_BLEND_POINT,
  DEFAULT_PROMPT_ASPECT_RATIO,
  DEFAULT_PROMPT_NODE_OUTPUT_COUNT,
  MAX_IMAGE_NODE_OUTPUT_COUNT,
  MIN_IMAGE_NODE_OUTPUT_COUNT,
  type CanvasBoardSnapshot,
} from '../../workspace/types/canvasNodes'
import { PROMPT_ASPECT_RATIO_OPTIONS } from '../../workspace/config/promptAspectRatio'

type PersistedCanvasBoardState = CanvasBoardSnapshot & {
  previewSnapshotDataUrl?: string
  previewSnapshotUpdatedAt?: number
  previewSnapshotVersion?: number
  updatedAt: number
}

const finiteNumberSchema = v.pipe(v.number(), v.finite())

const positiveFiniteNumberSchema = v.pipe(finiteNumberSchema, v.minValue(Number.MIN_VALUE))

const outputCountSchema = v.pipe(
  finiteNumberSchema,
  v.integer(),
  v.minValue(MIN_IMAGE_NODE_OUTPUT_COUNT),
  v.maxValue(MAX_IMAGE_NODE_OUTPUT_COUNT),
)

const positiveIntegerSchema = v.pipe(finiteNumberSchema, v.integer(), v.minValue(1))

const nonEmptyStringSchema = v.pipe(v.string(), v.minLength(1))

const workflowNodeOutputStatusSchema = v.picklist(['idle', 'running', 'ready', 'stale'])
const canvasConnectionNodeTypeSchema = v.picklist(['image', 'prompt', 'viewpoint', 'fusion'])
const canvasConnectionPortSchema = v.picklist(['image', 'prompt'])

const imageNodeOutputItemSchema = v.strictObject({
  url: nonEmptyStringSchema,
  ratio: positiveFiniteNumberSchema,
})

const promptNodeInputImageSchema = v.strictObject({
  url: nonEmptyStringSchema,
  ratio: positiveFiniteNumberSchema,
  id: nonEmptyStringSchema,
  sourceType: v.picklist(['node', 'external']),
  sourceId: nonEmptyStringSchema,
  sourceNodeId: v.optional(nonEmptyStringSchema),
})

const promptNodeItemSchema = v.strictObject({
  id: finiteNumberSchema,
  type: v.literal('prompt'),
  x: finiteNumberSchema,
  y: finiteNumberSchema,
  width: finiteNumberSchema,
  title: v.optional(v.string()),
  text: v.string(),
  outputCount: v.optional(outputCountSchema, DEFAULT_PROMPT_NODE_OUTPUT_COUNT),
  aspectRatio: v.optional(v.picklist(PROMPT_ASPECT_RATIO_OPTIONS), DEFAULT_PROMPT_ASPECT_RATIO),
  inputImages: v.optional(v.array(promptNodeInputImageSchema), () => []),
})

const viewpointNodeItemSchema = v.strictObject({
  id: finiteNumberSchema,
  type: v.literal('viewpoint'),
  x: finiteNumberSchema,
  y: finiteNumberSchema,
  width: finiteNumberSchema,
  title: v.optional(v.string()),
  activeCategory: v.picklist(['top', 'bottom', 'set']),
  selectedViewpoint: v.string(),
  lighting: v.picklist(['warm_white', 'afternoon', 'hard_light']),
  outputStatus: workflowNodeOutputStatusSchema,
  outputImages: v.array(imageNodeOutputItemSchema),
})

const imageNodeItemSchema = v.strictObject({
  id: finiteNumberSchema,
  type: v.literal('image-node'),
  x: finiteNumberSchema,
  y: finiteNumberSchema,
  width: finiteNumberSchema,
  title: v.optional(v.string()),
  sourceImageUrl: v.string(),
  sourceImageRatio: positiveFiniteNumberSchema,
  outputCount: v.optional(outputCountSchema),
  pendingTaskId: v.optional(nonEmptyStringSchema),
  outputStatus: workflowNodeOutputStatusSchema,
  outputImages: v.array(imageNodeOutputItemSchema),
})

const fusionNodeItemSchema = v.strictObject({
  id: finiteNumberSchema,
  type: v.literal('fusion'),
  x: finiteNumberSchema,
  y: finiteNumberSchema,
  width: finiteNumberSchema,
  title: v.optional(v.string()),
  prompt: v.optional(v.string(), ''),
  outputCount: v.optional(outputCountSchema, DEFAULT_PROMPT_NODE_OUTPUT_COUNT),
  aspectRatio: v.optional(v.picklist(PROMPT_ASPECT_RATIO_OPTIONS), DEFAULT_PROMPT_ASPECT_RATIO),
  blendPoint: v.optional(
    v.strictObject({
      x: v.pipe(finiteNumberSchema, v.minValue(0), v.maxValue(1)),
      y: v.pipe(finiteNumberSchema, v.minValue(0), v.maxValue(1)),
    }),
    () => ({ ...DEFAULT_FUSION_BLEND_POINT }),
  ),
  inputImages: v.optional(v.pipe(v.array(promptNodeInputImageSchema), v.maxLength(4)), () => []),
  outputStatus: workflowNodeOutputStatusSchema,
  outputImages: v.array(imageNodeOutputItemSchema),
})

const canvasConnectionSchema = v.strictObject({
  id: finiteNumberSchema,
  fromNodeType: canvasConnectionNodeTypeSchema,
  fromNodeId: finiteNumberSchema,
  fromPort: canvasConnectionPortSchema,
  toNodeType: canvasConnectionNodeTypeSchema,
  toNodeId: finiteNumberSchema,
  toPort: canvasConnectionPortSchema,
})

const canvasViewportStateSchema = v.strictObject({
  canvasX: finiteNumberSchema,
  canvasY: finiteNumberSchema,
  zoomLevel: finiteNumberSchema,
})

const canvasBoardSnapshotShape = v.strictObject({
  imageNodes: v.array(imageNodeItemSchema),
  promptNodes: v.array(promptNodeItemSchema),
  viewpointNodes: v.array(viewpointNodeItemSchema),
  fusionNodes: v.array(fusionNodeItemSchema),
  connections: v.array(canvasConnectionSchema),
  viewport: canvasViewportStateSchema,
})

export const canvasBoardSnapshotSchema = canvasBoardSnapshotShape satisfies v.GenericSchema<
  unknown,
  CanvasBoardSnapshot
>

const canvasBoardStateShape = v.strictObject({
  imageNodes: v.array(imageNodeItemSchema),
  promptNodes: v.array(promptNodeItemSchema),
  viewpointNodes: v.array(viewpointNodeItemSchema),
  fusionNodes: v.array(fusionNodeItemSchema),
  connections: v.array(canvasConnectionSchema),
  viewport: canvasViewportStateSchema,
  previewSnapshotDataUrl: v.optional(nonEmptyStringSchema),
  previewSnapshotUpdatedAt: v.optional(finiteNumberSchema),
  previewSnapshotVersion: v.optional(positiveIntegerSchema),
  updatedAt: finiteNumberSchema,
})

export const canvasBoardStateSchema = canvasBoardStateShape satisfies v.GenericSchema<
  unknown,
  PersistedCanvasBoardState
>
