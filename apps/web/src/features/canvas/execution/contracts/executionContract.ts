import type { SubmitTaskData, SubmitTaskParams } from '../../../../shared/api/task'
import { canonicalizeRuntimeUploadRef } from '../../../../shared/config/runtimeUrls'
import { WORKFLOW_FEATURE } from '../../../../shared/domain/workflowFeatures'

export const EXECUTION_FEATURE = {
  promptGeneration: WORKFLOW_FEATURE.promptGeneration,
  localInpaint: WORKFLOW_FEATURE.localInpaint,
  removeBg: WORKFLOW_FEATURE.removeBg,
  styleFusion: WORKFLOW_FEATURE.styleFusion,
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
  [EXECUTION_FEATURE.promptGeneration]: {
    feature: EXECUTION_FEATURE.promptGeneration,
  },
  [EXECUTION_FEATURE.localInpaint]: {
    feature: EXECUTION_FEATURE.localInpaint,
  },
  [EXECUTION_FEATURE.styleFusion]: {
    feature: EXECUTION_FEATURE.styleFusion,
    promptPrefix: '[style-fusion]',
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
  },
  [EXECUTION_FEATURE.imageExtend]: {
    feature: EXECUTION_FEATURE.imageExtend,
    promptPrefix: '[image-extend]',
  },
} as const satisfies Record<ExecutionFeature, ExecutionMetaEntry>

export const EXECUTION_LOCAL_INPAINT_META = EXECUTION_META[EXECUTION_FEATURE.localInpaint]

export const EXECUTION_FUSION_META = EXECUTION_META[EXECUTION_FEATURE.styleFusion]

interface BuildExecutionTaskRequestOptions {
  prompt: string
  feature?: ExecutionFeature
  imageUrls?: string[]
  params?: SubmitTaskParams
}

const CANVAS_INTERACTIVE_TASK_PRIORITY = 1
const MEDIA_STRING_PARAM_KEYS = new Set(['original_image_url', 'mask_image_url'])

function canonicalizeTaskMediaParam(key: string, value: unknown): unknown {
  if (MEDIA_STRING_PARAM_KEYS.has(key) && typeof value === 'string') {
    return canonicalizeRuntimeUploadRef(value)
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
