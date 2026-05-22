import type { SubmitTaskData } from '../../../shared/api/task'
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
  [EXECUTION_FEATURE.bgSwap]: {
    feature: EXECUTION_FEATURE.bgSwap,
    promptPrefix: '[bg-swap]',
  },
  [EXECUTION_FEATURE.fabricOnBody]: {
    feature: EXECUTION_FEATURE.fabricOnBody,
    promptPrefix: '[fabric-on-body]',
    successLabel: 'Wear',
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
  [EXECUTION_FEATURE.multiAngle]: {
    feature: EXECUTION_FEATURE.multiAngle,
    promptPrefix: '[multi-angle]',
  },
  [EXECUTION_FEATURE.virtual3d]: {
    feature: EXECUTION_FEATURE.virtual3d,
    promptPrefix: '[virtual-3d]',
    successLabel: 'Display',
  },
  [EXECUTION_FEATURE.faceSwap]: {
    feature: EXECUTION_FEATURE.faceSwap,
    prompt: '[face-swap] 换脸',
  },
  [EXECUTION_FEATURE.hdUpscale]: {
    feature: EXECUTION_FEATURE.hdUpscale,
    prompt: '[hd-upscale] 局部高清放大，仅增强蒙版区域',
  },
  [EXECUTION_FEATURE.imageExtend]: {
    feature: EXECUTION_FEATURE.imageExtend,
    promptPrefix: '[image-extend]',
  },
} as const satisfies Record<ExecutionFeature, ExecutionMetaEntry>

export const EXECUTION_EDIT_MODE_META = {
  [EXECUTION_FEATURE.partialEdit]: EXECUTION_META[EXECUTION_FEATURE.partialEdit],
  [EXECUTION_FEATURE.bgSwap]: EXECUTION_META[EXECUTION_FEATURE.bgSwap],
} as const

export const EXECUTION_WEAR_META = EXECUTION_META[EXECUTION_FEATURE.fabricOnBody]

export const EXECUTION_FUSION_META = EXECUTION_META[EXECUTION_FEATURE.styleFusion]

export const EXECUTION_DIRECT_ACTION_META = {
  patternGen: EXECUTION_META[EXECUTION_FEATURE.patternGen],
  sketchToImage: EXECUTION_META[EXECUTION_FEATURE.sketchToImage],
  styleVariation: EXECUTION_META[EXECUTION_FEATURE.styleVariation],
  removeBg: EXECUTION_META[EXECUTION_FEATURE.removeBg],
} as const

export const EXECUTION_MODEL_META = EXECUTION_META[EXECUTION_FEATURE.multiAngle]

export const EXECUTION_VIEWPOINT_META = EXECUTION_META[EXECUTION_FEATURE.virtual3d]

export const EXECUTION_FACE_SWAP_META = EXECUTION_META[EXECUTION_FEATURE.faceSwap]

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
