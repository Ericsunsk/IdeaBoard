import { WORKFLOW_FEATURE } from './workflowFeatures'

export const IMAGE_NODE_ACTION = {
  cutout: WORKFLOW_FEATURE.cutout,
  variations: WORKFLOW_FEATURE.variations,
  render: WORKFLOW_FEATURE.render,
  hdUpscale: WORKFLOW_FEATURE.hdUpscale,
  outpaint: WORKFLOW_FEATURE.outpaint,
} as const
