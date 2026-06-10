import type { SubmitTaskData } from '../../../shared/api/task'
import { canonicalizeRuntimeUploadRef } from '../../../shared/config/runtimeUrls'
import { WORKFLOW_FEATURE, type WorkflowFeature } from '../../../shared/domain/workflowFeatures'

export const EXECUTION_FEATURE = WORKFLOW_FEATURE

export type ExecutionFeature = WorkflowFeature

type ExecutionMetaEntry = {
  feature: ExecutionFeature
  promptPrefix?: string
  prompt?: string
  successLabel?: string
}

export const EXECUTION_META = {
  [EXECUTION_FEATURE.partialEdit]: {
    feature: EXECUTION_FEATURE.partialEdit,
    promptPrefix: '[partial-edit]',
  },
  [EXECUTION_FEATURE.styleFusion]: {
    feature: EXECUTION_FEATURE.styleFusion,
    promptPrefix: '[style-fusion]',
    successLabel: 'Fusion',
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
  [EXECUTION_FEATURE.virtual3d]: {
    feature: EXECUTION_FEATURE.virtual3d,
    promptPrefix: '[virtual-3d]',
    successLabel: 'Display',
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

export const EXECUTION_DIRECT_ACTION_META = {
  patternGen: EXECUTION_META[EXECUTION_FEATURE.patternGen],
  sketchToImage: EXECUTION_META[EXECUTION_FEATURE.sketchToImage],
  styleVariation: EXECUTION_META[EXECUTION_FEATURE.styleVariation],
  removeBg: EXECUTION_META[EXECUTION_FEATURE.removeBg],
} as const

export const EXECUTION_VIEWPOINT_META = EXECUTION_META[EXECUTION_FEATURE.virtual3d]

export const EXECUTION_IMAGE_TOOL_META = {
  hdUpscale: EXECUTION_META[EXECUTION_FEATURE.hdUpscale],
  imageExtend: EXECUTION_META[EXECUTION_FEATURE.imageExtend],
} as const

interface BuildExecutionTaskRequestOptions {
  prompt: string
  feature?: ExecutionFeature
  imageUrls?: string[]
  params?: Record<string, unknown>
}

const MEDIA_STRING_PARAM_KEYS = new Set(['image_url', 'original_image_url', 'mask_image_url'])
const MEDIA_LIST_PARAM_KEYS = new Set(['image_urls', 'mask_image_urls', 'reference_image_urls'])

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
  )
  delete nextParams.feature
  delete nextParams.image_urls
  if (Object.keys(nextParams).length > 0) {
    request.params = nextParams
  }

  return request
}
