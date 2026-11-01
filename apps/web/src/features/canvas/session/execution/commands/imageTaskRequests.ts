import type { CanvasTaskRequestData } from '../../../core/execution/canvasTaskRequest'
import type {
  OutpaintExecutionPlan,
  ImageActionExecutionPlan,
  ImageInpaintExecutionPlan,
  ImageExecutionInput,
  GenerationExecutionPlan,
  PreparedFusionExecutionPlan,
} from '../planning/imageExecutionPlan'
import { resolveOutpaintPixelGeometry, type OutpaintRect } from '../../../tools/outpaint/core'
import {
  buildExecutionTaskRequest,
  EXECUTION_SERVICE,
  EXECUTION_FUSION_META,
  EXECUTION_PARTIAL_EDIT_META,
  EXECUTION_META,
} from '../../../core/execution/executionContract.ts'
import {
  buildExecutionSizeParams,
  type ExecutionImageSize,
} from '../../../core/execution/sizeParams.ts'
import { buildPromptAspectRatioTaskParams } from '../../../core/execution/promptAspectRatioTaskParams.ts'
import { assertRequestedOutputCount } from '../../../../../core/contracts/generationContract'
import type { OutputResolution } from '../../../../../core/contracts/generationContract'
import type { PromptAspectRatio } from '../../../../board/ports/canvas'
import { i18n } from '../../../../../core/i18n'
import { CanvasWorkspaceTaskUnmappableError } from '../workspace/canvasTaskContractMapping'
import {
  IMAGE_OUTPAINT_ACTION,
  IMAGE_UPSCALE_ACTION,
  isImageDirectTaskAction,
  resolveUpscaleScale,
  requireImageActionExecution,
  type ImageDirectTaskAction,
  type ImageExecutionAction,
} from '../../../core/execution/imageActionExecution.ts'

function requireImageAssetId(source: { assetId?: string | undefined }): string {
  const id = `${source.assetId ?? ''}`.trim()
  if (!id) {
    throw new CanvasWorkspaceTaskUnmappableError(
      'media-ids',
      'Canvas execution requires an asset-backed image so media_asset_ids is populated.',
    )
  }
  return id
}

type ImageInpaintTaskRequestInput = {
  prompt: string
  imageSize: ExecutionImageSize | null
  originalImageAssetId: string
  maskAssetId: string
}

