import type { SubmitTaskData, SubmitTaskParams } from '../../../../shared/api/task'
import { canonicalizeRuntimeUploadRef } from '../../../../shared/config/runtimeUrls'
import { WORKFLOW_FEATURE } from '../../../../shared/domain/workflowFeatures'

export const EXECUTION_FEATURE = {
  promptGeneration: WORKFLOW_FEATURE.promptGeneration,
  inpaint: WORKFLOW_FEATURE.inpaint,
  cutout: WORKFLOW_FEATURE.cutout,
  styleFusion: WORKFLOW_FEATURE.styleFusion,
  render: WORKFLOW_FEATURE.render,
  variations: WORKFLOW_FEATURE.variations,
  hdUpscale: WORKFLOW_FEATURE.hdUpscale,
  outpaint: WORKFLOW_FEATURE.outpaint,
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
  [EXECUTION_FEATURE.inpaint]: {
    feature: EXECUTION_FEATURE.inpaint,
  },
  [EXECUTION_FEATURE.styleFusion]: {
    feature: EXECUTION_FEATURE.styleFusion,
    promptPrefix: '[style-fusion]',
  },
  [EXECUTION_FEATURE.render]: {
    feature: EXECUTION_FEATURE.render,
  },
  [EXECUTION_FEATURE.variations]: {
    feature: EXECUTION_FEATURE.variations,
  },
  [EXECUTION_FEATURE.cutout]: {
    feature: EXECUTION_FEATURE.cutout,
  },
  [EXECUTION_FEATURE.hdUpscale]: {
    feature: EXECUTION_FEATURE.hdUpscale,
  },
  [EXECUTION_FEATURE.outpaint]: {
    feature: EXECUTION_FEATURE.outpaint,
    promptPrefix: '[outpaint]',
  },
} as const satisfies Record<ExecutionFeature, ExecutionMetaEntry>

export const EXECUTION_PARTIAL_EDIT_META = EXECUTION_META[EXECUTION_FEATURE.inpaint]

export const EXECUTION_FUSION_META = EXECUTION_META[EXECUTION_FEATURE.styleFusion]

interface BuildExecutionTaskRequestOptions {
  prompt: string
  feature?: ExecutionFeature
  imageUrls?: string[]
  imageAssetIds?: number[]
  referenceImageAssetIds?: number[]
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
  if (options.imageAssetIds && options.imageAssetIds.length > 0) {
    request.image_asset_ids = options.imageAssetIds
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
  if (options.referenceImageAssetIds && options.referenceImageAssetIds.length > 0) {
    request.reference_image_asset_ids = options.referenceImageAssetIds
  }

  return request
}
