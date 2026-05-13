import { z } from 'zod'
import {
  ENHANCE_EXTEND_PERCENT_MAX,
  ENHANCE_EXTEND_PERCENT_MIN,
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

type PersistedCanvasBoardState = CanvasBoardSnapshot & {
  previewSnapshotDataUrl?: string
  previewSnapshotUpdatedAt?: number
  updatedAt: number
}

const finiteNumberSchema = z.number().finite()

const workflowNodeOutputStatusSchema = z.enum(['idle', 'running', 'ready', 'stale'])
const canvasConnectionNodeTypeSchema = z.enum([
  'image',
  'prompt',
  'mask',
  'color',
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
})

const promptNodeItemSchema = z.object({
  id: finiteNumberSchema,
  type: z.literal('prompt'),
  x: finiteNumberSchema,
  y: finiteNumberSchema,
  width: finiteNumberSchema,
  text: z.string(),
})

const modelNodeItemSchema = z.object({
  id: finiteNumberSchema,
  type: z.literal('model'),
  x: finiteNumberSchema,
  y: finiteNumberSchema,
  width: finiteNumberSchema,
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
})

const viewpointNodeItemSchema = z.object({
  id: finiteNumberSchema,
  type: z.literal('viewpoint'),
  x: finiteNumberSchema,
  y: finiteNumberSchema,
  width: finiteNumberSchema,
  activeCategory: z.enum(['top', 'bottom', 'set']),
  selectedViewpoint: z.string(),
  lighting: z.enum(['warm_white', 'afternoon', 'hard_light']),
  outputStatus: workflowNodeOutputStatusSchema,
  outputImages: z.array(imageNodeOutputItemSchema),
})

const maskNodeItemSchema = z.object({
  id: finiteNumberSchema,
  type: z.literal('mask'),
  x: finiteNumberSchema,
  y: finiteNumberSchema,
  width: finiteNumberSchema,
  sourceImageUrl: z.string(),
  sourceImageRatio: finiteNumberSchema.positive(),
  executionMaskDataUrl: z.string(),
  maskDataUrl: z.string(),
  brushSize: finiteNumberSchema.min(2).max(64),
  drawMode: z.enum(['paint', 'erase']),
  refineAmount: finiteNumberSchema.min(-100).max(100),
  invertMask: z.boolean(),
})

const imageNodeItemSchema = z.object({
  id: finiteNumberSchema,
  type: z.literal('image-node'),
  x: finiteNumberSchema,
  y: finiteNumberSchema,
  width: finiteNumberSchema,
  sourceImageUrl: z.string(),
  sourceImageRatio: finiteNumberSchema.positive(),
  outputStatus: workflowNodeOutputStatusSchema,
  outputImages: z.array(imageNodeOutputItemSchema),
})

const colorNodeItemSchema = z.object({
  id: finiteNumberSchema,
  type: z.literal('color'),
  x: finiteNumberSchema,
  y: finiteNumberSchema,
  width: finiteNumberSchema,
  colors: z.array(z.string()),
  strength: finiteNumberSchema.min(0).max(100),
  outputStatus: workflowNodeOutputStatusSchema,
  outputImages: z.array(imageNodeOutputItemSchema),
})

const editNodeItemSchema = z.object({
  id: finiteNumberSchema,
  type: z.literal('edit'),
  x: finiteNumberSchema,
  y: finiteNumberSchema,
  width: finiteNumberSchema,
  mode: z.enum(['partial-edit', 'bg-swap', 'face-swap']),
  outputStatus: workflowNodeOutputStatusSchema,
  outputImages: z.array(imageNodeOutputItemSchema),
})

const fusionNodeItemSchema = z.object({
  id: finiteNumberSchema,
  type: z.literal('fusion'),
  x: finiteNumberSchema,
  y: finiteNumberSchema,
  width: finiteNumberSchema,
  blendRatio: finiteNumberSchema.min(0).max(100),
  outputStatus: workflowNodeOutputStatusSchema,
  outputImages: z.array(imageNodeOutputItemSchema),
})

const enhanceNodeItemSchema = z.object({
  id: finiteNumberSchema,
  type: z.literal('enhance'),
  x: finiteNumberSchema,
  y: finiteNumberSchema,
  width: finiteNumberSchema,
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
})

const wearNodeItemSchema = z.object({
  id: finiteNumberSchema,
  type: z.literal('wear'),
  x: finiteNumberSchema,
  y: finiteNumberSchema,
  width: finiteNumberSchema,
  tileMode: z.enum(['seamless', 'single']),
  rotation: finiteNumberSchema.min(WEAR_ROTATION_MIN).max(WEAR_ROTATION_MAX),
  scale: finiteNumberSchema.min(WEAR_SCALE_MIN).max(WEAR_SCALE_MAX),
  opacity: finiteNumberSchema.min(WEAR_OPACITY_MIN).max(WEAR_OPACITY_MAX),
  outputStatus: workflowNodeOutputStatusSchema,
  outputImages: z.array(imageNodeOutputItemSchema),
})

const canvasConnectionSchema = z.object({
  id: finiteNumberSchema,
  fromNodeType: canvasConnectionNodeTypeSchema,
  fromNodeId: finiteNumberSchema,
  fromPort: canvasConnectionPortSchema,
  toNodeType: canvasConnectionNodeTypeSchema,
  toNodeId: finiteNumberSchema,
  toPort: canvasConnectionPortSchema,
})

const canvasViewportStateSchema = z.object({
  canvasX: finiteNumberSchema,
  canvasY: finiteNumberSchema,
  zoomLevel: finiteNumberSchema,
})

// Board state parsing intentionally uses unknown-key strip mode.
// Reason: remote/local legacy snapshots may contain stale fields; we keep known data
// and discard unsupported fields instead of hard-failing workspace hydration.
export const canvasBoardSnapshotSchema = z
  .object({
    imageNodes: z.array(imageNodeItemSchema),
    promptNodes: z.array(promptNodeItemSchema),
    modelNodes: z.array(modelNodeItemSchema),
    viewpointNodes: z.array(viewpointNodeItemSchema),
    maskNodes: z.array(maskNodeItemSchema),
    colorNodes: z.array(colorNodeItemSchema),
    editNodes: z.array(editNodeItemSchema),
    fusionNodes: z.array(fusionNodeItemSchema),
    enhanceNodes: z.array(enhanceNodeItemSchema),
    wearNodes: z.array(wearNodeItemSchema),
    connections: z.array(canvasConnectionSchema),
    viewport: canvasViewportStateSchema,
  })
  .strip() satisfies z.ZodType<CanvasBoardSnapshot>

export const canvasBoardStateSchema = canvasBoardSnapshotSchema
  .extend({
    previewSnapshotDataUrl: z.string().min(1).optional(),
    previewSnapshotUpdatedAt: finiteNumberSchema.optional(),
    updatedAt: finiteNumberSchema,
  })
  .strip() satisfies z.ZodType<PersistedCanvasBoardState>
