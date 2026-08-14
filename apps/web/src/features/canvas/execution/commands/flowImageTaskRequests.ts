import type { SubmitTaskData } from '../../../../shared/api/task'
import type {
  CanvasExtendExecutionPlan,
  ImageActionExecutionPlan,
  ImageInpaintExecutionPlan,
  ImagePromptExecutionPlan,
  ImageExecutionInput,
  PromptGenerationExecutionPlan,
  PreparedFusionExecutionPlan,
} from '../planning/ImageExecutionPlan'
import type { CanvasExtendPercent } from '../../utils/canvasExtendGeometry'
import {
  buildExecutionTaskRequest,
  EXECUTION_FEATURE,
  EXECUTION_FUSION_META,
  EXECUTION_IMAGE_EDIT_META,
  EXECUTION_LOCAL_INPAINT_META,
  EXECUTION_META,
} from '../contracts/executionContract'
import {
  buildExecutionSizeParams,
  extendExecutionImageSize,
  type ExecutionImageSize,
} from '../contracts/executionSizeParams'
import { buildPromptAspectRatioTaskParams } from '../contracts/promptAspectRatioTaskParams'
import { assertRequestedOutputCount } from '../../../../shared/domain/generationContract'
import {
  normalizePromptReferenceImageUrls,
  type PromptAspectRatio,
} from '../../../canvas-board/domain'
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
  originalImageUrl: string
  maskImageUrl: string
  referenceImageUrls?: string[]
  outputCount: number
}

export type FusionTaskRequestInput = {
  prompt: string
  imageUrls: string[]
  inputWeights: number[]
  outputCount?: number
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
    prompt: '',
    feature: execution.feature,
    params,
  })
}

export function buildPromptImageReadinessRequest(): SubmitTaskData {
  return buildExecutionTaskRequest({
    prompt: '',
    feature: EXECUTION_IMAGE_EDIT_META.feature,
    params: { output_count: 1 },
  })
}

export function buildLocalInpaintReadinessRequest(): SubmitTaskData {
  return buildExecutionTaskRequest({
    prompt: '',
    feature: EXECUTION_LOCAL_INPAINT_META.feature,
    params: { output_count: 1 },
  })
}

