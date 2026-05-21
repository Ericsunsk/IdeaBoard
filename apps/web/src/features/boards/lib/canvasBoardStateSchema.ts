import { z } from 'zod'
import {
  DEFAULT_FUSION_BLEND_POINT,
  DEFAULT_PROMPT_ASPECT_RATIO,
  DEFAULT_PROMPT_NODE_OUTPUT_COUNT,
  ENHANCE_EXTEND_PERCENT_MAX,
  ENHANCE_EXTEND_PERCENT_MIN,
  MAX_IMAGE_NODE_OUTPUT_COUNT,
  MIN_IMAGE_NODE_OUTPUT_COUNT,
  MODEL_ANGLE_YAW_MAX,
  MODEL_ANGLE_YAW_MIN,
  MODEL_POSE_STRENGTH_MAX,
  MODEL_POSE_STRENGTH_MIN,
  MODEL_REFERENCE_STRENGTH_MAX,
  MODEL_REFERENCE_STRENGTH_MIN,
  WEAR_OPACITY_MAX,
  WEAR_OPACITY_MIN,
  WEAR_ROTATION_MAX,
  WEAR_ROTATION_MIN,
  WEAR_SCALE_MAX,
  WEAR_SCALE_MIN,
  type CanvasBoardSnapshot,
} from '../../workspace/types/canvasNodes'
import { PROMPT_ASPECT_RATIO_OPTIONS } from '../../workspace/config/promptAspectRatio'

type PersistedCanvasBoardState = CanvasBoardSnapshot & {
  previewSnapshotDataUrl?: string
  previewSnapshotUpdatedAt?: number
  previewSnapshotVersion?: number
  updatedAt: number
}

const finiteNumberSchema = z.number().finite()

const workflowNodeOutputStatusSchema = z.enum(['idle', 'running', 'ready', 'stale'])
const canvasConnectionNodeTypeSchema = z.enum([
  'image',
  'prompt',
  'mask',
  'model',
  'viewpoint',
  'edit',
  'fusion',
  'enhance',
  'wear',
])
const canvasConnectionPortSchema = z.enum(['image', 'mask', 'prompt', 'imageA', 'imageB'])

const imageNodeOutputItemSchema = z.object({
  url: z.string().min(1),
  ratio: finiteNumberSchema.positive(),
}).strict()

const promptNodeInputImageSchema = imageNodeOutputItemSchema.extend({
  id: z.string().min(1),
  sourceType: z.enum(['node', 'external']),
  sourceId: z.string().min(1),
  sourceNodeId: z.string().min(1).optional(),
}).strict()

const promptNodeItemSchema = z.object({
  id: finiteNumberSchema,
  type: z.literal('prompt'),
  x: finiteNumberSchema,
  y: finiteNumberSchema,
  width: finiteNumberSchema,
  title: z.string().optional(),
  text: z.string(),
  outputCount: finiteNumberSchema
    .int()
    .min(MIN_IMAGE_NODE_OUTPUT_COUNT)
    .max(MAX_IMAGE_NODE_OUTPUT_COUNT)
    .default(DEFAULT_PROMPT_NODE_OUTPUT_COUNT),
  aspectRatio: z.enum(PROMPT_ASPECT_RATIO_OPTIONS).default(DEFAULT_PROMPT_ASPECT_RATIO),
  inputImages: z.array(promptNodeInputImageSchema).default([]),
}).strict()

const modelNodeItemSchema = z.object({
  id: finiteNumberSchema,
  type: z.literal('model'),
  x: finiteNumberSchema,
  y: finiteNumberSchema,
  width: finiteNumberSchema,
  title: z.string().optional(),
  angleSpec: z.object({
    yaw: finiteNumberSchema.min(MODEL_ANGLE_YAW_MIN).max(MODEL_ANGLE_YAW_MAX),
    framing: z.enum(['full', 'half', 'portrait']),
  }),
  modelStrength: finiteNumberSchema
    .min(MODEL_REFERENCE_STRENGTH_MIN)
    .max(MODEL_REFERENCE_STRENGTH_MAX),
  poseStrength: finiteNumberSchema.min(MODEL_POSE_STRENGTH_MIN).max(MODEL_POSE_STRENGTH_MAX),
  background: z.string(),
  outputStatus: workflowNodeOutputStatusSchema,
  outputImages: z.array(imageNodeOutputItemSchema),
}).strict()

