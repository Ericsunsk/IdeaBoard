import type { SubmitTaskData } from '../../../../shared/api/task'
import type {
  CanvasExtendExecutionPlan,
  ImageActionExecutionPlan,
  ImageInpaintExecutionPlan,
  ImageExecutionInput,
  PromptGenerationExecutionPlan,
  PreparedFusionExecutionPlan,
} from '../planning/ImageExecutionPlan'
import { resolveCanvasExtendPixelGeometry, type CanvasExtendRect } from '../../utils/canvasExtendGeometry'
import {
  buildExecutionTaskRequest,
  EXECUTION_FEATURE,
  EXECUTION_FUSION_META,
  EXECUTION_PARTIAL_EDIT_META,
  EXECUTION_META,
} from '../contracts/executionContract'
import {
  buildExecutionSizeParams,
  type ExecutionImageSize,
} from '../contracts/executionSizeParams'
import { buildPromptAspectRatioTaskParams } from '../contracts/promptAspectRatioTaskParams'
import { assertRequestedOutputCount } from '../../../../shared/domain/generationContract'
import type { OutputResolution } from '../../../../shared/domain/generationContract'
import type { PromptAspectRatio } from '../../../canvas-board/domain'
import {
  IMAGE_CANVAS_EXTEND_ACTION,
  IMAGE_HD_UPSCALE_ACTION,
  isImageDirectTaskAction,
  resolveHdScale,
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
  imageSize: ExecutionImageSize | null
}

export type FusionTaskRequestInput = {
  prompt: string
  imageUrls: string[]
  inputWeights: number[]
  outputCount?: number
  sizeSource: ExecutionImageSize | null
  targetSize: ExecutionImageSize | null
  aspectRatio: PromptAspectRatio
  outputResolution?: OutputResolution
}

type ImageActionReadinessOptions = {
  outputCount?: number
  outputResolution?: OutputResolution
}

function buildOutputBillingParams(
  options: ImageActionReadinessOptions = {},
  feature?: string,
) {
  const isRemoveBg = feature === EXECUTION_FEATURE.removeBg
  return {
    output_count: assertRequestedOutputCount(isRemoveBg ? 1 : (options.outputCount ?? 1)),
    output_resolution: isRemoveBg ? ('default' as const) : (options.outputResolution ?? '1k'),
  }
}

function buildDirectActionReadinessRequest(
  action: ImageDirectTaskAction,
  options: ImageActionReadinessOptions = {},
): SubmitTaskData {
  const execution = requireImageActionExecution(action)
  const strengthConfig = execution.strength
  const params = {
    ...buildOutputBillingParams(options, execution.feature),
    ...(strengthConfig?.paramPath === 'variation.strength'
      ? { variation: { strength: strengthConfig.defaultValue } }
      : {}),
  }
  return buildExecutionTaskRequest({
    prompt: '',
    feature: execution.feature,
    params,
  })
}

export function buildPartialEditReadinessRequest(): SubmitTaskData {
  return buildExecutionTaskRequest({
    prompt: '局部重绘',
    feature: EXECUTION_PARTIAL_EDIT_META.feature,
    params: {
      output_count: 1,
      output_resolution: 'default',
      ...buildExecutionSizeParams(
        { width: 1024, height: 1024 },
        { width: 1024, height: 1024 },
      ),
    },
  })
}

export function buildCanvasExtendReadinessRequest(): SubmitTaskData {
  const execution = requireImageActionExecution(IMAGE_CANVAS_EXTEND_ACTION)
  return buildExecutionTaskRequest({
    prompt: '',
    feature: execution.feature,
    params: {
      output_count: 1,
      output_resolution: 'default',
      original_image_width: 1024,
      original_image_height: 1024,
      extend: { target_width: 1032, target_height: 1024, image_offset_x: 0, image_offset_y: 0 },
    },
  })
}

function buildHdUpscaleReadinessRequest(): SubmitTaskData {
  const execution = requireImageActionExecution(IMAGE_HD_UPSCALE_ACTION)
  return buildExecutionTaskRequest({
    prompt: '',
    feature: execution.feature,
    params: {
      hd: {
        scale: resolveHdScale(),
      },
    },
  })
}

export function buildImageActionReadinessRequest(
  action: ImageExecutionAction,
  options: ImageActionReadinessOptions = {},
): SubmitTaskData {
  if (action === IMAGE_CANVAS_EXTEND_ACTION) return buildCanvasExtendReadinessRequest()
  if (action === IMAGE_HD_UPSCALE_ACTION) return buildHdUpscaleReadinessRequest()
  if (isImageDirectTaskAction(action)) return buildDirectActionReadinessRequest(action, options)
  throw new Error(`Unsupported image execution action ${action}`)
}

