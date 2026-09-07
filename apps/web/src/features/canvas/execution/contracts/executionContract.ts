import type { SubmitTaskData, SubmitTaskParams } from '../../../../shared/api/task'
import { canonicalizeRuntimeUploadRef } from '../../../../shared/config/runtimeUrls'
import { WORKFLOW_FEATURE } from '../../../../shared/domain/workflowFeatures'

export const EXECUTION_FEATURE = {
  generation: WORKFLOW_FEATURE.generation,
  inpaint: WORKFLOW_FEATURE.inpaint,
  cutout: WORKFLOW_FEATURE.cutout,
  fusion: WORKFLOW_FEATURE.fusion,
  render: WORKFLOW_FEATURE.render,
  variations: WORKFLOW_FEATURE.variations,
  upscale: WORKFLOW_FEATURE.upscale,
  outpaint: WORKFLOW_FEATURE.outpaint,
} as const

export type ExecutionFeature = (typeof EXECUTION_FEATURE)[keyof typeof EXECUTION_FEATURE]

type ExecutionMetaEntry = {
  feature: ExecutionFeature
  promptPrefix?: string
  prompt?: string
}

export const EXECUTION_META = {
  [EXECUTION_FEATURE.generation]: {
    feature: EXECUTION_FEATURE.generation,
  },
  [EXECUTION_FEATURE.inpaint]: {
    feature: EXECUTION_FEATURE.inpaint,
  },
  [EXECUTION_FEATURE.fusion]: {
    feature: EXECUTION_FEATURE.fusion,
    promptPrefix: '[fusion]',
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
  [EXECUTION_FEATURE.upscale]: {
    feature: EXECUTION_FEATURE.upscale,
  },
  [EXECUTION_FEATURE.outpaint]: {
    feature: EXECUTION_FEATURE.outpaint,
    promptPrefix: '[outpaint]',
  },
} as const satisfies Record<ExecutionFeature, ExecutionMetaEntry>

export const EXECUTION_PARTIAL_EDIT_META = EXECUTION_META[EXECUTION_FEATURE.inpaint]

export const EXECUTION_FUSION_META = EXECUTION_META[EXECUTION_FEATURE.fusion]

interface BuildExecutionTaskRequestOptions {
  prompt: string
  // Optional task inputs: explicit undefined is equivalent to absent (every
  // consumer guards with truthiness/length checks before setting DTO keys).
  feature?: ExecutionFeature | undefined
  imageUrls?: string[] | undefined
  imageAssetIds?: number[] | undefined
  referenceImageAssetIds?: number[] | undefined
  params?: SubmitTaskParams | undefined
}

const CANVAS_INTERACTIVE_TASK_PRIORITY = 1
const MEDIA_STRING_PARAM_KEYS = new Set(['mask_image_url'])

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
