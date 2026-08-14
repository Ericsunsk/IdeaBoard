import { WORKFLOW_FEATURE } from './workflowFeatures'

export const IMAGE_NODE_ACTION = {
  imageEdit: WORKFLOW_FEATURE.imageEdit,
  removeBg: WORKFLOW_FEATURE.removeBg,
  styleVariation: WORKFLOW_FEATURE.styleVariation,
  sketchToImage: WORKFLOW_FEATURE.sketchToImage,
  hdUpscale: WORKFLOW_FEATURE.hdUpscale,
  imageExtend: WORKFLOW_FEATURE.imageExtend,
} as const
