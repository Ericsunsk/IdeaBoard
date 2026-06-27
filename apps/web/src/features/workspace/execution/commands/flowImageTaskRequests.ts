import type { SubmitTaskData } from '../../../../shared/api/task'
import {
  buildExecutionTaskRequest,
  EXECUTION_FEATURE,
  EXECUTION_FUSION_META,
  EXECUTION_META,
  EXECUTION_PARTIAL_EDIT_META,
} from '../contracts/executionContract'
import { buildExecutionSizeParams, extendExecutionImageSize } from '../contracts/executionSizeParams'
import { buildPromptAspectRatioTaskParams } from '../contracts/promptAspectRatioTaskParams'
import { getCanvasNodeImageInputLimit, type PromptAspectRatio } from '../../../canvas-board/domain'
import type { ImageDirectActionSource } from '../media/flowImageDirectActionSource'
import {
  DEFAULT_IMAGE_ACTION_STRENGTH,
  IMAGE_ACTION_EXECUTION_REGISTRY,
  IMAGE_CANVAS_EXTEND_ACTION,
  IMAGE_HD_UPSCALE_ACTION,
  isImageDirectTaskAction,
  resolveHdClarityStrength,
  requireImageActionExecution,
  type ImageDirectTaskAction,
  type ImageExecutionAction,
  type ImageTaskAction,
} from '../contracts/imageActionExecution'

type ImageSize = {
  width: number
  height: number
}

export type CanvasExtendPercent = {
  top: number
  right: number
  bottom: number
  left: number
}

export type ImageInpaintTaskRequestInput = {
  prompt: string
  croppedImageUrl: string
  croppedMaskUrl: string
  referenceImageUrls?: string[]
  cropSize: ImageSize | null
}

export type FusionTaskRequestInput = {
  prompt: string
  imageUrls: string[]
  inputWeights: number[]
  sizeSource: ImageSize | null
  targetSize: ImageSize | null
  aspectRatio: PromptAspectRatio
}

function buildDirectActionReadinessRequest(action: ImageDirectTaskAction): SubmitTaskData {
  const actionMeta = IMAGE_ACTION_EXECUTION_REGISTRY[action]
  const execution = requireImageActionExecution(action)
  const strengthConfig = actionMeta.strength
  const params =
    strengthConfig?.paramPath === 'variation.strength'
      ? { variation: { strength: strengthConfig.defaultValue } }
      : undefined
  return buildExecutionTaskRequest({
    prompt: 'submit readiness',
    feature: execution.feature,
    params,
  })
}

export function buildPromptImageReadinessRequest(): SubmitTaskData {
  return buildExecutionTaskRequest({
    prompt: 'submit readiness',
    feature: EXECUTION_PARTIAL_EDIT_META.feature,
  })
}

export function buildCanvasExtendReadinessRequest(): SubmitTaskData {
  const execution = requireImageActionExecution(IMAGE_CANVAS_EXTEND_ACTION)
  return buildExecutionTaskRequest({
    prompt: 'submit readiness',
    feature: execution.feature,
    params: {
      extend: {
        top_percent: 0,
        right_percent: 1,
        bottom_percent: 0,
        left_percent: 0,
      },
    },
  })
}

function buildHdUpscaleReadinessRequest(): SubmitTaskData {
  const actionMeta = IMAGE_ACTION_EXECUTION_REGISTRY[IMAGE_HD_UPSCALE_ACTION]
  const execution = requireImageActionExecution(IMAGE_HD_UPSCALE_ACTION)
  const clarityStrength = actionMeta.strength?.defaultValue ?? DEFAULT_IMAGE_ACTION_STRENGTH
  return buildExecutionTaskRequest({
    prompt: 'submit readiness',
    feature: execution.feature,
    params: {
      hd: {
        clarity_strength: clarityStrength,
      },
    },
  })
}

export function buildImageActionReadinessRequest(action: ImageExecutionAction): SubmitTaskData {
  if (action === IMAGE_CANVAS_EXTEND_ACTION) return buildCanvasExtendReadinessRequest()
  if (action === IMAGE_HD_UPSCALE_ACTION) return buildHdUpscaleReadinessRequest()
  if (isImageDirectTaskAction(action)) return buildDirectActionReadinessRequest(action)
  throw new Error(`Unsupported image execution action ${action}`)
}

export function buildTextPromptTaskRequest(input: {
  prompt: string
  aspectRatio: PromptAspectRatio
}): SubmitTaskData {
  return buildExecutionTaskRequest({
    prompt: input.prompt,
    feature: EXECUTION_META[EXECUTION_FEATURE.patternGen].feature,
    params: buildPromptAspectRatioTaskParams(input.aspectRatio),
  })
}

