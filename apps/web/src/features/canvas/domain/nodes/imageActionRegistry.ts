import { IMAGE_NODE_ACTION as WORKFLOW_IMAGE_NODE_ACTION } from '../../../../shared/domain/imageActions'
import { serviceLabelKey } from '../../../../shared/domain/serviceRegistry'
import type { Service } from '../../../../shared/domain/imageServices'
import {
  IMAGE_EXECUTION_MODE,
  isImageStrengthAction,
  type ImageStrengthAction,
} from '../execution-contracts/imageActionExecution.ts'

/** Copy resolver injected by the caller (application/presentation own i18n). */
export type ImageActionCopyResolver = (labelKey: string) => string

export const IMAGE_NODE_ACTION = {
  ...WORKFLOW_IMAGE_NODE_ACTION,
  inpaint: 'inpaint',
  retouch: 'retouch',
} as const

export type ImageNodeAction = (typeof IMAGE_NODE_ACTION)[keyof typeof IMAGE_NODE_ACTION]

export const IMAGE_ACTION_MODE = {
  directTask: IMAGE_EXECUTION_MODE.directTask,
  upscaleTask: IMAGE_EXECUTION_MODE.upscaleTask,
  outpaint: IMAGE_EXECUTION_MODE.outpaint,
  inpaint: 'inpaint',
  retouch: 'retouch',
} as const

export type ImageActionMode = (typeof IMAGE_ACTION_MODE)[keyof typeof IMAGE_ACTION_MODE]

export type ImageActionMenuIcon =
  'cutout' | 'variations' | 'render' | 'upscale' | 'outpaint' | 'inpaint' | 'retouch'

type WorkflowImageNodeAction = Extract<ImageNodeAction, Service>

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
    labelKey: serviceLabelKey(action),
  }
}

function localImageAction(
  action: typeof IMAGE_NODE_ACTION.inpaint | typeof IMAGE_NODE_ACTION.retouch,
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
  workflowImageAction(IMAGE_NODE_ACTION.upscale, {
    mode: IMAGE_ACTION_MODE.upscaleTask,
    icon: 'upscale',
  }),
  workflowImageAction(IMAGE_NODE_ACTION.outpaint, {
    mode: IMAGE_ACTION_MODE.outpaint,
    icon: 'outpaint',
  }),
  localImageAction(IMAGE_NODE_ACTION.inpaint, 'canvas.imageAction.inpaint', {
    mode: IMAGE_ACTION_MODE.inpaint,
    icon: 'inpaint',
  }),
  localImageAction(IMAGE_NODE_ACTION.retouch, 'canvas.imageAction.editImage', {
    mode: IMAGE_ACTION_MODE.retouch,
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
export function getImageActionMenuItems(
  translate: ImageActionCopyResolver,
): readonly ImageActionMenuItem[] {
  return IMAGE_ACTION_REGISTRY_LIST.map((item) => {
    const label = translate(item.labelKey)
    return {
      action: item.action,
      label,
      title: label,
      icon: item.icon,
      ...(isImageStrengthAction(item.action) ? { strengthAction: item.action } : {}),
    }
  })
}
