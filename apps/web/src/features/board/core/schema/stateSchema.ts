import * as v from 'valibot'
import { opaqueIdPattern } from '../../../../core/utils/opaqueIds'
import { OUTPUT_RESOLUTION_OPTIONS } from '../../../../core/contracts/generationContract'
import {
  DEFAULT_FUSION_BLEND_POINT,
  DEFAULT_FUSION_NODE_OUTPUT_COUNT,
  DEFAULT_IMAGE_NODE_OUTPUT_COUNT,
  DEFAULT_PROMPT_ASPECT_RATIO,
  DEFAULT_PROMPT_NODE_OUTPUT_COUNT,
  MAX_IMAGE_NODE_OUTPUT_COUNT,
  MIN_IMAGE_NODE_OUTPUT_COUNT,
  BOARD_STATE_SCHEMA_VERSION,
  type BoardSnapshot,
  type BoardState,
  type CanvasGroupId,
  type CanvasGroupNodeId,
} from '../document/nodes'
import { PROMPT_ASPECT_RATIO_OPTIONS } from '../document/promptAspectRatio'

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
const outputResolutionSchema = v.picklist(OUTPUT_RESOLUTION_OPTIONS)
const canvasConnectionNodeTypeSchema = v.picklist(['image', 'prompt', 'fusion'])
const canvasConnectionPortSchema = v.picklist(['image', 'prompt'])
const canvasGroupIdSchema = v.pipe(
  v.string(),
  v.regex(/^group-\d+$/),
  v.transform((value) => value as CanvasGroupId),
)
const canvasGroupNodeIdSchema = v.pipe(
  v.string(),
  v.regex(/^(?:image|prompt|fusion)-\d+(?:\.\d+)?$/),
  v.transform((value) => value as CanvasGroupNodeId),
)

const mediaRefSchema = v.strictObject({
  assetId: nonEmptyStringSchema,
  derivativeTier: v.picklist(['thumb', 'preview', 'large-preview']),
})

const imageNodeOutputItemSchema = v.strictObject({
  url: nonEmptyStringSchema,
  ratio: positiveFiniteNumberSchema,
  mediaRef: v.optional(mediaRefSchema),
  mediaAssetId: v.optional(positiveIntegerSchema),
})

const libraryAssetOpaqueIdSchema = v.pipe(v.string(), v.regex(opaqueIdPattern('lasset')))

const libraryAssetIdSchema = libraryAssetOpaqueIdSchema

const libraryImportFactSchema = v.union([
  v.strictObject({
    state: v.literal('pending'),
    libraryAssetId: libraryAssetIdSchema,
  }),
  v.strictObject({
    state: v.literal('failed'),
    libraryAssetId: libraryAssetIdSchema,
    failureMessage: v.pipe(nonEmptyStringSchema, v.maxLength(500)),
  }),
])

const imageRetouchRecipeStepSchema = v.union([
  v.strictObject({
    kind: v.literal('hsl'),
    params: v.strictObject({
      hueShift: finiteNumberSchema,
      saturationScale: finiteNumberSchema,
      lightnessOffset: finiteNumberSchema,
    }),
  }),
  v.strictObject({
    kind: v.literal('threshold'),
    threshold: finiteNumberSchema,
    invert: v.boolean(),
  }),
  v.strictObject({
    kind: v.literal('channel-lut'),
    luts: v.strictObject({
      red: v.pipe(v.array(v.pipe(finiteNumberSchema, v.integer())), v.length(256)),
      green: v.pipe(v.array(v.pipe(finiteNumberSchema, v.integer())), v.length(256)),
      blue: v.pipe(v.array(v.pipe(finiteNumberSchema, v.integer())), v.length(256)),
    }),
  }),
  v.strictObject({
    kind: v.literal('sharpen'),
    params: v.strictObject({
      amount: finiteNumberSchema,
      radius: finiteNumberSchema,
      threshold: finiteNumberSchema,
      protectColor: v.boolean(),
    }),
  }),
])