const viewpointNodeItemSchema = z.object({
  id: finiteNumberSchema,
  type: z.literal('viewpoint'),
  x: finiteNumberSchema,
  y: finiteNumberSchema,
  width: finiteNumberSchema,
  title: z.string().optional(),
  activeCategory: z.enum(['top', 'bottom', 'set']),
  selectedViewpoint: z.string(),
  lighting: z.enum(['warm_white', 'afternoon', 'hard_light']),
  outputStatus: workflowNodeOutputStatusSchema,
  outputImages: z.array(imageNodeOutputItemSchema),
}).strict()

const maskNodeItemSchema = z.object({
  id: finiteNumberSchema,
  type: z.literal('mask'),
  x: finiteNumberSchema,
  y: finiteNumberSchema,
  width: finiteNumberSchema,
  title: z.string().optional(),
  sourceImageUrl: z.string(),
  sourceImageRatio: finiteNumberSchema.positive(),
  executionMaskDataUrl: z.string(),
  maskDataUrl: z.string(),
  brushSize: finiteNumberSchema.min(2).max(64),
  drawMode: z.enum(['paint', 'erase']),
  refineAmount: finiteNumberSchema.min(-100).max(100),
  invertMask: z.boolean(),
}).strict()

const imageNodeItemSchema = z.object({
  id: finiteNumberSchema,
  type: z.literal('image-node'),
  x: finiteNumberSchema,
  y: finiteNumberSchema,
  width: finiteNumberSchema,
  title: z.string().optional(),
  sourceImageUrl: z.string(),
  sourceImageRatio: finiteNumberSchema.positive(),
  outputCount: finiteNumberSchema
    .int()
    .min(MIN_IMAGE_NODE_OUTPUT_COUNT)
    .max(MAX_IMAGE_NODE_OUTPUT_COUNT)
    .optional(),
  outputStatus: workflowNodeOutputStatusSchema,
  outputImages: z.array(imageNodeOutputItemSchema),
}).strict()

const editNodeItemSchema = z.object({
  id: finiteNumberSchema,
  type: z.literal('edit'),
  x: finiteNumberSchema,
  y: finiteNumberSchema,
  width: finiteNumberSchema,
  title: z.string().optional(),
  mode: z.enum(['partial-edit', 'bg-swap', 'face-swap']),
  outputStatus: workflowNodeOutputStatusSchema,
  outputImages: z.array(imageNodeOutputItemSchema),
}).strict()

const fusionNodeItemSchema = z.object({
  id: finiteNumberSchema,
  type: z.literal('fusion'),
  x: finiteNumberSchema,
  y: finiteNumberSchema,
  width: finiteNumberSchema,
  title: z.string().optional(),
  prompt: z.string().default(''),
  outputCount: finiteNumberSchema
    .int()
    .min(MIN_IMAGE_NODE_OUTPUT_COUNT)
    .max(MAX_IMAGE_NODE_OUTPUT_COUNT)
    .default(DEFAULT_PROMPT_NODE_OUTPUT_COUNT),
  blendPoint: z.object({
    x: finiteNumberSchema.min(0).max(1),
    y: finiteNumberSchema.min(0).max(1),
  }).default(DEFAULT_FUSION_BLEND_POINT),
  inputImages: z.array(promptNodeInputImageSchema).max(4).default([]),
  outputStatus: workflowNodeOutputStatusSchema,
  outputImages: z.array(imageNodeOutputItemSchema),
}).strict()

