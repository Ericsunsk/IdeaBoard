import { IMAGE_NODE_ACTION as WORKFLOW_IMAGE_NODE_ACTION } from '../../../shared/domain/imageActions'
import { WORKFLOW_FEATURE_REGISTRY_MAP } from '../../../shared/domain/workflowFeatureRegistry'
import type { WorkflowFeature } from '../../../shared/domain/workflowFeatures'
import {
  IMAGE_EXECUTION_MODE,
  isImageStrengthAction,
  type ImageStrengthAction,
} from '../execution/contracts/imageActionExecution'

export const IMAGE_NODE_ACTION = {
  ...WORKFLOW_IMAGE_NODE_ACTION,
  imageInpaint: 'image-inpaint',
  localImageEditor: 'local-image-editor',
} as const

export type ImageNodeAction = (typeof IMAGE_NODE_ACTION)[keyof typeof IMAGE_NODE_ACTION]

export const IMAGE_ACTION_MODE = {
  directTask: IMAGE_EXECUTION_MODE.directTask,
  hdUpscaleTask: IMAGE_EXECUTION_MODE.hdUpscaleTask,
  canvasExtend: IMAGE_EXECUTION_MODE.canvasExtend,
  localInpaint: 'local-inpaint',
  localImageEditor: 'local-image-editor',
} as const

export type ImageActionMode = (typeof IMAGE_ACTION_MODE)[keyof typeof IMAGE_ACTION_MODE]

export type ImageActionMenuIcon =
  | 'remove-bg'
  | 'style-variation'
  | 'sketch-to-image'
  | 'hd'
  | 'image-extend'
  | 'image-inpaint'
  | 'adobe-photoshop'

type WorkflowImageNodeAction = Extract<ImageNodeAction, WorkflowFeature>

export type ImageActionRegistryItem = {
  action: ImageNodeAction
  mode: ImageActionMode
  label: string
  title: string
  icon: ImageActionMenuIcon
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
  }),
  createWorkflowImageAction(IMAGE_NODE_ACTION.styleVariation, {
    mode: IMAGE_ACTION_MODE.directTask,
    icon: 'style-variation',
  }),
  createWorkflowImageAction(IMAGE_NODE_ACTION.sketchToImage, {
    mode: IMAGE_ACTION_MODE.directTask,
    icon: 'sketch-to-image',
  }),
  createWorkflowImageAction(IMAGE_NODE_ACTION.hdUpscale, {
    mode: IMAGE_ACTION_MODE.hdUpscaleTask,
    icon: 'hd',
  }),
  createWorkflowImageAction(IMAGE_NODE_ACTION.imageExtend, {
    mode: IMAGE_ACTION_MODE.canvasExtend,
    icon: 'image-extend',
  }),
  {
    action: IMAGE_NODE_ACTION.imageInpaint,
    mode: IMAGE_ACTION_MODE.localInpaint,
    label: '局部重绘',
    title: '局部重绘',
    icon: 'image-inpaint',
  },
  {
    action: IMAGE_NODE_ACTION.localImageEditor,
    mode: IMAGE_ACTION_MODE.localImageEditor,
    label: '图像调整',
    title: '图像调整',
    icon: 'adobe-photoshop',
  },
] as const satisfies readonly ImageActionRegistryItem[]

type ImageActionRegistryListItem = (typeof IMAGE_ACTION_REGISTRY_LIST)[number]

export type ImageActionRegistry = {
  readonly [TAction in ImageNodeAction]: Extract<ImageActionRegistryListItem, { action: TAction }>
}

export const IMAGE_ACTION_REGISTRY = Object.fromEntries(
  IMAGE_ACTION_REGISTRY_LIST.map((item) => [item.action, item]),
) as ImageActionRegistry

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
  ...(isImageStrengthAction(item.action) ? { strengthAction: item.action } : {}),
})) as readonly ImageActionMenuItem[]
