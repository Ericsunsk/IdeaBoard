export const WORKFLOW_FEATURE = {
  partialEdit: 'partial-edit',
  removeBg: 'remove-bg',
  styleFusion: 'style-fusion',
  patternGen: 'pattern-gen',
  sketchToImage: 'sketch-to-image',
  styleVariation: 'style-variation',
  hdUpscale: 'hd-upscale',
  imageExtend: 'image-extend',
} as const

export type WorkflowFeature = (typeof WORKFLOW_FEATURE)[keyof typeof WORKFLOW_FEATURE]
