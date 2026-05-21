import { IMAGE_NODE_ACTION, type ImageNodeAction, type ImageStrengthAction } from '../types/imageActions'

export type ImageActionMenuIcon = 'remove-bg' | 'style-variation' | 'sketch-to-image' | 'hd-text' | 'image-extend' | 'color-placeholder'

type ImageActionMenuBase = {
  action: ImageNodeAction
  label: string
  title: string
  icon: ImageActionMenuIcon
}

type ImageActionMenuActionItem = ImageActionMenuBase & {
  placeholder?: false
  strengthAction?: ImageStrengthAction
}

type ImageActionMenuPlaceholderItem = Omit<ImageActionMenuBase, 'action'> & {
  action: null
  placeholder: true
}

export type ImageActionMenuItem = ImageActionMenuActionItem | ImageActionMenuPlaceholderItem

export const IMAGE_ACTION_MENU_ITEMS: readonly ImageActionMenuItem[] = [
  {
    action: IMAGE_NODE_ACTION.removeBg,
    label: '抠图',
    title: '抠图',
    icon: 'remove-bg',
  },
  {
    action: IMAGE_NODE_ACTION.styleVariation,
    label: '风格延展',
    title: '风格延展',
    icon: 'style-variation',
    strengthAction: IMAGE_NODE_ACTION.styleVariation,
  },
  {
    action: IMAGE_NODE_ACTION.sketchToImage,
    label: '草图成图',
    title: '草图成图',
    icon: 'sketch-to-image',
  },
  {
    action: IMAGE_NODE_ACTION.hdUpscale,
    label: 'HD',
    title: 'HD',
    icon: 'hd-text',
    strengthAction: IMAGE_NODE_ACTION.hdUpscale,
  },
  {
    action: IMAGE_NODE_ACTION.imageExtend,
    label: '扩图',
    title: '扩图',
    icon: 'image-extend',
  },
  {
    action: null,
    label: '调色',
    title: '调色（占位）',
    icon: 'color-placeholder',
    placeholder: true,
  },
] as const
