import type { SubmitTaskData } from '../../../../../shared/api/task'
import type {
  OutpaintExecutionPlan,
  ImageActionExecutionPlan,
  ImageInpaintExecutionPlan,
  ImageExecutionInput,
  GenerationExecutionPlan,
  PreparedFusionExecutionPlan,
} from '../planning/imageExecutionPlan'
import {
  resolveOutpaintPixelGeometry,
  type OutpaintRect,
} from '../../../domain/graph/outpaintGeometry.ts'
import {
  buildExecutionTaskRequest,
  EXECUTION_FEATURE,
  EXECUTION_FUSION_META,
  EXECUTION_PARTIAL_EDIT_META,
  EXECUTION_META,
} from '../../../domain/execution-contracts/executionContract.ts'
import {
  buildExecutionSizeParams,
  type ExecutionImageSize,
} from '../../../domain/execution-contracts/sizeParams.ts'
import { buildPromptAspectRatioTaskParams } from '../../../domain/execution-contracts/promptAspectRatioTaskParams.ts'
import { assertRequestedOutputCount } from '../../../../../shared/domain/generationContract'
import type { OutputResolution } from '../../../../../shared/domain/generationContract'
import type { PromptAspectRatio } from '../../../../board'
import { i18n } from '../../../../../shared/i18n'
import {
  IMAGE_OUTPAINT_ACTION,
  IMAGE_UPSCALE_ACTION,
  isImageDirectTaskAction,
  resolveUpscaleScale,
  requireImageActionExecution,
  type ImageDirectTaskAction,
  type ImageExecutionAction,
} from '../../../domain/execution-contracts/imageActionExecution.ts'

function requireImageExecutionInputUrl(source: ImageExecutionInput): string {
  if (source.mediaAssetId) return ''
  const url = `${source.url ?? ''}`.trim()
  if (!url) throw new Error(i18n.global.t('canvas.execution.imageMissingUrl'))
  return url
}

function imageAssetIds(sources: ImageExecutionInput[]): number[] {
  return sources
    .map((source) => source.mediaAssetId)
    .filter((id): id is number => id !== undefined && Number.isInteger(id) && id > 0)
}

export type ImageInpaintTaskRequestInput = {
  prompt: string
  originalImageUrl: string
  maskImageUrl: string
  imageSize: ExecutionImageSize | null
  originalImageAssetId?: number | undefined
}

export type FusionTaskRequestInput = {
  prompt: string
  imageUrls: string[]
  imageAssetIds?: number[] | undefined
  inputWeights: number[]
  outputCount?: number | undefined
  sizeSource: ExecutionImageSize | null
  targetSize: ExecutionImageSize | null
  aspectRatio: PromptAspectRatio
  outputResolution?: OutputResolution | undefined
}

type ImageActionReadinessOptions = {
  outputCount?: number | undefined
  outputResolution?: OutputResolution | undefined
}

function buildOutputBillingParams(options: ImageActionReadinessOptions = {}, feature?: string) {
  const isRemoveBg = feature === EXECUTION_FEATURE.cutout
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

export function buildInpaintReadinessRequest(): SubmitTaskData {
  return buildExecutionTaskRequest({
    prompt: '[inpaint] submit readiness',
    feature: EXECUTION_PARTIAL_EDIT_META.feature,
    params: {
      output_count: 1,
      output_resolution: 'default',
      ...buildExecutionSizeParams({ width: 1024, height: 1024 }, { width: 1024, height: 1024 }),
    },
  })
}

export function buildOutpaintReadinessRequest(): SubmitTaskData {
  const execution = requireImageActionExecution(IMAGE_OUTPAINT_ACTION)
  return buildExecutionTaskRequest({
    prompt: '',
    feature: execution.feature,
    params: {
      output_count: 1,
      output_resolution: 'default',
      original_image_width: 1024,
      original_image_height: 1024,
      outpaint: { target_width: 1032, target_height: 1024, image_offset_x: 0, image_offset_y: 0 },
    },
  })
}

function buildUpscaleReadinessRequest(): SubmitTaskData {
  const execution = requireImageActionExecution(IMAGE_UPSCALE_ACTION)
  return buildExecutionTaskRequest({
    prompt: '',
    feature: execution.feature,
    params: {
      upscale: {
        scale: resolveUpscaleScale(),
      },
    },
  })
}

export function buildImageActionReadinessRequest(
  action: ImageExecutionAction,
  options: ImageActionReadinessOptions = {},
): SubmitTaskData {
  if (action === IMAGE_OUTPAINT_ACTION) return buildOutpaintReadinessRequest()
  if (action === IMAGE_UPSCALE_ACTION) return buildUpscaleReadinessRequest()
  if (isImageDirectTaskAction(action)) return buildDirectActionReadinessRequest(action, options)
  throw new Error(`Unsupported image execution action ${action}`)
}

export function buildGenerationTaskRequest(input: {
  prompt: string
  aspectRatio: PromptAspectRatio
  imageUrls?: string[] | undefined
  imageAssetIds?: number[] | undefined
  referenceImageAssetIds?: number[] | undefined
  outputCount: number
  outputResolution?:
    import('../../../../../shared/domain/generationContract').OutputResolution | undefined
}): SubmitTaskData {
  const outputCount = assertRequestedOutputCount(input.outputCount)
  return buildExecutionTaskRequest({
    prompt: `${input.prompt ?? ''}`.trim(),
    feature: EXECUTION_META[EXECUTION_FEATURE.generation].feature,
    imageUrls: input.imageUrls,
    imageAssetIds: input.imageAssetIds,
    referenceImageAssetIds: input.referenceImageAssetIds,
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
  strength?: number | undefined
  outputResolution?: OutputResolution | undefined
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
      imageUrls: input.source.mediaAssetId ? undefined : [sourceUrl],
      imageAssetIds: input.source.mediaAssetId ? [input.source.mediaAssetId] : undefined,
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
    imageUrls: input.source.mediaAssetId ? undefined : [sourceUrl],
    imageAssetIds: input.source.mediaAssetId ? [input.source.mediaAssetId] : undefined,
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
      mask_image_url: input.maskImageUrl,
      use_mask: true,
      ...(input.originalImageAssetId
        ? { original_image_asset_id: input.originalImageAssetId }
        : {}),
    },
  })
}