const imageRetouchRecipeSchema = v.strictObject({
  version: v.literal(1),
  sourceAssetId: nonEmptyStringSchema,
  sourceMimeType: v.optional(nonEmptyStringSchema),
  plan: v.array(imageRetouchRecipeStepSchema),
})

const boardStateMediaRefSchema = v.strictObject({
  mediaAssetId: positiveIntegerSchema,
  refType: v.picklist(['board', 'board_node']),
  refId: v.pipe(nonEmptyStringSchema, v.maxLength(128)),
  role: v.optional(nonEmptyStringSchema),
})

const promptNodeInputImageSchema = v.strictObject({
  mediaRef: v.optional(mediaRefSchema),
  url: nonEmptyStringSchema,
  ratio: positiveFiniteNumberSchema,
  mediaAssetId: v.optional(positiveIntegerSchema),
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
  outputResolution: v.optional(outputResolutionSchema),
  aspectRatio: v.optional(v.picklist(PROMPT_ASPECT_RATIO_OPTIONS), DEFAULT_PROMPT_ASPECT_RATIO),
  inputImages: v.optional(v.array(promptNodeInputImageSchema), () => []),
})

const imageNodeItemSchema = v.strictObject({
  id: finiteNumberSchema,
  type: v.literal('image-node'),
  x: finiteNumberSchema,
  y: finiteNumberSchema,
  width: finiteNumberSchema,
  title: v.optional(v.string()),
  sourceImageUrl: v.string(),
  sourceMediaRef: v.optional(mediaRefSchema),
  sourceMediaAssetId: v.optional(positiveIntegerSchema),
  retouchRecipe: v.optional(imageRetouchRecipeSchema),
  sourceImageRatio: positiveFiniteNumberSchema,
  outputCount: v.optional(outputCountSchema, DEFAULT_IMAGE_NODE_OUTPUT_COUNT),
  outputResolution: v.optional(outputResolutionSchema),
  pendingTaskId: v.optional(nonEmptyStringSchema),
  pendingResultIndex: v.optional(v.pipe(v.number(), v.integer(), v.minValue(0))),
  pendingClientActionId: v.optional(nonEmptyStringSchema),
  libraryImport: v.optional(libraryImportFactSchema),
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
  outputCount: v.optional(outputCountSchema, DEFAULT_FUSION_NODE_OUTPUT_COUNT),
  outputResolution: v.optional(outputResolutionSchema),
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

const canvasGroupSchema = v.strictObject({
  id: canvasGroupIdSchema,
  nodeIds: v.array(canvasGroupNodeIdSchema),
  x: finiteNumberSchema,
  y: finiteNumberSchema,
  width: positiveFiniteNumberSchema,
  height: positiveFiniteNumberSchema,
})

const boardSnapshotShape = v.strictObject({
  schemaVersion: v.literal(BOARD_STATE_SCHEMA_VERSION),
  imageNodes: v.array(imageNodeItemSchema),
  promptNodes: v.array(promptNodeItemSchema),
  fusionNodes: v.array(fusionNodeItemSchema),
  connections: v.array(canvasConnectionSchema),
  groups: v.array(canvasGroupSchema),
  mediaRefs: v.optional(v.array(boardStateMediaRefSchema)),
})

export const boardSnapshotSchema = boardSnapshotShape satisfies v.GenericSchema<
  unknown,
  BoardSnapshot
>

const boardStateShape = v.strictObject({
  schemaVersion: v.literal(BOARD_STATE_SCHEMA_VERSION),
  imageNodes: v.array(imageNodeItemSchema),
  promptNodes: v.array(promptNodeItemSchema),
  fusionNodes: v.array(fusionNodeItemSchema),
  connections: v.array(canvasConnectionSchema),
  groups: v.array(canvasGroupSchema),
  mediaRefs: v.optional(v.array(boardStateMediaRefSchema)),
  updatedAt: finiteNumberSchema,
})

export const boardStateSchema = boardStateShape satisfies v.GenericSchema<unknown, BoardState>
