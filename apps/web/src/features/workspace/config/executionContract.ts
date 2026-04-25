import type { SubmitTaskData } from '../../../shared/api/index'

export const EXECUTION_FEATURE = {
  partialEdit: 'partial-edit',
  bgSwap: 'bg-swap',
  removeBg: 'remove-bg',
  fabricOnBody: 'fabric-on-body',
  styleFusion: 'style-fusion',
  seriesColor: 'series-color',
  directedRecolor: 'directed-recolor',
  patternGen: 'pattern-gen',
  sketchToDesign: 'sketch-to-design',
  styleInnovate: 'style-innovate',
  multiAngle: 'multi-angle',
  virtual3d: 'virtual-3d',
  faceSwap: 'face-swap',
  hdUpscale: 'hd-upscale',
  canvasExtend: 'canvas-extend',
} as const

export type ExecutionFeature = (typeof EXECUTION_FEATURE)[keyof typeof EXECUTION_FEATURE]

export const EXECUTION_EDIT_MODE_META = {
  'partial-edit': {
    feature: EXECUTION_FEATURE.partialEdit,
    promptPrefix: '[partial-edit]',
    fallbackPrompt: '局部改款',
  },
  'bg-swap': {
    feature: EXECUTION_FEATURE.bgSwap,
    promptPrefix: '[bg-swap]',
    fallbackPrompt: '换背景',
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
  fallbackPrompt: '双图融合',
  successLabel: 'Fusion',
} as const

export const EXECUTION_COLOR_SERIES_META = {
  feature: EXECUTION_FEATURE.seriesColor,
  colorType: 'reference',
  promptPrefix: '[series-color] 多色配色',
} as const

export const EXECUTION_DIRECTED_RECOLOR_META = {
  feature: EXECUTION_FEATURE.directedRecolor,
  promptPrefix: '[directed-recolor]',
  successLabel: 'Color',
} as const

export const EXECUTION_DIRECT_ACTION_META = {
  patternGen: {
    feature: EXECUTION_FEATURE.patternGen,
  },
  sketchToDesign: {
    feature: EXECUTION_FEATURE.sketchToDesign,
    promptPrefix: '[sketch-to-design]',
    fallbackPrompt: 'Sketch to design',
  },
  styleInnovate: {
    feature: EXECUTION_FEATURE.styleInnovate,
    promptPrefix: '[style-innovate]',
    fallbackPrompt: '保持原图主体身份、服装品类、版型轮廓、构图与光影关系不变，只在此基础上按强度做可控款式创新与细节变化',
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

export const EXECUTION_ENHANCE_META = {
  hdUpscale: {
    feature: EXECUTION_FEATURE.hdUpscale,
    prompt: '[hd-upscale] 局部高清放大，仅增强蒙版区域',
  },
  canvasExtend: {
    feature: EXECUTION_FEATURE.canvasExtend,
    promptPrefix: '[canvas-extend]',
    fallbackPrompt: '保持原图主体、人物、服装、构图、光影与透视不变，仅按扩边方向在新增画布区域自然补全连续背景与缺失内容',
  },
} as const

interface BuildExecutionTaskRequestOptions {
  prompt: string
  cardIds?: number[]
  feature?: ExecutionFeature
  imageUrls?: string[]
  params?: Record<string, unknown>
}

export function buildExecutionTaskRequest(options: BuildExecutionTaskRequestOptions): SubmitTaskData {
  const request: SubmitTaskData = {
    prompt: options.prompt,
  }

  if (options.cardIds && options.cardIds.length > 0) {
    request.card_ids = options.cardIds
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