export function buildOutpaintTaskRequest(input: {
  source: ImageExecutionInput
  imageSize: ExecutionImageSize | null
  frameRect: OutpaintRect
  originalImageRect: OutpaintRect
  prompt?: string
}): SubmitTaskData {
  const sourceUrl = requireImageExecutionInputUrl(input.source)
  const prompt = `${input.prompt ?? ''}`.trim()
  const execution = requireImageActionExecution(IMAGE_OUTPAINT_ACTION)
  if (!input.imageSize)
    throw new Error(i18n.global.t('canvas.execution.outpaintMissingOriginalSize'))
  const geometry = resolveOutpaintPixelGeometry(
    input.imageSize,
    input.frameRect,
    input.originalImageRect,
  )
  return buildExecutionTaskRequest({
    prompt,
    feature: execution.feature,
    imageUrls: input.source.mediaAssetId ? undefined : [sourceUrl],
    imageAssetIds: input.source.mediaAssetId ? [input.source.mediaAssetId] : undefined,
    params: {
      output_count: 1,
      output_resolution: 'default',
      original_image_width: input.imageSize.width,
      original_image_height: input.imageSize.height,
      outpaint: {
        target_width: geometry.targetWidth,
        target_height: geometry.targetHeight,
        image_offset_x: geometry.imageOffsetX,
        image_offset_y: geometry.imageOffsetY,
      },
    },
  })
}

export function buildUpscaleTaskRequest(input: {
  source: ImageExecutionInput
  imageSize: ExecutionImageSize | null
  prompt: string
  outputCount: number
  strength?: number | undefined
  outputResolution?: OutputResolution | undefined
}): SubmitTaskData {
  const sourceUrl = requireImageExecutionInputUrl(input.source)
  const execution = requireImageActionExecution(IMAGE_UPSCALE_ACTION)
  return buildExecutionTaskRequest({
    prompt: '',
    feature: execution.feature,
    imageUrls: input.source.mediaAssetId ? undefined : [sourceUrl],
    imageAssetIds: input.source.mediaAssetId ? [input.source.mediaAssetId] : undefined,
    params: {
      ...buildExecutionSizeParams(input.imageSize, input.imageSize),
      upscale: {
        scale: resolveUpscaleScale(input.strength),
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
    imageAssetIds: input.imageAssetIds,
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
  | OutpaintExecutionPlan
  | GenerationExecutionPlan
  | PreparedFusionExecutionPlan

export function buildImageExecutionTaskRequest(
  plan: ImageExecutionTaskRequestPlan,
  imageSize: ExecutionImageSize | null = null,
): SubmitTaskData {
  switch (plan.kind) {
    case 'image-action':
      if (plan.action === IMAGE_UPSCALE_ACTION) {
        return buildUpscaleTaskRequest({
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
    case 'outpaint':
      return buildOutpaintTaskRequest({
        source: plan.source,
        imageSize,
        frameRect: plan.frameRect,
        originalImageRect: plan.originalImageRect,
        prompt: plan.prompt,
      })
    case 'generation':
      return buildGenerationTaskRequest({
        prompt: plan.prompt,
        aspectRatio: plan.aspectRatio,
        imageUrls: plan.referenceImages.some((image) => !image.mediaAssetId)
          ? plan.referenceImages.filter((image) => !image.mediaAssetId).map((image) => image.url)
          : undefined,
        referenceImageAssetIds: imageAssetIds(plan.referenceImages),
        outputCount: plan.outputCount,
        outputResolution: plan.outputResolution,
      })
    case 'fusion':
      return buildFusionTaskRequest({
        prompt: plan.taskPrompt,
        imageUrls: plan.imageUrls,
        imageAssetIds: imageAssetIds(plan.inputImages),
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
    originalImageAssetId: input.plan.source.mediaAssetId,
  })
}
