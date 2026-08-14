import { IMAGE_NODE_ACTION } from '../../../../shared/domain/imageActions'
import type { WorkflowFeature } from '../../../../shared/domain/workflowFeatures'
import { EXECUTION_META, type ExecutionFeature } from './executionContract'

export const IMAGE_EXECUTION_MODE = {
  directTask: 'direct-task',
  hdUpscaleTask: 'hd-upscale-task',
  canvasExtend: 'canvas-extend',
} as const

export type ImageExecutionMode = (typeof IMAGE_EXECUTION_MODE)[keyof typeof IMAGE_EXECUTION_MODE]

export const DEFAULT_IMAGE_ACTION_STRENGTH = 50

export const IMAGE_DIRECT_TASK_ACTIONS = [
  IMAGE_NODE_ACTION.imageEdit,
  IMAGE_NODE_ACTION.removeBg,
  IMAGE_NODE_ACTION.styleVariation,
  IMAGE_NODE_ACTION.sketchToImage,
] as const

export type ImageDirectTaskAction = (typeof IMAGE_DIRECT_TASK_ACTIONS)[number]

export const IMAGE_HD_UPSCALE_ACTION = IMAGE_NODE_ACTION.hdUpscale
export type ImageHdUpscaleTaskAction = typeof IMAGE_HD_UPSCALE_ACTION

export const IMAGE_CANVAS_EXTEND_ACTION = IMAGE_NODE_ACTION.imageExtend
export type ImageCanvasExtendAction = typeof IMAGE_CANVAS_EXTEND_ACTION

export type ImageTaskAction = ImageDirectTaskAction | ImageHdUpscaleTaskAction
export type ImageExecutionAction = ImageTaskAction | ImageCanvasExtendAction

export type ImageActionStrengthConfig = {
  defaultValue: number
  paramPath?: 'variation.strength' | 'hd.clarity_strength'
}

export type ImageStrengthAction =
  typeof IMAGE_NODE_ACTION.styleVariation | typeof IMAGE_HD_UPSCALE_ACTION

export const IMAGE_STRENGTH_ACTIONS = [
  IMAGE_NODE_ACTION.styleVariation,
  IMAGE_HD_UPSCALE_ACTION,
] as const satisfies readonly ImageStrengthAction[]

export type ImageActionExecutionConfig = {
  action: ImageExecutionAction
  mode: ImageExecutionMode
  feature: ExecutionFeature
  prompt?: string
  promptPrefix?: string
  strength?: ImageActionStrengthConfig
}

type WorkflowImageExecutionAction = Extract<ImageExecutionAction, WorkflowFeature>

function createWorkflowExecutionConfig<TAction extends WorkflowImageExecutionAction>(
  action: TAction,
  config: Omit<ImageActionExecutionConfig, 'action' | 'feature'>,
): ImageActionExecutionConfig & { action: TAction } {
  const meta = EXECUTION_META[action] as {
    feature: ExecutionFeature
    prompt?: string
    promptPrefix?: string
  }
  return {
    action,
    feature: meta.feature,
    prompt: meta.prompt,
    promptPrefix: meta.promptPrefix,
    ...config,
  }
}

export const IMAGE_ACTION_EXECUTION_REGISTRY_LIST = [
  createWorkflowExecutionConfig(IMAGE_NODE_ACTION.imageEdit, {
    mode: IMAGE_EXECUTION_MODE.directTask,
  }),
  createWorkflowExecutionConfig(IMAGE_NODE_ACTION.removeBg, {
    mode: IMAGE_EXECUTION_MODE.directTask,
  }),
  createWorkflowExecutionConfig(IMAGE_NODE_ACTION.styleVariation, {
    mode: IMAGE_EXECUTION_MODE.directTask,
    strength: {
      defaultValue: DEFAULT_IMAGE_ACTION_STRENGTH,
      paramPath: 'variation.strength',
    },
  }),
  createWorkflowExecutionConfig(IMAGE_NODE_ACTION.sketchToImage, {
    mode: IMAGE_EXECUTION_MODE.directTask,
  }),
  createWorkflowExecutionConfig(IMAGE_NODE_ACTION.hdUpscale, {
    mode: IMAGE_EXECUTION_MODE.hdUpscaleTask,
    strength: {
      defaultValue: DEFAULT_IMAGE_ACTION_STRENGTH,
      paramPath: 'hd.clarity_strength',
    },
  }),
  createWorkflowExecutionConfig(IMAGE_NODE_ACTION.imageExtend, {
    mode: IMAGE_EXECUTION_MODE.canvasExtend,
  }),
] as const satisfies readonly ImageActionExecutionConfig[]

export type ImageActionExecutionRegistry = {
  readonly [TAction in ImageExecutionAction]: Extract<
    (typeof IMAGE_ACTION_EXECUTION_REGISTRY_LIST)[number],
    { action: TAction }
  >
}

export const IMAGE_ACTION_EXECUTION_REGISTRY = Object.fromEntries(
  IMAGE_ACTION_EXECUTION_REGISTRY_LIST.map((item) => [item.action, item]),
) as ImageActionExecutionRegistry

export const DEFAULT_IMAGE_ACTION_STRENGTHS = {
  [IMAGE_NODE_ACTION.styleVariation]: DEFAULT_IMAGE_ACTION_STRENGTH,
  [IMAGE_HD_UPSCALE_ACTION]: DEFAULT_IMAGE_ACTION_STRENGTH,
} as const satisfies Record<ImageStrengthAction, number>

const IMAGE_DIRECT_TASK_ACTION_SET: ReadonlySet<string> = new Set(IMAGE_DIRECT_TASK_ACTIONS)
const IMAGE_TASK_ACTION_SET: ReadonlySet<string> = new Set([
  ...IMAGE_DIRECT_TASK_ACTIONS,
  IMAGE_HD_UPSCALE_ACTION,
])
const IMAGE_STRENGTH_ACTION_SET: ReadonlySet<string> = new Set(IMAGE_STRENGTH_ACTIONS)

export function isImageDirectTaskAction(action: string): action is ImageDirectTaskAction {
  return IMAGE_DIRECT_TASK_ACTION_SET.has(action)
}

export function isImageTaskAction(action: string): action is ImageTaskAction {
  return IMAGE_TASK_ACTION_SET.has(action)
}

export function isImageStrengthAction(action: string): action is ImageStrengthAction {
  return IMAGE_STRENGTH_ACTION_SET.has(action)
}

function getImageActionExecution(action: string): ImageActionExecutionConfig | undefined {
  return IMAGE_ACTION_EXECUTION_REGISTRY[action as ImageExecutionAction]
}

export function requireImageActionExecution(
  action: ImageExecutionAction,
): ImageActionExecutionConfig {
  const execution = getImageActionExecution(action)
  if (!execution) {
    throw new Error(`Image action ${action} has no execution config`)
  }
  return execution
}

export function resolveHdClarityStrength(strength: number | undefined): number {
  return Number.isFinite(strength)
    ? Math.max(0, Math.min(100, Math.round(strength as number)))
    : DEFAULT_IMAGE_ACTION_STRENGTH
}