export function buildPromptGenerationTaskRequest(input: {
  prompt: string
  aspectRatio: PromptAspectRatio
  imageUrls?: string[]
  outputCount: number
  outputResolution?: import('../../../../shared/domain/generationContract').OutputResolution
}): SubmitTaskData {
  const outputCount = assertRequestedOutputCount(input.outputCount)
  return buildExecutionTaskRequest({
    prompt: `${input.prompt ?? ''}`.trim(),
    feature: EXECUTION_META[EXECUTION_FEATURE.promptGeneration].feature,
    imageUrls: input.imageUrls,
    params: {
      output_count: outputCount,
      output_resolution: input.outputResolution ?? '1k',
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
  outputResolution?: OutputResolution
}): SubmitTaskData {
  const sourceUrl = requireImageExecutionInputUrl(input.source)
  const execution = requireImageActionExecution(input.action)
  const sizeParams = buildExecutionSizeParams(input.imageSize)
  const strengthConfig = execution.strength
  const outputParams = buildOutputBillingParams(
    { outputCount: input.outputCount, outputResolution: input.outputResolution },
    execution.feature,
  )
  if (strengthConfig?.paramPath === 'variation.strength') {
    return buildExecutionTaskRequest({
      prompt: input.prompt.trim(),
      feature: execution.feature,
      imageUrls: [sourceUrl],
      params: {
        ...outputParams,
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
      ...outputParams,
      ...sizeParams,
    },
  })
}

export function buildImageInpaintTaskRequest(input: ImageInpaintTaskRequestInput): SubmitTaskData {
  return buildExecutionTaskRequest({
    prompt: input.prompt.trim(),
    feature: EXECUTION_PARTIAL_EDIT_META.feature,
    params: {
      output_count: 1,
      output_resolution: 'default',
      ...buildExecutionSizeParams(input.imageSize, input.imageSize),
      original_image_url: input.originalImageUrl,
      mask_image_url: input.maskImageUrl,
      use_mask: true,
    },
  })
}

export function buildCanvasExtendTaskRequest(input: {
  source: ImageExecutionInput
  imageSize: ExecutionImageSize | null
  frameRect: CanvasExtendRect
  originalImageRect: CanvasExtendRect
  prompt?: string
}): SubmitTaskData {
  const sourceUrl = requireImageExecutionInputUrl(input.source)
  const prompt = `${input.prompt ?? ''}`.trim()
  const execution = requireImageActionExecution(IMAGE_CANVAS_EXTEND_ACTION)
  if (!input.imageSize) throw new Error('扩图缺少原图尺寸')
  const geometry = resolveCanvasExtendPixelGeometry(input.imageSize, input.frameRect, input.originalImageRect)
  return buildExecutionTaskRequest({
    prompt,
    feature: execution.feature,
    imageUrls: [sourceUrl],
    params: {
      output_count: 1,
      output_resolution: 'default',
      original_image_width: input.imageSize.width,
      original_image_height: input.imageSize.height,
      extend: {
        target_width: geometry.targetWidth,
        target_height: geometry.targetHeight,
        image_offset_x: geometry.imageOffsetX,
        image_offset_y: geometry.imageOffsetY,
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
  outputResolution?: OutputResolution
}): SubmitTaskData {
  const sourceUrl = requireImageExecutionInputUrl(input.source)
  const execution = requireImageActionExecution(IMAGE_HD_UPSCALE_ACTION)
  return buildExecutionTaskRequest({
    prompt: '',
    feature: execution.feature,
    imageUrls: [sourceUrl],
    params: {
      ...buildExecutionSizeParams(input.imageSize, input.imageSize),
      hd: {
        scale: resolveHdScale(input.strength),
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
      output_resolution: input.outputResolution ?? '1k',
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
          outputResolution: plan.outputResolution,
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
          outputResolution: plan.outputResolution,
        })
      }
      throw new Error(`Unsupported image task action ${plan.action}`)
    case 'canvas-extend':
      return buildCanvasExtendTaskRequest({
        source: plan.source,
        imageSize,
        frameRect: plan.frameRect,
        originalImageRect: plan.originalImageRect,
        prompt: plan.prompt,
      })
    case 'prompt-generation':
      return buildPromptGenerationTaskRequest({
        prompt: plan.prompt,
        aspectRatio: plan.aspectRatio,
        imageUrls: plan.referenceImages.map((image) => image.url),
        outputCount: plan.outputCount,
        outputResolution: plan.outputResolution,
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
        outputResolution: plan.outputResolution,
      })
  }
}

export function buildPreparedImageInpaintTaskRequest(input: {
  plan: ImageInpaintExecutionPlan
  maskImageUrl: string
  imageSize: ExecutionImageSize | null
}): SubmitTaskData {
  return buildImageInpaintTaskRequest({
    prompt: input.plan.prompt,
    originalImageUrl: input.plan.source.url,
    maskImageUrl: input.maskImageUrl,
    imageSize: input.imageSize,
  })
}
