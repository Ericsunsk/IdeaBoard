import type { SubmitTaskData, SubmitTaskParams } from '../../../../shared/api/task'
import { canonicalizeRuntimeUploadRef } from '../../../../shared/config/runtimeUrls'
import { WORKFLOW_FEATURE } from '../../../../shared/domain/workflowFeatures'

export const EXECUTION_FEATURE = {
  partialEdit: WORKFLOW_FEATURE.partialEdit,
  removeBg: WORKFLOW_FEATURE.removeBg,
  styleFusion: WORKFLOW_FEATURE.styleFusion,
  patternGen: WORKFLOW_FEATURE.patternGen,
  sketchToImage: WORKFLOW_FEATURE.sketchToImage,
  styleVariation: WORKFLOW_FEATURE.styleVariation,
  hdUpscale: WORKFLOW_FEATURE.hdUpscale,
  imageExtend: WORKFLOW_FEATURE.imageExtend,
} as const

export type ExecutionFeature = (typeof EXECUTION_FEATURE)[keyof typeof EXECUTION_FEATURE]

type ExecutionMetaEntry = {
  feature: ExecutionFeature
  promptPrefix?: string
  prompt?: string
}

export const EXECUTION_META = {
  [EXECUTION_FEATURE.partialEdit]: {
    feature: EXECUTION_FEATURE.partialEdit,
    promptPrefix: '[partial-edit]',
  },
  [EXECUTION_FEATURE.styleFusion]: {
    feature: EXECUTION_FEATURE.styleFusion,
    promptPrefix: '[style-fusion]',
  },
  [EXECUTION_FEATURE.patternGen]: {
    feature: EXECUTION_FEATURE.patternGen,
  },
  [EXECUTION_FEATURE.sketchToImage]: {
    feature: EXECUTION_FEATURE.sketchToImage,
  },
  [EXECUTION_FEATURE.styleVariation]: {
    feature: EXECUTION_FEATURE.styleVariation,
  },
  [EXECUTION_FEATURE.removeBg]: {
    feature: EXECUTION_FEATURE.removeBg,
  },
  [EXECUTION_FEATURE.hdUpscale]: {
    feature: EXECUTION_FEATURE.hdUpscale,
    prompt: '[hd-upscale] HD 与细节增强',
  },
  [EXECUTION_FEATURE.imageExtend]: {
    feature: EXECUTION_FEATURE.imageExtend,
    promptPrefix: '[image-extend]',
  },
} as const satisfies Record<ExecutionFeature, ExecutionMetaEntry>

export const EXECUTION_PARTIAL_EDIT_META = EXECUTION_META[EXECUTION_FEATURE.partialEdit]

export const EXECUTION_FUSION_META = EXECUTION_META[EXECUTION_FEATURE.styleFusion]

interface BuildExecutionTaskRequestOptions {
  prompt: string
  feature?: ExecutionFeature
  imageUrls?: string[]
  params?: SubmitTaskParams
}

const CANVAS_INTERACTIVE_TASK_PRIORITY = 1
const MEDIA_STRING_PARAM_KEYS = new Set(['original_image_url', 'mask_image_url'])
const MEDIA_LIST_PARAM_KEYS = new Set(['reference_image_urls'])

function canonicalizeTaskMediaParam(key: string, value: unknown): unknown {
  if (MEDIA_STRING_PARAM_KEYS.has(key) && typeof value === 'string') {
    return canonicalizeRuntimeUploadRef(value)
  }
  if (MEDIA_LIST_PARAM_KEYS.has(key) && Array.isArray(value)) {
    return value.map((item) =>
      typeof item === 'string' ? canonicalizeRuntimeUploadRef(item) : item,
    )
  }
  return value
}

export function buildExecutionTaskRequest(
  options: BuildExecutionTaskRequestOptions,
): SubmitTaskData {
  const request: SubmitTaskData = {
    prompt: options.prompt,
    priority: CANVAS_INTERACTIVE_TASK_PRIORITY,
  }

  if (options.feature) {
    request.feature = options.feature
  }
  if (options.imageUrls && options.imageUrls.length > 0) {
    request.image_urls = options.imageUrls.map(canonicalizeRuntimeUploadRef)
  }

  const nextParams = Object.fromEntries(
    Object.entries(options.params ?? {}).map(([key, value]) => [
      key,
      canonicalizeTaskMediaParam(key, value),
    ]),
  ) as SubmitTaskParams
  if (Object.keys(nextParams).length > 0) {
    request.params = nextParams
  }

  return request
}
