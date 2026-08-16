import { WORKFLOW_FEATURE } from './workflowFeatures'

export const IMAGE_NODE_ACTION = {
  removeBg: WORKFLOW_FEATURE.removeBg,
  styleVariation: WORKFLOW_FEATURE.styleVariation,
  sketchToImage: WORKFLOW_FEATURE.sketchToImage,
  hdUpscale: WORKFLOW_FEATURE.hdUpscale,
  outpaint: WORKFLOW_FEATURE.outpaint,
} as const
