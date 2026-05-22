import { IMAGE_NODE_ACTION as WORKFLOW_IMAGE_NODE_ACTION } from '../../../shared/domain/imageActions'

export const IMAGE_NODE_ACTION = {
  ...WORKFLOW_IMAGE_NODE_ACTION,
  imageInpaint: 'image-inpaint',
} as const

export const IMAGE_DIRECT_TASK_ACTIONS = [
  IMAGE_NODE_ACTION.removeBg,
  IMAGE_NODE_ACTION.styleVariation,
  IMAGE_NODE_ACTION.sketchToImage,
] as const

export const IMAGE_STRENGTH_ACTIONS = [
  IMAGE_NODE_ACTION.styleVariation,
  IMAGE_NODE_ACTION.hdUpscale,
] as const

export type ImageDirectTaskAction = (typeof IMAGE_DIRECT_TASK_ACTIONS)[number]
export type ImageStrengthAction = (typeof IMAGE_STRENGTH_ACTIONS)[number]
export type ImageNodeAction =
  | ImageDirectTaskAction
  | ImageStrengthAction
  | typeof IMAGE_NODE_ACTION.imageExtend
  | typeof IMAGE_NODE_ACTION.imageInpaint

export const DEFAULT_IMAGE_ACTION_STRENGTH = 50

export const DEFAULT_IMAGE_ACTION_STRENGTHS = {
  [IMAGE_NODE_ACTION.styleVariation]: DEFAULT_IMAGE_ACTION_STRENGTH,
  [IMAGE_NODE_ACTION.hdUpscale]: DEFAULT_IMAGE_ACTION_STRENGTH,
} satisfies Record<ImageStrengthAction, number>

const IMAGE_STRENGTH_ACTION_SET: ReadonlySet<string> = new Set(IMAGE_STRENGTH_ACTIONS)
const IMAGE_DIRECT_TASK_ACTION_SET: ReadonlySet<string> = new Set(IMAGE_DIRECT_TASK_ACTIONS)

export function isImageStrengthAction(action: ImageNodeAction): action is ImageStrengthAction {
  return IMAGE_STRENGTH_ACTION_SET.has(action)
}

export function isImageDirectTaskAction(action: ImageNodeAction): action is ImageDirectTaskAction {
  return IMAGE_DIRECT_TASK_ACTION_SET.has(action)
}