export function buildCanvasExtendReadinessRequest(): SubmitTaskData {
  const execution = requireImageActionExecution(IMAGE_CANVAS_EXTEND_ACTION)
  return buildExecutionTaskRequest({
    prompt: '',
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
    prompt: '',
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

export function buildPromptGenerationTaskRequest(input: {
  prompt: string
  aspectRatio: PromptAspectRatio
  imageUrls?: string[]
  outputCount: number
}): SubmitTaskData {
  const outputCount = assertRequestedOutputCount(input.outputCount)
  return buildExecutionTaskRequest({
    prompt: `${input.prompt ?? ''}`.trim(),
    feature: EXECUTION_META[EXECUTION_FEATURE.promptGeneration].feature,
    imageUrls: input.imageUrls,
    params: {
      output_count: outputCount,
      ...buildPromptAspectRatioTaskParams(input.aspectRatio),
    },
  })
}

export function buildDirectActionTaskRequest(input: {
  source: ImageExecutionInput
  action: ImageDirectTaskAction
  imageSize: ExecutionImageSize | null
  prompt: string
  outputCount: number
  strength?: number
}): SubmitTaskData {
  const sourceUrl = requireImageExecutionInputUrl(input.source)
  const execution = requireImageActionExecution(input.action)
  const sizeParams = buildExecutionSizeParams(input.imageSize)
  const strengthConfig = execution.strength
  if (strengthConfig?.paramPath === 'variation.strength') {
    return buildExecutionTaskRequest({
      prompt: input.prompt.trim(),
      feature: execution.feature,
      imageUrls: [sourceUrl],
      params: {
        output_count: assertRequestedOutputCount(input.outputCount),
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
    prompt: input.prompt.trim(),
    feature: execution.feature,
    imageUrls: [sourceUrl],
    params: {
      output_count: assertRequestedOutputCount(input.outputCount),
      ...sizeParams,
    },
  })
}

export function buildPromptImageTaskRequest(input: {
  source: ImageExecutionInput
  prompt: string
  imageSize: ExecutionImageSize | null
  aspectRatio: PromptAspectRatio
  outputCount: number
}): SubmitTaskData {
  const sourceUrl = requireImageExecutionInputUrl(input.source)
  return buildExecutionTaskRequest({
    prompt: input.prompt.trim(),
    feature: EXECUTION_IMAGE_EDIT_META.feature,
    imageUrls: [sourceUrl],
    params: {
      output_count: assertRequestedOutputCount(input.outputCount),
      ...buildExecutionSizeParams(input.imageSize),
      ...buildPromptAspectRatioTaskParams(input.aspectRatio),
    },
  })
}

export function buildImageInpaintTaskRequest(input: ImageInpaintTaskRequestInput): SubmitTaskData {
  const referenceImageUrls = normalizePromptReferenceImageUrls(input.referenceImageUrls)
  return buildExecutionTaskRequest({
    prompt: input.prompt.trim(),
    feature: EXECUTION_LOCAL_INPAINT_META.feature,
    params: {
      output_count: assertRequestedOutputCount(input.outputCount),
      original_image_url: input.originalImageUrl,
      mask_image_url: input.maskImageUrl,
      reference_image_urls: referenceImageUrls,
      use_mask: true,
    },
  })
}

export function buildCanvasExtendTaskRequest(input: {
  source: ImageExecutionInput
  imageSize: ExecutionImageSize | null
  extendPercent: CanvasExtendPercent
  prompt?: string
  outputCount: number
}): SubmitTaskData {
  const sourceUrl = requireImageExecutionInputUrl(input.source)
  const prompt = `${input.prompt ?? ''}`.trim()
  const execution = requireImageActionExecution(IMAGE_CANVAS_EXTEND_ACTION)
  return buildExecutionTaskRequest({
    prompt,
    feature: execution.feature,
    imageUrls: [sourceUrl],
    params: {
      output_count: assertRequestedOutputCount(input.outputCount),
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
  prompt: string
  outputCount: number
  strength?: number
}): SubmitTaskData {
  const sourceUrl = requireImageExecutionInputUrl(input.source)
  const execution = requireImageActionExecution(IMAGE_HD_UPSCALE_ACTION)
  return buildExecutionTaskRequest({
    prompt: input.prompt.trim(),
    feature: execution.feature,
    imageUrls: [sourceUrl],
    params: {
      output_count: assertRequestedOutputCount(input.outputCount),
      ...buildExecutionSizeParams(input.imageSize, input.imageSize),
      hd: {
        clarity_strength: resolveHdClarityStrength(input.strength),
      },
    },
  })
}

export function buildFusionTaskRequest(input: FusionTaskRequestInput): SubmitTaskData {
  const outputCount =
    input.outputCount === undefined ? undefined : assertRequestedOutputCount(input.outputCount)
  return buildExecutionTaskRequest({
    prompt: input.prompt,
    feature: EXECUTION_FUSION_META.feature,
    imageUrls: input.imageUrls,
    params: {
      ...(outputCount ? { output_count: outputCount } : {}),
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
  | PromptGenerationExecutionPlan
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
          prompt: plan.prompt,
          outputCount: plan.outputCount,
          strength: plan.strength,
        })
      }
      if (isImageDirectTaskAction(plan.action)) {
        return buildDirectActionTaskRequest({
          source: plan.source,
          action: plan.action,
          imageSize,
          prompt: plan.prompt,
          outputCount: plan.outputCount,
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
        outputCount: plan.outputCount,
      })
    case 'image-prompt':
      return buildPromptImageTaskRequest({
        source: plan.source,
        prompt: plan.prompt,
        imageSize,
        aspectRatio: plan.aspectRatio,
        outputCount: plan.outputCount,
      })
    case 'prompt-generation':
      return buildPromptGenerationTaskRequest({
        prompt: plan.prompt,
        aspectRatio: plan.aspectRatio,
        imageUrls: plan.referenceImages.map((image) => image.url),
        outputCount: plan.outputCount,
      })
    case 'fusion':
      return buildFusionTaskRequest({
        prompt: plan.taskPrompt,
        imageUrls: plan.imageUrls,
        inputWeights: plan.inputWeights,
        outputCount: plan.outputCount,
        sizeSource: plan.sizeSource,
        targetSize: plan.targetSize,
        aspectRatio: plan.aspectRatio,
      })
  }
}

export function buildPreparedImageInpaintTaskRequest(input: {
  plan: ImageInpaintExecutionPlan
  maskImageUrl: string
}): SubmitTaskData {
  return buildImageInpaintTaskRequest({
    prompt: input.plan.prompt,
    originalImageUrl: input.plan.source.url,
    maskImageUrl: input.maskImageUrl,
    referenceImageUrls: input.plan.referenceImageUrls,
    outputCount: input.plan.outputCount,
  })
}
