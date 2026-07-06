import type { SubmitTaskData } from '../../../../shared/api/task'
import type {
  CanvasExtendExecutionPlan,
  ImageActionExecutionPlan,
  ImageInpaintExecutionPlan,
  ImagePromptExecutionPlan,
  ImageExecutionInput,
  PreparedFusionExecutionPlan,
  TextPromptExecutionPlan,
} from '../planning/ImageExecutionPlan'
import type { CanvasExtendPercent } from '../../utils/canvasExtendGeometry'
import {
  buildExecutionTaskRequest,
  EXECUTION_FEATURE,
  EXECUTION_FUSION_META,
  EXECUTION_META,
  EXECUTION_PARTIAL_EDIT_META,
} from '../contracts/executionContract'
import {
  buildExecutionSizeParams,
  extendExecutionImageSize,
  type ExecutionImageSize,
} from '../contracts/executionSizeParams'
import { buildPromptAspectRatioTaskParams } from '../contracts/promptAspectRatioTaskParams'
import { getCanvasNodeImageInputLimit, type PromptAspectRatio } from '../../../canvas-board/domain'
import {
  DEFAULT_IMAGE_ACTION_STRENGTH,
  IMAGE_CANVAS_EXTEND_ACTION,
  IMAGE_HD_UPSCALE_ACTION,
  isImageDirectTaskAction,
  resolveHdClarityStrength,
  requireImageActionExecution,
  type ImageDirectTaskAction,
  type ImageExecutionAction,
} from '../contracts/imageActionExecution'

function requireImageExecutionInputUrl(source: ImageExecutionInput): string {
  const url = `${source.url ?? ''}`.trim()
  if (!url) throw new Error('图片缺少 URL')
  return url
}

export type ImageInpaintTaskRequestInput = {
  prompt: string
  croppedImageUrl: string
  croppedMaskUrl: string
  referenceImageUrls?: string[]
  cropSize: ExecutionImageSize | null
}

export type FusionTaskRequestInput = {
  prompt: string
  imageUrls: string[]
  inputWeights: number[]
  sizeSource: ExecutionImageSize | null
  targetSize: ExecutionImageSize | null
  aspectRatio: PromptAspectRatio
}

function buildDirectActionReadinessRequest(action: ImageDirectTaskAction): SubmitTaskData {
  const execution = requireImageActionExecution(action)
  const strengthConfig = execution.strength
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
  const execution = requireImageActionExecution(IMAGE_HD_UPSCALE_ACTION)
  const clarityStrength = execution.strength?.defaultValue ?? DEFAULT_IMAGE_ACTION_STRENGTH
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
  source: ImageExecutionInput
  action: ImageDirectTaskAction
  imageSize: ExecutionImageSize | null
  strength?: number
}): SubmitTaskData {
  const sourceUrl = requireImageExecutionInputUrl(input.source)
  const execution = requireImageActionExecution(input.action)
  const sizeParams = buildExecutionSizeParams(input.imageSize)
  const strengthConfig = execution.strength
  if (strengthConfig?.paramPath === 'variation.strength') {
    return buildExecutionTaskRequest({
      prompt: '',
      feature: execution.feature,
      imageUrls: [sourceUrl],
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
    imageUrls: [sourceUrl],
    params: sizeParams,
  })
}

export function buildPromptImageTaskRequest(input: {
  source: ImageExecutionInput
  prompt: string
  imageSize: ExecutionImageSize | null
  aspectRatio: PromptAspectRatio
}): SubmitTaskData {
  const sourceUrl = requireImageExecutionInputUrl(input.source)
  return buildExecutionTaskRequest({
    prompt: `${EXECUTION_PARTIAL_EDIT_META.promptPrefix} ${input.prompt}`,
    feature: EXECUTION_PARTIAL_EDIT_META.feature,
    imageUrls: [sourceUrl],
    params: {
      original_image_url: sourceUrl,
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
  source: ImageExecutionInput
  imageSize: ExecutionImageSize | null
  extendPercent: CanvasExtendPercent
  prompt?: string
}): SubmitTaskData {
  const sourceUrl = requireImageExecutionInputUrl(input.source)
  const prompt = `${input.prompt ?? ''}`.trim()
  const execution = requireImageActionExecution(IMAGE_CANVAS_EXTEND_ACTION)
  const executionPrompt =
    prompt && execution.promptPrefix ? `${execution.promptPrefix} ${prompt}` : prompt
  return buildExecutionTaskRequest({
    prompt: executionPrompt,
    feature: execution.feature,
    imageUrls: [sourceUrl],
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
  source: ImageExecutionInput
  imageSize: ExecutionImageSize | null
  strength?: number
}): SubmitTaskData {
  const sourceUrl = requireImageExecutionInputUrl(input.source)
  const execution = requireImageActionExecution(IMAGE_HD_UPSCALE_ACTION)
  if (!execution.prompt) {
    throw new Error('HD action execution prompt is not configured')
  }
  return buildExecutionTaskRequest({
    prompt: execution.prompt,
    feature: execution.feature,
    imageUrls: [sourceUrl],
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

type ImageExecutionTaskRequestPlan =
  | ImageActionExecutionPlan
  | CanvasExtendExecutionPlan
  | ImagePromptExecutionPlan
  | TextPromptExecutionPlan
  | PreparedFusionExecutionPlan

export function buildImageExecutionTaskRequest(
  plan: ImageExecutionTaskRequestPlan,
  imageSize: ExecutionImageSize | null = null,
): SubmitTaskData {
  switch (plan.kind) {
    case 'image-action':
      if (plan.action === IMAGE_HD_UPSCALE_ACTION) {
        return buildHdUpscaleTaskRequest({
          source: plan.source,
          imageSize,
          strength: plan.strength,
        })
      }
      if (isImageDirectTaskAction(plan.action)) {
        return buildDirectActionTaskRequest({
          source: plan.source,
          action: plan.action,
          imageSize,
          strength: plan.strength,
        })
      }
      throw new Error(`Unsupported image task action ${plan.action}`)
    case 'canvas-extend':
      return buildCanvasExtendTaskRequest({
        source: plan.source,
        imageSize,
        extendPercent: plan.extendPercent,
        prompt: plan.prompt,
      })
    case 'image-prompt':
      return buildPromptImageTaskRequest({
        source: plan.source,
        prompt: plan.prompt,
        imageSize,
        aspectRatio: plan.aspectRatio,
      })
    case 'text-prompt':
      return buildTextPromptTaskRequest({
        prompt: plan.prompt,
        aspectRatio: plan.aspectRatio,
      })
    case 'fusion':
      return buildFusionTaskRequest({
        prompt: plan.taskPrompt,
        imageUrls: plan.imageUrls,
        inputWeights: plan.inputWeights,
        sizeSource: plan.sizeSource,
        targetSize: plan.targetSize,
        aspectRatio: plan.aspectRatio,
      })
  }
}

export function buildPreparedImageInpaintTaskRequest(input: {
  plan: ImageInpaintExecutionPlan
  croppedImageUrl: string
  croppedMaskUrl: string
  cropSize: ExecutionImageSize | null
}): SubmitTaskData {
  return buildImageInpaintTaskRequest({
    prompt: input.plan.prompt,
    croppedImageUrl: input.croppedImageUrl,
    croppedMaskUrl: input.croppedMaskUrl,
    referenceImageUrls: input.plan.referenceImageUrls,
    cropSize: input.cropSize,
  })
}
