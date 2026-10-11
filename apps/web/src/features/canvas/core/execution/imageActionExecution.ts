import { IMAGE_NODE_ACTION } from '../nodes/imageActions'
import type { Service } from '../../../../core/contracts/imageServices'
import { EXECUTION_META, type ExecutionService } from './executionContract'

export const IMAGE_EXECUTION_MODE = {
  directTask: 'direct-task',
  upscaleTask: 'upscale-task',
  outpaint: 'outpaint',
} as const

type ImageExecutionMode = (typeof IMAGE_EXECUTION_MODE)[keyof typeof IMAGE_EXECUTION_MODE]

export const DEFAULT_IMAGE_ACTION_STRENGTH = 50

export const IMAGE_DIRECT_TASK_ACTIONS = [
  IMAGE_NODE_ACTION.cutout,
  IMAGE_NODE_ACTION.variations,
  IMAGE_NODE_ACTION.render,
] as const

export type ImageDirectTaskAction = (typeof IMAGE_DIRECT_TASK_ACTIONS)[number]

export const IMAGE_UPSCALE_ACTION = IMAGE_NODE_ACTION.upscale
type ImageUpscaleTaskAction = typeof IMAGE_UPSCALE_ACTION

export const IMAGE_OUTPAINT_ACTION = IMAGE_NODE_ACTION.outpaint
type ImageOutpaintAction = typeof IMAGE_OUTPAINT_ACTION

export type ImageTaskAction = ImageDirectTaskAction | ImageUpscaleTaskAction
export type ImageExecutionAction = ImageTaskAction | ImageOutpaintAction

export type ImageActionStrengthConfig = {
  defaultValue: number
  min: number
  max: number
  step: number
  paramPath?: 'variation.strength' | 'upscale.scale'
}

export type ImageStrengthAction = typeof IMAGE_NODE_ACTION.variations | typeof IMAGE_UPSCALE_ACTION

export const IMAGE_STRENGTH_ACTIONS = [
  IMAGE_NODE_ACTION.variations,
  IMAGE_UPSCALE_ACTION,
] as const satisfies readonly ImageStrengthAction[]

export type ImageActionExecutionConfig = {
  action: ImageExecutionAction
  mode: ImageExecutionMode
  service: ExecutionService
  prompt?: string | undefined
  promptPrefix?: string | undefined
  strength?: ImageActionStrengthConfig | undefined
}

type WorkflowImageExecutionAction = Extract<ImageExecutionAction, Service>

function createWorkflowExecutionConfig<TAction extends WorkflowImageExecutionAction>(
  action: TAction,
  config: Omit<ImageActionExecutionConfig, 'action' | 'service'>,
): ImageActionExecutionConfig & { action: TAction } {
  const meta = EXECUTION_META[action] as {
    service: ExecutionService
    prompt?: string
    promptPrefix?: string
  }
  return {
    action,
    service: meta.service,
    prompt: meta.prompt,
    promptPrefix: meta.promptPrefix,
    ...config,
  }
}

export const IMAGE_ACTION_EXECUTION_REGISTRY_LIST = [
  createWorkflowExecutionConfig(IMAGE_NODE_ACTION.cutout, {
    mode: IMAGE_EXECUTION_MODE.directTask,
  }),
  createWorkflowExecutionConfig(IMAGE_NODE_ACTION.variations, {
    mode: IMAGE_EXECUTION_MODE.directTask,
    strength: {
      defaultValue: DEFAULT_IMAGE_ACTION_STRENGTH,
      min: 0,
      max: 100,
      step: 1,
      paramPath: 'variation.strength',
    },
  }),
  createWorkflowExecutionConfig(IMAGE_NODE_ACTION.render, {
    mode: IMAGE_EXECUTION_MODE.directTask,
  }),
  createWorkflowExecutionConfig(IMAGE_NODE_ACTION.upscale, {
    mode: IMAGE_EXECUTION_MODE.upscaleTask,
    strength: {
      defaultValue: 2,
      min: 1,
      max: 4,
      step: 0.1,
      paramPath: 'upscale.scale',
    },
  }),
  createWorkflowExecutionConfig(IMAGE_NODE_ACTION.outpaint, {
    mode: IMAGE_EXECUTION_MODE.outpaint,
  }),
] as const satisfies readonly ImageActionExecutionConfig[]

type ImageActionExecutionRegistry = {
  readonly [TAction in ImageExecutionAction]: Extract<
    (typeof IMAGE_ACTION_EXECUTION_REGISTRY_LIST)[number],
    { action: TAction }
  >
}

export const IMAGE_ACTION_EXECUTION_REGISTRY = Object.fromEntries(
  IMAGE_ACTION_EXECUTION_REGISTRY_LIST.map((item) => [item.action, item]),
) as ImageActionExecutionRegistry

export const DEFAULT_IMAGE_ACTION_STRENGTHS = {
  [IMAGE_NODE_ACTION.variations]: DEFAULT_IMAGE_ACTION_STRENGTH,
  [IMAGE_UPSCALE_ACTION]: 2,
} as const satisfies Record<ImageStrengthAction, number>

const IMAGE_DIRECT_TASK_ACTION_SET: ReadonlySet<string> = new Set(IMAGE_DIRECT_TASK_ACTIONS)
const IMAGE_TASK_ACTION_SET: ReadonlySet<string> = new Set([
  ...IMAGE_DIRECT_TASK_ACTIONS,
  IMAGE_UPSCALE_ACTION,
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

const DEFAULT_UPSCALE_SCALE = 2

export function getImageActionStrengthConfig(
  action: ImageStrengthAction,
): ImageActionStrengthConfig {
  const config = requireImageActionExecution(action).strength
  if (!config) throw new Error(`Image action ${action} has no strength config`)
  return config
}

export function resolveUpscaleScale(strength?: number): number {
  if (!Number.isFinite(strength)) return DEFAULT_UPSCALE_SCALE
  return Math.round(Math.max(1, Math.min(4, strength as number)) * 10) / 10
}
