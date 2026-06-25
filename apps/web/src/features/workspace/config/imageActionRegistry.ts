import { IMAGE_NODE_ACTION as WORKFLOW_IMAGE_NODE_ACTION } from '../../../shared/domain/imageActions'
import { WORKFLOW_FEATURE_REGISTRY_MAP } from '../../../shared/domain/workflowFeatureRegistry'
import type { WorkflowFeature } from '../../../shared/domain/workflowFeatures'
import { EXECUTION_FEATURE, EXECUTION_META, type ExecutionFeature } from './executionContract'

export const IMAGE_NODE_ACTION = {
  ...WORKFLOW_IMAGE_NODE_ACTION,
  imageInpaint: 'image-inpaint',
} as const

export type ImageNodeAction = (typeof IMAGE_NODE_ACTION)[keyof typeof IMAGE_NODE_ACTION]

export const IMAGE_ACTION_MODE = {
  directTask: 'direct-task',
  hdUpscaleTask: 'hd-upscale-task',
  canvasExtend: 'canvas-extend',
  localInpaint: 'local-inpaint',
} as const

export type ImageActionMode = (typeof IMAGE_ACTION_MODE)[keyof typeof IMAGE_ACTION_MODE]

export type ImageActionMenuIcon =
  | 'remove-bg'
  | 'style-variation'
  | 'sketch-to-image'
  | 'hd'
  | 'image-extend'
  | 'image-inpaint'

export const DEFAULT_IMAGE_ACTION_STRENGTH = 50

type WorkflowImageNodeAction = Extract<ImageNodeAction, WorkflowFeature>

type ImageActionExecutionConfig = {
  feature: ExecutionFeature
  prompt?: string
  promptPrefix?: string
}

type ImageActionStrengthConfig = {
  defaultValue: number
  paramPath?: 'variation.strength'
}

export type ImageActionRegistryItem = {
  action: ImageNodeAction
  mode: ImageActionMode
  label: string
  title: string
  icon: ImageActionMenuIcon
  execution?: ImageActionExecutionConfig
  strength?: ImageActionStrengthConfig
}

function getWorkflowImageActionText(action: WorkflowImageNodeAction): {
  label: string
  title: string
} {
  const meta = WORKFLOW_FEATURE_REGISTRY_MAP[action]
  return {
    label: meta.imageActionLabel ?? meta.adminTitle,
    title: meta.imageActionTitle ?? meta.imageActionLabel ?? meta.adminTitle,
  }
}

function createWorkflowImageAction<
  TAction extends WorkflowImageNodeAction,
  TConfig extends Omit<ImageActionRegistryItem, 'action' | 'label' | 'title'>,
>(action: TAction, config: TConfig): { action: TAction; label: string; title: string } & TConfig {
  const text = getWorkflowImageActionText(action)
  return {
    action,
    label: text.label,
    title: text.title,
    ...config,
  }
}

export const IMAGE_ACTION_REGISTRY_LIST = [
  createWorkflowImageAction(IMAGE_NODE_ACTION.removeBg, {
    mode: IMAGE_ACTION_MODE.directTask,
    icon: 'remove-bg',
    execution: EXECUTION_META[EXECUTION_FEATURE.removeBg],
  }),
  createWorkflowImageAction(IMAGE_NODE_ACTION.styleVariation, {
    mode: IMAGE_ACTION_MODE.directTask,
    icon: 'style-variation',
    execution: EXECUTION_META[EXECUTION_FEATURE.styleVariation],
    strength: {
      defaultValue: DEFAULT_IMAGE_ACTION_STRENGTH,
      paramPath: 'variation.strength',
    },
  }),
  createWorkflowImageAction(IMAGE_NODE_ACTION.sketchToImage, {
    mode: IMAGE_ACTION_MODE.directTask,
    icon: 'sketch-to-image',
    execution: EXECUTION_META[EXECUTION_FEATURE.sketchToImage],
  }),
  createWorkflowImageAction(IMAGE_NODE_ACTION.hdUpscale, {
    mode: IMAGE_ACTION_MODE.hdUpscaleTask,
    icon: 'hd',
    execution: EXECUTION_META[EXECUTION_FEATURE.hdUpscale],
    strength: {
      defaultValue: DEFAULT_IMAGE_ACTION_STRENGTH,
    },
  }),
  createWorkflowImageAction(IMAGE_NODE_ACTION.imageExtend, {
    mode: IMAGE_ACTION_MODE.canvasExtend,
    icon: 'image-extend',
    execution: EXECUTION_META[EXECUTION_FEATURE.imageExtend],
  }),
  {
    action: IMAGE_NODE_ACTION.imageInpaint,
    mode: IMAGE_ACTION_MODE.localInpaint,
    label: '局部重绘',
    title: '局部重绘',
    icon: 'image-inpaint',
  },
] as const satisfies readonly ImageActionRegistryItem[]

