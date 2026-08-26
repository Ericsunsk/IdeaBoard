import { IMAGE_NODE_ACTION as WORKFLOW_IMAGE_NODE_ACTION } from '../../../shared/domain/imageActions'
import { workflowFeatureLabelKey } from '../../../shared/domain/workflowFeatureRegistry'
import { i18n } from '../../../shared/i18n'
import type { WorkflowFeature } from '../../../shared/domain/workflowFeatures'
import { IMAGE_EXECUTION_MODE, isImageStrengthAction, type ImageStrengthAction } from '../execution'

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
  inpaint: 'inpaint',
  localImageEditor: 'local-image-editor',
} as const

export type ImageActionMode = (typeof IMAGE_ACTION_MODE)[keyof typeof IMAGE_ACTION_MODE]

export type ImageActionMenuIcon =
  | 'cutout'
  | 'variations'
  | 'render'
  | 'hd'
  | 'outpaint'
  | 'image-inpaint'
  | 'retouch'

type WorkflowImageNodeAction = Extract<ImageNodeAction, WorkflowFeature>

/**
 * Registry facts only: mode + icon + label key. Copy resolves read-time so it
 * never freezes at module import.
 */
export type ImageActionRegistryItem = {
  action: ImageNodeAction
  mode: ImageActionMode
  labelKey: string
  icon: ImageActionMenuIcon
}

function workflowImageAction(
  action: WorkflowImageNodeAction,
  config: { mode: ImageActionMode; icon: ImageActionMenuIcon },
): ImageActionRegistryItem {
  return {
    action,
    ...config,
    // The shared registry derives the locale key from the slug.
    labelKey: workflowFeatureLabelKey(action),
  }
}

function localImageAction(
  action: typeof IMAGE_NODE_ACTION.imageInpaint | typeof IMAGE_NODE_ACTION.localImageEditor,
  labelKey: string,
  config: { mode: ImageActionMode; icon: ImageActionMenuIcon },
): ImageActionRegistryItem {
  return { action, ...config, labelKey }
}

export const IMAGE_ACTION_REGISTRY_LIST: readonly ImageActionRegistryItem[] = [
  workflowImageAction(IMAGE_NODE_ACTION.cutout, {
    mode: IMAGE_ACTION_MODE.directTask,
    icon: 'cutout',
  }),
  workflowImageAction(IMAGE_NODE_ACTION.variations, {
    mode: IMAGE_ACTION_MODE.directTask,
    icon: 'variations',
  }),
  workflowImageAction(IMAGE_NODE_ACTION.render, {
    mode: IMAGE_ACTION_MODE.directTask,
    icon: 'render',
  }),
  workflowImageAction(IMAGE_NODE_ACTION.hdUpscale, {
    mode: IMAGE_ACTION_MODE.hdUpscaleTask,
    icon: 'hd',
  }),
  workflowImageAction(IMAGE_NODE_ACTION.outpaint, {
    mode: IMAGE_ACTION_MODE.canvasExtend,
    icon: 'outpaint',
  }),
  localImageAction(IMAGE_NODE_ACTION.imageInpaint, 'canvas.imageAction.inpaint', {
    mode: IMAGE_ACTION_MODE.inpaint,
    icon: 'image-inpaint',
  }),
  localImageAction(IMAGE_NODE_ACTION.localImageEditor, 'canvas.imageAction.editImage', {
    mode: IMAGE_ACTION_MODE.localImageEditor,
    icon: 'retouch',
  }),
]

export type ImageActionRegistry = Readonly<Record<ImageNodeAction, ImageActionRegistryItem>>

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

/** Menu copy resolved at call time; callers invoke inside render/computed. */
export function getImageActionMenuItems(): readonly ImageActionMenuItem[] {
  return IMAGE_ACTION_REGISTRY_LIST.map((item) => {
    const label = i18n.global.t(item.labelKey)
    return {
      action: item.action,
      label,
      title: label,
      icon: item.icon,
      ...(isImageStrengthAction(item.action) ? { strengthAction: item.action } : {}),
    }
  })
}
