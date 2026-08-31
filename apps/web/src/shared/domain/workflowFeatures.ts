export const WORKFLOW_FEATURE = {
  generation: 'generation',
  inpaint: 'inpaint',
  cutout: 'cutout',
  fusion: 'fusion',
  render: 'render',
  variations: 'variations',
  upscale: 'upscale',
  outpaint: 'outpaint',
} as const

export type WorkflowFeature = (typeof WORKFLOW_FEATURE)[keyof typeof WORKFLOW_FEATURE]

export const WORKFLOW_FEATURE_SLUGS = Object.values(WORKFLOW_FEATURE) as readonly WorkflowFeature[]
