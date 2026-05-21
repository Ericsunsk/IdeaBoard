import type { SubmitTaskData } from '../../../shared/api/task'

export const EXECUTION_FEATURE = {
  partialEdit: 'partial-edit',
  bgSwap: 'bg-swap',
  removeBg: 'remove-bg',
  fabricOnBody: 'fabric-on-body',
  styleFusion: 'style-fusion',
  patternGen: 'pattern-gen',
  sketchToImage: 'sketch-to-image',
  styleVariation: 'style-variation',
  multiAngle: 'multi-angle',
  virtual3d: 'virtual-3d',
  faceSwap: 'face-swap',
  hdUpscale: 'hd-upscale',
  imageExtend: 'image-extend',
} as const

export type ExecutionFeature = (typeof EXECUTION_FEATURE)[keyof typeof EXECUTION_FEATURE]

export const EXECUTION_EDIT_MODE_META = {
  'partial-edit': {
    feature: EXECUTION_FEATURE.partialEdit,
    promptPrefix: '[partial-edit]',
  },
  'bg-swap': {
    feature: EXECUTION_FEATURE.bgSwap,
    promptPrefix: '[bg-swap]',
  },
} as const

export const EXECUTION_WEAR_META = {
  feature: EXECUTION_FEATURE.fabricOnBody,
  promptPrefix: '[fabric-on-body]',
  successLabel: 'Wear',
} as const

export const EXECUTION_FUSION_META = {
  feature: EXECUTION_FEATURE.styleFusion,
  promptPrefix: '[style-fusion]',
  successLabel: 'Fusion',
} as const

export const EXECUTION_DIRECT_ACTION_META = {
  patternGen: {
    feature: EXECUTION_FEATURE.patternGen,
  },
  sketchToImage: {
    feature: EXECUTION_FEATURE.sketchToImage,
  },
  styleVariation: {
    feature: EXECUTION_FEATURE.styleVariation,
  },
  removeBg: {
    feature: EXECUTION_FEATURE.removeBg,
  },
} as const

export const EXECUTION_MODEL_META = {
  feature: EXECUTION_FEATURE.multiAngle,
  promptPrefix: '[multi-angle]',
} as const

export const EXECUTION_VIEWPOINT_META = {
  feature: EXECUTION_FEATURE.virtual3d,
  promptPrefix: '[virtual-3d]',
  successLabel: 'Display',
} as const

export const EXECUTION_FACE_SWAP_META = {
  feature: EXECUTION_FEATURE.faceSwap,
  prompt: '[face-swap] 换脸',
} as const

export const EXECUTION_IMAGE_TOOL_META = {
  hdUpscale: {
    feature: EXECUTION_FEATURE.hdUpscale,
    prompt: '[hd-upscale] 局部高清放大，仅增强蒙版区域',
  },
  imageExtend: {
    feature: EXECUTION_FEATURE.imageExtend,
    promptPrefix: '[image-extend]',
  },
} as const

interface BuildExecutionTaskRequestOptions {
  prompt: string
  feature?: ExecutionFeature
  imageUrls?: string[]
  params?: Record<string, unknown>
}

export function buildExecutionTaskRequest(
  options: BuildExecutionTaskRequestOptions,
): SubmitTaskData {
  const request: SubmitTaskData = {
    prompt: options.prompt,
  }

  if (options.feature) {
    request.feature = options.feature
  }
  if (options.imageUrls && options.imageUrls.length > 0) {
    request.image_urls = options.imageUrls
  }

  const nextParams = { ...(options.params ?? {}) }
  delete nextParams.feature
  delete nextParams.image_urls
  if (Object.keys(nextParams).length > 0) {
    request.params = nextParams
  }

  return request
}
