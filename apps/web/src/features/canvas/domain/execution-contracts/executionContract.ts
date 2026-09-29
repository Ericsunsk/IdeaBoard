import type { CanvasTaskRequestData, CanvasTaskParams } from './canvasTaskRequest'
import { canonicalizeRuntimeUploadRef } from '../../../../shared/config/runtimeUrls'
import { SERVICE } from '../../../../shared/domain/imageServices'

export const EXECUTION_SERVICE = {
  generation: SERVICE.generation,
  inpaint: SERVICE.inpaint,
  cutout: SERVICE.cutout,
  fusion: SERVICE.fusion,
  render: SERVICE.render,
  variations: SERVICE.variations,
  upscale: SERVICE.upscale,
  outpaint: SERVICE.outpaint,
} as const

export type ExecutionService = (typeof EXECUTION_SERVICE)[keyof typeof EXECUTION_SERVICE]

type ExecutionMetaEntry = {
  service: ExecutionService
  promptPrefix?: string
  prompt?: string
}

export const EXECUTION_META = {
  [EXECUTION_SERVICE.generation]: {
    service: EXECUTION_SERVICE.generation,
  },
  [EXECUTION_SERVICE.inpaint]: {
    service: EXECUTION_SERVICE.inpaint,
  },
  [EXECUTION_SERVICE.fusion]: {
    service: EXECUTION_SERVICE.fusion,
    promptPrefix: '[fusion]',
  },
  [EXECUTION_SERVICE.render]: {
    service: EXECUTION_SERVICE.render,
  },
  [EXECUTION_SERVICE.variations]: {
    service: EXECUTION_SERVICE.variations,
  },
  [EXECUTION_SERVICE.cutout]: {
    service: EXECUTION_SERVICE.cutout,
  },
  [EXECUTION_SERVICE.upscale]: {
    service: EXECUTION_SERVICE.upscale,
  },
  [EXECUTION_SERVICE.outpaint]: {
    service: EXECUTION_SERVICE.outpaint,
    promptPrefix: '[outpaint]',
  },
} as const satisfies Record<ExecutionService, ExecutionMetaEntry>

export const EXECUTION_PARTIAL_EDIT_META = EXECUTION_META[EXECUTION_SERVICE.inpaint]

export const EXECUTION_FUSION_META = EXECUTION_META[EXECUTION_SERVICE.fusion]

interface BuildExecutionTaskRequestOptions {
  prompt: string
  // Optional task inputs: explicit undefined is equivalent to absent (every
  // consumer guards with truthiness/length checks before setting DTO keys).
  service?: ExecutionService | undefined
  imageUrls?: string[] | undefined
  imageAssetIds?: number[] | undefined
  referenceImageAssetIds?: number[] | undefined
  /** Opaque `asset_...` ids; the only media refs the v1 transport sends. */
  mediaAssetIds?: string[] | undefined
  params?: CanvasTaskParams | undefined
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
): CanvasTaskRequestData {
  const request: CanvasTaskRequestData = {
    prompt: options.prompt,
    priority: CANVAS_INTERACTIVE_TASK_PRIORITY,
  }

  if (options.service) {
    request.service = options.service
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
  ) as CanvasTaskParams
  if (Object.keys(nextParams).length > 0) {
    request.params = nextParams
  }
  if (options.referenceImageAssetIds && options.referenceImageAssetIds.length > 0) {
    request.reference_image_asset_ids = options.referenceImageAssetIds
  }
  const opaqueIds = (options.mediaAssetIds ?? []).map((id) => `${id ?? ''}`.trim()).filter(Boolean)
  if (opaqueIds.length > 0) {
    request.media_asset_ids = [...new Set(opaqueIds)]
  }

  return request
}