type ImageActionRegistryListItem = (typeof IMAGE_ACTION_REGISTRY_LIST)[number]

export type ImageActionRegistry = {
  readonly [TAction in ImageNodeAction]: Extract<ImageActionRegistryListItem, { action: TAction }>
}

export const IMAGE_ACTION_REGISTRY = Object.fromEntries(
  IMAGE_ACTION_REGISTRY_LIST.map((item) => [item.action, item]),
) as ImageActionRegistry

export type ImageActionByMode<TMode extends ImageActionMode> = Extract<
  ImageActionRegistryListItem,
  { mode: TMode }
>['action']

export type ImageDirectTaskAction = ImageActionByMode<typeof IMAGE_ACTION_MODE.directTask>

export type ImageHdUpscaleTaskAction = ImageActionByMode<typeof IMAGE_ACTION_MODE.hdUpscaleTask>

export type ImageTaskAction = ImageDirectTaskAction | ImageHdUpscaleTaskAction

export type ImageStrengthAction = Extract<
  ImageActionRegistryListItem,
  { readonly strength: ImageActionStrengthConfig }
>['action']

export const IMAGE_DIRECT_TASK_ACTIONS = IMAGE_ACTION_REGISTRY_LIST.filter(
  (
    item,
  ): item is Extract<ImageActionRegistryListItem, { mode: typeof IMAGE_ACTION_MODE.directTask }> =>
    item.mode === IMAGE_ACTION_MODE.directTask,
).map((item) => item.action) as readonly ImageDirectTaskAction[]

export const IMAGE_STRENGTH_ACTIONS = IMAGE_ACTION_REGISTRY_LIST.filter(
  (
    item,
  ): item is Extract<
    ImageActionRegistryListItem,
    { readonly strength: ImageActionStrengthConfig }
  > => 'strength' in item,
).map((item) => item.action) as readonly ImageStrengthAction[]

export const DEFAULT_IMAGE_ACTION_STRENGTHS = Object.fromEntries(
  IMAGE_STRENGTH_ACTIONS.map((action) => [
    action,
    IMAGE_ACTION_REGISTRY[action].strength.defaultValue,
  ]),
) as Record<ImageStrengthAction, number>

const IMAGE_STRENGTH_ACTION_SET: ReadonlySet<ImageNodeAction> = new Set(IMAGE_STRENGTH_ACTIONS)
const IMAGE_DIRECT_TASK_ACTION_SET: ReadonlySet<ImageNodeAction> = new Set(
  IMAGE_DIRECT_TASK_ACTIONS,
)

export function isImageStrengthAction(action: ImageNodeAction): action is ImageStrengthAction {
  return IMAGE_STRENGTH_ACTION_SET.has(action)
}

export function isImageDirectTaskAction(action: ImageNodeAction): action is ImageDirectTaskAction {
  return IMAGE_DIRECT_TASK_ACTION_SET.has(action)
}

export function isImageTaskAction(action: ImageNodeAction): action is ImageTaskAction {
  const mode = IMAGE_ACTION_REGISTRY[action].mode
  return mode === IMAGE_ACTION_MODE.directTask || mode === IMAGE_ACTION_MODE.hdUpscaleTask
}

export function getImageActionByMode<TMode extends ImageActionMode>(
  mode: TMode,
): ImageActionByMode<TMode> {
  const matched = IMAGE_ACTION_REGISTRY_LIST.filter((item) => item.mode === mode)
  if (matched.length !== 1) {
    throw new Error(`Expected exactly one image action for mode ${mode}`)
  }
  return matched[0].action as ImageActionByMode<TMode>
}

export const IMAGE_HD_UPSCALE_ACTION = getImageActionByMode(IMAGE_ACTION_MODE.hdUpscaleTask)
export const IMAGE_CANVAS_EXTEND_ACTION = getImageActionByMode(IMAGE_ACTION_MODE.canvasExtend)

export function getImageActionExecution(
  action: ImageNodeAction,
): ImageActionExecutionConfig | undefined {
  const item = IMAGE_ACTION_REGISTRY[action]
  return 'execution' in item ? item.execution : undefined
}

export function requireImageActionExecution(action: ImageNodeAction): ImageActionExecutionConfig {
  const execution = getImageActionExecution(action)
  if (!execution) {
    throw new Error(`Image action ${action} has no execution config`)
  }
  return execution
}

export type ImageActionMenuItem = {
  action: ImageNodeAction
  label: string
  title: string
  icon: ImageActionMenuIcon
  strengthAction?: ImageStrengthAction
}

export const IMAGE_ACTION_MENU_ITEMS = IMAGE_ACTION_REGISTRY_LIST.map((item) => ({
  action: item.action,
  label: item.label,
  title: item.title,
  icon: item.icon,
  ...('strength' in item ? { strengthAction: item.action as ImageStrengthAction } : {}),
})) as readonly ImageActionMenuItem[]