export function buildDirectActionTaskRequest(input: {
  source: ImageDirectActionSource
  action: ImageDirectTaskAction
  imageSize: ImageSize | null
  strength?: number
}): SubmitTaskData {
  const actionMeta = IMAGE_ACTION_EXECUTION_REGISTRY[input.action]
  const execution = requireImageActionExecution(input.action)
  const sizeParams = buildExecutionSizeParams(input.imageSize)
  const strengthConfig = actionMeta.strength
  if (strengthConfig?.paramPath === 'variation.strength') {
    return buildExecutionTaskRequest({
      prompt: '',
      feature: execution.feature,
      imageUrls: [input.source.url],
      params: {
        ...sizeParams,
        variation: {
          strength: Number.isFinite(input.strength)
            ? Math.max(0, Math.min(100, Math.round(input.strength as number)))
            : strengthConfig.defaultValue,
        },
      },
    })
  }

  return buildExecutionTaskRequest({
    prompt: '',
    feature: execution.feature,
    imageUrls: [input.source.url],
    params: sizeParams,
  })
}

export function buildImageTaskRequest(input: {
  source: ImageDirectActionSource
  action: ImageTaskAction
  imageSize: ImageSize | null
  strength?: number
}): SubmitTaskData {
  if (input.action === IMAGE_HD_UPSCALE_ACTION) {
    return buildHdUpscaleTaskRequest(input)
  }
  if (isImageDirectTaskAction(input.action)) {
    return buildDirectActionTaskRequest({
      source: input.source,
      action: input.action,
      imageSize: input.imageSize,
      strength: input.strength,
    })
  }
  throw new Error(`Unsupported image task action ${input.action}`)
}

export function buildPromptImageTaskRequest(input: {
  source: ImageDirectActionSource
  prompt: string
  imageSize: ImageSize | null
  aspectRatio: PromptAspectRatio
}): SubmitTaskData {
  return buildExecutionTaskRequest({
    prompt: `${EXECUTION_PARTIAL_EDIT_META.promptPrefix} ${input.prompt}`,
    feature: EXECUTION_PARTIAL_EDIT_META.feature,
    imageUrls: [input.source.url],
    params: {
      original_image_url: input.source.url,
      ...buildExecutionSizeParams(input.imageSize),
      ...buildPromptAspectRatioTaskParams(input.aspectRatio),
    },
  })
}

export function buildImageInpaintTaskRequest(
  input: ImageInpaintTaskRequestInput,
): SubmitTaskData {
  const referenceImageLimit = getCanvasNodeImageInputLimit('prompt')
  const referenceImageUrls = (input.referenceImageUrls ?? [])
    .map((url) => url.trim())
    .filter(Boolean)
    .slice(0, referenceImageLimit)
  return buildExecutionTaskRequest({
    prompt: `${EXECUTION_PARTIAL_EDIT_META.promptPrefix} ${input.prompt}`,
    feature: EXECUTION_PARTIAL_EDIT_META.feature,
    imageUrls: [input.croppedImageUrl],
    params: {
      original_image_url: input.croppedImageUrl,
      mask_image_url: input.croppedMaskUrl,
      reference_image_urls: referenceImageUrls,
      use_mask: true,
      ...buildExecutionSizeParams(input.cropSize),
    },
  })
}

export function buildCanvasExtendTaskRequest(input: {
  source: ImageDirectActionSource
  imageSize: ImageSize | null
  extendPercent: CanvasExtendPercent
  prompt?: string
}): SubmitTaskData {
  const prompt = `${input.prompt ?? ''}`.trim()
  const execution = requireImageActionExecution(IMAGE_CANVAS_EXTEND_ACTION)
  const executionPrompt =
    prompt && execution.promptPrefix ? `${execution.promptPrefix} ${prompt}` : prompt
  return buildExecutionTaskRequest({
    prompt: executionPrompt,
    feature: execution.feature,
    imageUrls: [input.source.url],
    params: {
      ...buildExecutionSizeParams(
        input.imageSize,
        extendExecutionImageSize(input.imageSize, input.extendPercent),
      ),
      extend: {
        top_percent: input.extendPercent.top,
        right_percent: input.extendPercent.right,
        bottom_percent: input.extendPercent.bottom,
        left_percent: input.extendPercent.left,
      },
    },
  })
}

export function buildHdUpscaleTaskRequest(input: {
  source: ImageDirectActionSource
  imageSize: ImageSize | null
  strength?: number
}): SubmitTaskData {
  const execution = requireImageActionExecution(IMAGE_HD_UPSCALE_ACTION)
  if (!execution.prompt) {
    throw new Error('HD action execution prompt is not configured')
  }
  return buildExecutionTaskRequest({
    prompt: execution.prompt,
    feature: execution.feature,
    imageUrls: [input.source.url],
    params: {
      ...buildExecutionSizeParams(input.imageSize, input.imageSize),
      hd: {
        clarity_strength: resolveHdClarityStrength(input.strength),
      },
    },
  })
}

export function buildFusionTaskRequest(input: FusionTaskRequestInput): SubmitTaskData {
  return buildExecutionTaskRequest({
    prompt: input.prompt,
    feature: EXECUTION_FUSION_META.feature,
    imageUrls: input.imageUrls,
    params: {
      ...buildExecutionSizeParams(input.sizeSource, input.targetSize),
      ...buildPromptAspectRatioTaskParams(input.aspectRatio),
      fusion: {
        mode: 'weighted_reference',
        input_weights: input.inputWeights,
      },
    },
  })
}
