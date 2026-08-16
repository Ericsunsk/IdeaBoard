export const WORKFLOW_FEATURE = {
  promptGeneration: 'prompt-generation',
  partialEdit: 'partial-edit',
  removeBg: 'remove-bg',
  styleFusion: 'style-fusion',
  sketchToImage: 'sketch-to-image',
  styleVariation: 'style-variation',
  hdUpscale: 'hd-upscale',
  outpaint: 'outpaint',
} as const

export type WorkflowFeature = (typeof WORKFLOW_FEATURE)[keyof typeof WORKFLOW_FEATURE]

export const WORKFLOW_FEATURE_SLUGS = Object.values(WORKFLOW_FEATURE) as readonly WorkflowFeature[]

export const CANONICAL_WORKFLOW_FEATURE = {
  promptGeneration: 'prompt-generation',
  partialEdit: 'partial-edit',
  removeBg: 'remove-bg',
  styleFusion: 'style-fusion',
  sketchToImage: 'sketch-to-image',
  styleVariation: 'style-variation',
  hdUpscale: 'hd-upscale',
  outpaint: 'outpaint',
} as const

export type CanonicalWorkflowFeature =
  (typeof CANONICAL_WORKFLOW_FEATURE)[keyof typeof CANONICAL_WORKFLOW_FEATURE]

export const CANONICAL_WORKFLOW_FEATURE_SLUGS = Object.values(
  CANONICAL_WORKFLOW_FEATURE,
) as readonly CanonicalWorkflowFeature[]
