export const WORKFLOW_FEATURE = {
  partialEdit: 'partial-edit',
  removeBg: 'remove-bg',
  styleFusion: 'style-fusion',
  patternGen: 'pattern-gen',
  sketchToImage: 'sketch-to-image',
  styleVariation: 'style-variation',
  multiAngle: 'multi-angle',
  virtual3d: 'virtual-3d',
  hdUpscale: 'hd-upscale',
  imageExtend: 'image-extend',
} as const

export type WorkflowFeature = (typeof WORKFLOW_FEATURE)[keyof typeof WORKFLOW_FEATURE]
