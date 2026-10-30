import type { CanvasTaskRequestData, CanvasTaskParams } from './canvasTaskRequest'
import { SERVICE } from '../../../../core/contracts/imageServices'

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
  /** Opaque `asset_...` ids; the only media refs the v1 transport sends. */
  mediaAssetIds?: string[] | undefined
  params?: CanvasTaskParams | undefined
}

export function buildExecutionTaskRequest(
  options: BuildExecutionTaskRequestOptions,
): CanvasTaskRequestData {
  const request: CanvasTaskRequestData = {
    prompt: options.prompt,
  }

  if (options.service) {
    request.service = options.service
  }
  if (options.params && Object.keys(options.params).length > 0) {
    request.params = { ...options.params }
  }
  const opaqueIds = (options.mediaAssetIds ?? []).map((id) => `${id ?? ''}`.trim()).filter(Boolean)
  if (opaqueIds.length > 0) {
    request.media_asset_ids = [...new Set(opaqueIds)]
  }

  return request
}