const enhanceNodeItemSchema = z.object({
  id: finiteNumberSchema,
  type: z.literal('enhance'),
  x: finiteNumberSchema,
  y: finiteNumberSchema,
  width: finiteNumberSchema,
  title: z.string().optional(),
  mode: z.enum(['hd-upscale', 'canvas-extend']),
  upscaleMode: z.enum(['base', 'enhancement']),
  upscaleFactor: z.union([z.literal(2), z.literal(3), z.literal(4)]),
  outputFormat: z.enum(['jpg', 'png']),
  outputQuality: finiteNumberSchema.min(30).max(100),
  extendTop: finiteNumberSchema.min(ENHANCE_EXTEND_PERCENT_MIN).max(ENHANCE_EXTEND_PERCENT_MAX),
  extendRight: finiteNumberSchema.min(ENHANCE_EXTEND_PERCENT_MIN).max(ENHANCE_EXTEND_PERCENT_MAX),
  extendBottom: finiteNumberSchema.min(ENHANCE_EXTEND_PERCENT_MIN).max(ENHANCE_EXTEND_PERCENT_MAX),
  extendLeft: finiteNumberSchema.min(ENHANCE_EXTEND_PERCENT_MIN).max(ENHANCE_EXTEND_PERCENT_MAX),
  outputStatus: workflowNodeOutputStatusSchema,
  outputImages: z.array(imageNodeOutputItemSchema),
}).strict()

const wearNodeItemSchema = z.object({
  id: finiteNumberSchema,
  type: z.literal('wear'),
  x: finiteNumberSchema,
  y: finiteNumberSchema,
  width: finiteNumberSchema,
  title: z.string().optional(),
  tileMode: z.enum(['seamless', 'single']),
  rotation: finiteNumberSchema.min(WEAR_ROTATION_MIN).max(WEAR_ROTATION_MAX),
  scale: finiteNumberSchema.min(WEAR_SCALE_MIN).max(WEAR_SCALE_MAX),
  opacity: finiteNumberSchema.min(WEAR_OPACITY_MIN).max(WEAR_OPACITY_MAX),
  outputStatus: workflowNodeOutputStatusSchema,
  outputImages: z.array(imageNodeOutputItemSchema),
}).strict()

const canvasConnectionSchema = z.object({
  id: finiteNumberSchema,
  fromNodeType: canvasConnectionNodeTypeSchema,
  fromNodeId: finiteNumberSchema,
  fromPort: canvasConnectionPortSchema,
  toNodeType: canvasConnectionNodeTypeSchema,
  toNodeId: finiteNumberSchema,
  toPort: canvasConnectionPortSchema,
}).strict()

const canvasViewportStateSchema = z.object({
  canvasX: finiteNumberSchema,
  canvasY: finiteNumberSchema,
  zoomLevel: finiteNumberSchema,
}).strict()

export const canvasBoardSnapshotSchema = z
  .object({
    imageNodes: z.array(imageNodeItemSchema),
    promptNodes: z.array(promptNodeItemSchema),
    modelNodes: z.array(modelNodeItemSchema),
    viewpointNodes: z.array(viewpointNodeItemSchema),
    maskNodes: z.array(maskNodeItemSchema),
    editNodes: z.array(editNodeItemSchema),
    fusionNodes: z.array(fusionNodeItemSchema),
    enhanceNodes: z.array(enhanceNodeItemSchema),
    wearNodes: z.array(wearNodeItemSchema),
    connections: z.array(canvasConnectionSchema),
    viewport: canvasViewportStateSchema,
  })
  .strict() satisfies z.ZodType<CanvasBoardSnapshot>

export const canvasBoardStateSchema = canvasBoardSnapshotSchema
  .extend({
    previewSnapshotDataUrl: z.string().min(1).optional(),
    previewSnapshotUpdatedAt: finiteNumberSchema.optional(),
    previewSnapshotVersion: finiteNumberSchema.int().positive().optional(),
    updatedAt: finiteNumberSchema,
  })
  .strict() satisfies z.ZodType<PersistedCanvasBoardState>
