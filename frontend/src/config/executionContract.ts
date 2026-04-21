import type { SubmitTaskData } from '../api/index'

export const EXECUTION_EDIT_MODE_META = {
  'partial-edit': {
    feature: 'partial-edit',
    promptPrefix: '[partial-edit]',
    fallbackPrompt: '局部改款',
  },
  'bg-swap': {
    feature: 'bg-swap',
    promptPrefix: '[bg-swap]',
    fallbackPrompt: '换背景',
  },
} as const

export const EXECUTION_WEAR_META = {
  feature: 'fabric-on-body',
  promptPrefix: '[fabric-on-body]',
  successLabel: 'Wear',
} as const

export const EXECUTION_FUSION_META = {
  feature: 'style-fusion',
  promptPrefix: '[style-fusion]',
  fallbackPrompt: '双图融合',
  successLabel: 'Fusion',
} as const

export const EXECUTION_COLOR_SERIES_META = {
  feature: 'series-color',
  colorType: 'reference',
  promptPrefix: '[series-color] 多色配色',
} as const

export const EXECUTION_DIRECTED_RECOLOR_META = {
  feature: 'directed-recolor',
  promptPrefix: '[directed-recolor]',
  successLabel: 'Color',
} as const

export const EXECUTION_DIRECT_ACTION_META = {
  patternGen: {
    feature: 'pattern-gen',
  },
  sketchToDesign: {
    feature: 'sketch-to-design',
    promptPrefix: '[sketch-to-design]',
    fallbackPrompt: 'Sketch to design',
  },
  styleInnovate: {
    feature: 'style-innovate',
    promptPrefix: '[style-innovate]',
    fallbackPrompt: 'Style innovate',
  },
} as const

interface BuildExecutionTaskRequestOptions {
  prompt: string
  cardIds?: number[]
  feature?: string
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