export type FusionTaskRequestInput = {
  prompt: string
  /** Opaque `asset_...` refs; the only media inputs the v1 transport sends. */
  mediaAssetIds?: string[] | undefined
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

function buildOutputBillingParams(options: ImageActionReadinessOptions = {}, service?: string) {
  const isRemoveBg = service === EXECUTION_SERVICE.cutout
  return {
    output_count: assertRequestedOutputCount(isRemoveBg ? 1 : (options.outputCount ?? 1)),
    output_resolution: isRemoveBg ? ('1k' as const) : (options.outputResolution ?? '1k'),
  }
}

function buildDirectActionReadinessRequest(
  action: ImageDirectTaskAction,
  options: ImageActionReadinessOptions = {},
): CanvasTaskRequestData {
  const execution = requireImageActionExecution(action)
  const strengthConfig = execution.strength
  const params = {
    ...buildOutputBillingParams(options, execution.service),
    ...(strengthConfig?.paramPath === 'variation.strength'
      ? { variation: { strength: strengthConfig.defaultValue } }
      : {}),
  }
  return buildExecutionTaskRequest({
    prompt: '',
    service: execution.service,
    params,
  })
}

export function buildInpaintReadinessRequest(): CanvasTaskRequestData {
  return buildExecutionTaskRequest({
    prompt: '[inpaint] submit readiness',
    service: EXECUTION_PARTIAL_EDIT_META.service,
    params: {
      output_count: 1,
      output_resolution: '1k',
      ...buildExecutionSizeParams({ width: 1024, height: 1024 }, { width: 1024, height: 1024 }),
    },
  })
}

export function buildOutpaintReadinessRequest(): CanvasTaskRequestData {
  const execution = requireImageActionExecution(IMAGE_OUTPAINT_ACTION)
  return buildExecutionTaskRequest({
    prompt: '',
    service: execution.service,
    params: {
      output_count: 1,
      output_resolution: '1k',
      original_image_width: 1024,
      original_image_height: 1024,
      outpaint: { target_width: 1032, target_height: 1024, image_offset_x: 0, image_offset_y: 0 },
    },
  })
}

function buildUpscaleReadinessRequest(): CanvasTaskRequestData {
  const execution = requireImageActionExecution(IMAGE_UPSCALE_ACTION)
  return buildExecutionTaskRequest({
    prompt: '',
    service: execution.service,
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
): CanvasTaskRequestData {
  if (action === IMAGE_OUTPAINT_ACTION) return buildOutpaintReadinessRequest()
  if (action === IMAGE_UPSCALE_ACTION) return buildUpscaleReadinessRequest()
  if (isImageDirectTaskAction(action)) return buildDirectActionReadinessRequest(action, options)
  throw new Error(`Unsupported image execution action ${action}`)
}

export function buildGenerationTaskRequest(input: {
  prompt: string
  aspectRatio: PromptAspectRatio
  /** Opaque `asset_...` refs; the only media inputs the v1 transport sends. */
  mediaAssetIds?: string[] | undefined
  outputCount: number
  outputResolution?:
    import('../../../../../core/contracts/generationContract').OutputResolution | undefined
}): CanvasTaskRequestData {
  const outputCount = assertRequestedOutputCount(input.outputCount)
  return buildExecutionTaskRequest({
    prompt: `${input.prompt ?? ''}`.trim(),
    service: EXECUTION_META[EXECUTION_SERVICE.generation].service,
    mediaAssetIds: input.mediaAssetIds,
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
}): CanvasTaskRequestData {
  const execution = requireImageActionExecution(input.action)
  const sizeParams = buildExecutionSizeParams(input.imageSize)
  const strengthConfig = execution.strength
  const outputParams = buildOutputBillingParams(
    { outputCount: input.outputCount, outputResolution: input.outputResolution },
    execution.service,
  )
  const assetId = requireImageAssetId(input.source)
  if (strengthConfig?.paramPath === 'variation.strength') {
    return buildExecutionTaskRequest({
      prompt: input.prompt.trim(),
      service: execution.service,
      mediaAssetIds: [assetId],
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
    service: execution.service,
    mediaAssetIds: [assetId],
    params: {
      ...outputParams,
      ...sizeParams,
    },
  })
}

export function buildImageInpaintTaskRequest(
  input: ImageInpaintTaskRequestInput,
): CanvasTaskRequestData {
  const originalAssetId = requireImageAssetId({ assetId: input.originalImageAssetId })
  const maskAssetId = requireImageAssetId({ assetId: input.maskAssetId })
  return buildExecutionTaskRequest({
    prompt: input.prompt.trim(),
    service: EXECUTION_PARTIAL_EDIT_META.service,
    mediaAssetIds: [originalAssetId],
    params: {
      output_count: 1,
      output_resolution: '1k',
      ...buildExecutionSizeParams(input.imageSize, input.imageSize),
      mask_asset_id: maskAssetId,
      use_mask: true,
    },
  })
}

export function buildOutpaintTaskRequest(input: {
  source: ImageExecutionInput
  imageSize: ExecutionImageSize | null
  frameRect: OutpaintRect
  originalImageRect: OutpaintRect
  prompt?: string
}): CanvasTaskRequestData {
  const prompt = `${input.prompt ?? ''}`.trim()
  const execution = requireImageActionExecution(IMAGE_OUTPAINT_ACTION)
  if (!input.imageSize)
    throw new Error(i18n.global.t('canvas.execution.outpaintMissingOriginalSize'))
  const geometry = resolveOutpaintPixelGeometry(
    input.imageSize,
    input.frameRect,
    input.originalImageRect,
  )
  const assetId = requireImageAssetId(input.source)
  return buildExecutionTaskRequest({
    prompt,
    service: execution.service,
    mediaAssetIds: [assetId],
    params: {
      output_count: 1,
      output_resolution: '1k',
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
}): CanvasTaskRequestData {
  const execution = requireImageActionExecution(IMAGE_UPSCALE_ACTION)
  const assetId = requireImageAssetId(input.source)
  return buildExecutionTaskRequest({
    prompt: '',
    service: execution.service,
    mediaAssetIds: [assetId],
    params: {
      ...buildExecutionSizeParams(input.imageSize, input.imageSize),
      upscale: {
        scale: resolveUpscaleScale(input.strength),
      },
    },
  })
}

export function buildFusionTaskRequest(input: FusionTaskRequestInput): CanvasTaskRequestData {
  const outputCount =
    input.outputCount === undefined ? undefined : assertRequestedOutputCount(input.outputCount)
  return buildExecutionTaskRequest({
    prompt: input.prompt,
    service: EXECUTION_FUSION_META.service,
    mediaAssetIds: input.mediaAssetIds,
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
): CanvasTaskRequestData {
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
        mediaAssetIds: plan.referenceImages.map(requireImageAssetId),
        outputCount: plan.outputCount,
        outputResolution: plan.outputResolution,
      })
    case 'fusion':
      return buildFusionTaskRequest({
        prompt: plan.taskPrompt,
        mediaAssetIds: plan.inputImages.map(requireImageAssetId),
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
  maskAssetId: string
  imageSize: ExecutionImageSize | null
}): CanvasTaskRequestData {
  return buildImageInpaintTaskRequest({
    prompt: input.plan.prompt,
    maskAssetId: input.maskAssetId,
    imageSize: input.imageSize,
    originalImageAssetId: requireImageAssetId(input.plan.source),
  })
}
