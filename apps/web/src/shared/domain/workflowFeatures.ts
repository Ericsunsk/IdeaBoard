export const WORKFLOW_FEATURE = {
  promptGeneration: 'prompt-generation',
  inpaint: 'inpaint',
  cutout: 'cutout',
  styleFusion: 'style-fusion',
  render: 'render',
  variations: 'variations',
  hdUpscale: 'hd-upscale',
  outpaint: 'outpaint',
} as const

export type WorkflowFeature = (typeof WORKFLOW_FEATURE)[keyof typeof WORKFLOW_FEATURE]

export const WORKFLOW_FEATURE_SLUGS = Object.values(WORKFLOW_FEATURE) as readonly WorkflowFeature[]
