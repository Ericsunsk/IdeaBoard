import {
  IMAGE_NODE_ACTION,
  type ImageNodeAction,
  type ImageStrengthAction,
} from '../types/imageActions'
import { WORKFLOW_FEATURE_REGISTRY_MAP } from '../../../shared/domain/workflowFeatureRegistry'

export type ImageActionMenuIcon =
  | 'remove-bg'
  | 'style-variation'
  | 'sketch-to-image'
  | 'hd'
  | 'image-extend'
  | 'image-inpaint'

type ImageActionMenuBase = {
  action: ImageNodeAction
  label: string
  title: string
  icon: ImageActionMenuIcon
}

type ImageActionMenuActionItem = ImageActionMenuBase & {
  strengthAction?: ImageStrengthAction
}

export type ImageActionMenuItem = ImageActionMenuActionItem

type WorkflowRegistryImageAction =
  | typeof IMAGE_NODE_ACTION.removeBg
  | typeof IMAGE_NODE_ACTION.styleVariation
  | typeof IMAGE_NODE_ACTION.sketchToImage
  | typeof IMAGE_NODE_ACTION.hdUpscale
  | typeof IMAGE_NODE_ACTION.imageExtend

function getImageActionText(action: WorkflowRegistryImageAction): { label: string; title: string } {
  const meta = WORKFLOW_FEATURE_REGISTRY_MAP[action]
  return {
    label: meta.imageActionLabel ?? meta.adminTitle,
    title: meta.imageActionTitle ?? meta.imageActionLabel ?? meta.adminTitle,
  }
}

const removeBgText = getImageActionText(IMAGE_NODE_ACTION.removeBg)
const styleVariationText = getImageActionText(IMAGE_NODE_ACTION.styleVariation)
const sketchToImageText = getImageActionText(IMAGE_NODE_ACTION.sketchToImage)
const hdUpscaleText = getImageActionText(IMAGE_NODE_ACTION.hdUpscale)
const imageExtendText = getImageActionText(IMAGE_NODE_ACTION.imageExtend)

export const IMAGE_ACTION_MENU_ITEMS: readonly ImageActionMenuItem[] = [
  {
    action: IMAGE_NODE_ACTION.removeBg,
    label: removeBgText.label,
    title: removeBgText.title,
    icon: 'remove-bg',
  },
  {
    action: IMAGE_NODE_ACTION.styleVariation,
    label: styleVariationText.label,
    title: styleVariationText.title,
    icon: 'style-variation',
    strengthAction: IMAGE_NODE_ACTION.styleVariation,
  },
  {
    action: IMAGE_NODE_ACTION.sketchToImage,
    label: sketchToImageText.label,
    title: sketchToImageText.title,
    icon: 'sketch-to-image',
  },
  {
    action: IMAGE_NODE_ACTION.hdUpscale,
    label: hdUpscaleText.label,
    title: hdUpscaleText.title,
    icon: 'hd',
    strengthAction: IMAGE_NODE_ACTION.hdUpscale,
  },
  {
    action: IMAGE_NODE_ACTION.imageExtend,
    label: imageExtendText.label,
    title: imageExtendText.title,
    icon: 'image-extend',
  },
  {
    action: IMAGE_NODE_ACTION.imageInpaint,
    label: '局部重绘',
    title: '局部重绘',
    icon: 'image-inpaint',
  },
] as const
